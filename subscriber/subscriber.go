package subscriber

import (
	
	"time"
	"fmt"
	"errors"
	"encoding/json"
	// "github.com/centrifuge/go-substrate-rpc-client/v3/scale"
	"github.com/centrifuge/go-substrate-rpc-client/v3/types"
	

	"github.com/gorilla/websocket"
	"github.com/smartcontractkit/chainlink/core/logger"

)

// JsonrpcMessage declares JSON-RPC message type
type JsonrpcMessage struct {
	Version string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Error   *interface{}    `json:"error,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
}

func GetTestJson() []byte {
	msg := JsonrpcMessage{
		Version: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "state_getMetadata",
	}
	data, _ := json.Marshal(msg)
	return data
}

var sm_meta *types.Metadata
var sm_key types.StorageKey

func ParseTestResponse(data []byte) error {
	var msg JsonrpcMessage
	err := json.Unmarshal(data, &msg)
	if err != nil {
		return err
	}

	var res string
	err = json.Unmarshal(msg.Result, &res)
	if err != nil {
		return err
	}
	// fmt.Println(res)

	var metadata types.Metadata
	err = types.DecodeFromHexString(res, &metadata)
	if err != nil {
		return err
	}

	// fmt.Println(metadata)
	
	sm_meta = &metadata
	return nil
}

func Test(endpoint string) error {

	// TEST

	c, _, err := websocket.DefaultDialer.Dial(endpoint, nil)
	if err != nil {
		return err
	}
	defer logger.ErrorIfCalling(c.Close)


	testPayload := GetTestJson()
	if testPayload == nil {
		return nil
	}

	resp := make(chan []byte)

	go func() {
		var body []byte
		_, body, err = c.ReadMessage()
		if err != nil {
			fmt.Println(err)
			close(resp)
		}
		resp <- body
	}()

	err = c.WriteMessage(websocket.BinaryMessage, testPayload)
	if err != nil {
		return err
	}

	// Set timeout for response to 5 seconds
	t := time.NewTimer(5 * time.Second)
	defer t.Stop()

	select {
	case <-t.C:
		return errors.New("timeout from test payload")
	case body, ok := <-resp:
		if !ok {
			return errors.New("failed reading test response from WS endpoint")
		}

		return ParseTestResponse(body)
	}

}



func readMessages(c *websocket.Conn, channel chan<- Event) {
	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			_ = c.Close()
			// if !wss.conn.closing {
			// 	wss.reconnect()
			// 	return
			// }
			return
		}

		// // First message is a confirmation with the subscription id
		// // Ignore this
		// if !wss.confirmed {
		// 	wss.confirmed = true
		// 	continue
		// }

		events, ok := ParseResponse(message)
		if !ok {
			continue
		}

		for _, event := range events {
			channel <- event
		}
	}
}

type Event []byte

type substrateSubscribeResponse struct {
	Subscription string          `json:"subscription"`
	Result       json.RawMessage `json:"result"`
}

type SubstrateRequestParams []string

// EventChainlinkOracleRequest is the event structure we expect
// to be emitted from the Chainlink pallet
type EventChainlinkOracleRequest struct {
	Phase              types.Phase
	OracleAccountID    types.AccountID
	SpecIndex          types.Text
	RequestIdentifier  types.U64
	RequesterAccountID types.AccountID
	DataVersion        types.U64
	Bytes              SubstrateRequestParams
	Callback           types.Text
	Payment            types.U32
	Topics             []types.Hash
}

type EventChainlinkOracleAnswer struct {
	Phase              types.Phase
	OracleAccountID    types.AccountID
	RequestIdentifier  types.U64
	RequesterAccountID types.AccountID
	Bytes              types.Text
	Payment            types.U32
	Topics             []types.Hash
}

type EventChainlinkOperatorRegistered struct {
	Phase     types.Phase
	AccountID types.AccountID
	Topics    []types.Hash
}

type EventChainlinkOperatorUnregistered struct {
	Phase     types.Phase
	AccountID types.AccountID
	Topics    []types.Hash
}

type EventChainlinkKillRequest struct {
	Phase             types.Phase
	RequestIdentifier types.U64
	Topics            []types.Hash
}

type EventRecords struct {
	types.EventRecords
	Chainlink_OracleRequest        []EventChainlinkOracleRequest        //nolint:stylecheck,golint
	Chainlink_OracleAnswer         []EventChainlinkOracleAnswer         //nolint:stylecheck,golint
	Chainlink_OperatorRegistered   []EventChainlinkOperatorRegistered   //nolint:stylecheck,golint
	Chainlink_OperatorUnregistered []EventChainlinkOperatorUnregistered //nolint:stylecheck,golint
	Chainlink_KillRequest          []EventChainlinkKillRequest          //nolint:stylecheck,golint
}

func convertStringArrayToKV(data []string) map[string]string {
	result := make(map[string]string)
	var key string

	for i, val := range data {
		if len(val) == 0 {
			continue
		}

		if i%2 == 0 {
			key = val
		} else if len(key) != 0 {
			result[key] = val
			key = ""
		}
	}

	return result
}



func  ParseResponse(data []byte) ([]Event, bool) {
	// promLastSourcePing.With(prometheus.Labels{"endpoint": sm.endpointName, "jobid": string(sm.filter.JobID)}).SetToCurrentTime()

	var msg JsonrpcMessage
	err := json.Unmarshal(data, &msg)
	if err != nil {
		logger.Error("Failed parsing JSON-RPC message:", err)
		return nil, false
	}

	var subRes substrateSubscribeResponse
	err = json.Unmarshal(msg.Params, &subRes)
	if err != nil {
		logger.Error("Failed parsing substrateSubscribeResponse:", err)
		return nil, false
	}

	var changes types.StorageChangeSet
	err = json.Unmarshal(subRes.Result, &changes)
	if err != nil {
		logger.Error("Failed parsing StorageChangeSet:", err)
		return nil, false
	}

	var subEvents []Event
	for _, change := range changes.Changes {
		if !types.Eq(change.StorageKey, sm_key) || !change.HasStorageData {
			logger.Error("Does not match storage")
			continue
		}

		events := EventRecords{}
		err = types.EventRecordsRaw(change.StorageData).DecodeEventRecords(sm_meta, &events)
		if err != nil {
			logger.Errorw("Failed parsing EventRecords:",
				"err", err,
				"change.StorageData", change.StorageData,
				"sm_key", sm_key,
				"types.EventRecordsRaw", types.EventRecordsRaw(change.StorageData))
			continue
		}

		for _, request := range events.Chainlink_OracleRequest {
			// Check if our jobID matches
			// jobID := fmt.Sprint(sm.filter.JobID)
			// specIndex := fmt.Sprint(request.SpecIndex)
			// if !matchesJobID(jobID, specIndex) {
			// 	logger.Errorf("Does not match job : expected %s, requested %s", jobID, specIndex)
			// 	continue
			// }

			// Check if request is being sent from correct
			// oracle address
			// found := false
			// for _, address := range sm.filter.Address {
			// 	if request.OracleAccountID == address.AsAccountID {
			// 		found = true
			// 		break
			// 	}
			// }
			// if !found {
			// 	logger.Errorf("Does not match OracleAccountID, requested is %s", request.OracleAccountID)
			// 	continue
			// }

			requestParams := convertStringArrayToKV(request.Bytes)
			requestParams["function"] = string(request.Callback)
			requestParams["request_id"] = fmt.Sprint(request.RequestIdentifier)
			requestParams["payment"] = fmt.Sprint(request.Payment)
			event, err := json.Marshal(requestParams)
			if err != nil {
				logger.Error(err)
				continue
			}
			subEvents = append(subEvents, event)
		}
	}

	return subEvents, true
}

func GetTriggerJson() []byte {
	if sm_meta == nil {
		return nil
	}

	if len(sm_key) == 0 {
		key, err := types.CreateStorageKey(sm_meta, "System", "Events", nil, nil)
		if err != nil {
			logger.Error(err)
			return nil
		}
		sm_key = key
	}

	msg := JsonrpcMessage{
		Version: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "state_subscribeStorage",
	}

	keys := [][]string{{sm_key.Hex()}}
	params, err := json.Marshal(keys)
	if err != nil {
		logger.Error(err)
		return nil
	}
	msg.Params = params

	data, _ := json.Marshal(msg)
	return data
}

func forceClose(c *websocket.Conn) {
	
	_ = c.Close()
}

func Init(endpoint string, c *websocket.Conn, channel chan<- Event)  {
	go readMessages(c, channel)

	err := c.WriteMessage(websocket.TextMessage, GetTriggerJson())
	if err != nil {
		forceClose(c)
		return
	}

	logger.Infof("Connected to %s\n", endpoint)
}

func SubscribeToEvents(endpoint string, channel chan<- Event) (*websocket.Conn, error) {
	logger.Infof("Connecting to WS endpoint: %s\n", endpoint)

	c, _, err := websocket.DefaultDialer.Dial(endpoint, nil)
	if err != nil {
		return nil, err
	}

	Init(endpoint, c, channel)

	return c, nil

}