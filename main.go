package main

import (
	"net/http"

	"time"
	"fmt"
	"github.com/pkg/errors"

	"github.com/labstack/echo/v4"
	subscriber "github.com/ubinix-warun/offchain-worker/subscriber"


)

func offchain_worker() {

	endpoint := "ws://localhost:9944/"

	if err := subscriber.Test(endpoint); err != nil {
		// return nil, errors.Wrap(err, "Failed testing subscriber")
		fmt.Println(errors.Wrap(err, "Failed testing subscriber"))
		return
	}

	events := make(chan subscriber.Event)

	_, err := subscriber.SubscribeToEvents(endpoint, events)
	if err != nil {
		// return err
		fmt.Println(err)
		return 
	}

	go func() {
		
		// sync up before sending the first job run trigger.
		time.Sleep(1 * time.Second)

		for {
			_, ok := <-events
			if !ok {
				return
			}
			go func() {
				
				fmt.Println("GOT SSS")

				// err := as.Node.TriggerJob(as.Subscription.Job, event)
				// if err != nil {
				// 	logger.Error("Failed sending job run trigger: ", err)
				// }
			}()
		}
	}()

	// return nil
	return
}

func main() {

	go offchain_worker()

	e := echo.New()
	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "Hello, World!")
	})
	e.Logger.Fatal(e.Start(":1323"))
}
