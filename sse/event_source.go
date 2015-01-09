package sse

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

type BadResponseError struct {
	Response *http.Response
}

func (err BadResponseError) Error() string {
	return fmt.Sprintf("bad response from event source: %s", err.Response.Status)
}

type EventSource struct {
	Client               *http.Client
	CreateRequest        func() *http.Request
	DefaultRetryInterval time.Duration

	currentBody   io.Closer
	currentReader *Reader
	lastEventID   string
	retryInterval time.Duration
	lock          sync.Mutex
}

func (source *EventSource) Next() (Event, error) {
	for {
		err := source.Connect()
		if err != nil {
			return Event{}, err
		}

		event, err := source.currentReader.Next()
		if err == nil {
			source.lastEventID = event.ID

			if event.Retry != 0 {
				source.retryInterval = event.Retry
			}

			return event, nil
		}

		if err == io.EOF {
			return Event{}, err
		}

		source.currentBody = nil
		source.currentReader = nil

		source.waitForRetry()
	}

	panic("unreachable")
}

func (source *EventSource) Close() error {
	source.lock.Lock()
	defer source.lock.Unlock()

	if source.currentBody != nil {
		err := source.currentBody.Close()
		if err != nil {
			return err
		}
	}

	source.currentBody = nil
	source.currentReader = nil

	return nil
}

func (source *EventSource) Connect() error {
	if source.currentReader != nil {
		return nil
	}

	for {
		req := source.CreateRequest()

		req.Header.Set("Last-Event-ID", source.lastEventID)

		res, err := source.Client.Do(req)
		if err != nil {
			source.waitForRetry()
			continue
		}

		switch res.StatusCode {
		case http.StatusOK:
			source.currentReader = NewReader(res.Body)
			source.currentBody = res.Body

			return nil

		// reestablish the connection
		case http.StatusInternalServerError,
			http.StatusBadGateway,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout:
			res.Body.Close()
			source.waitForRetry()
			continue

		// fail the connection
		default:
			res.Body.Close()

			return BadResponseError{
				Response: res,
			}
		}
	}
}

func (source *EventSource) waitForRetry() {
	if source.retryInterval != 0 {
		time.Sleep(source.retryInterval)
	} else if source.DefaultRetryInterval != 0 {
		time.Sleep(source.DefaultRetryInterval)
	} else {
		time.Sleep(time.Second)
	}
}
