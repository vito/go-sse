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

// EventSource behaves like the EventSource interface from the Server-Sent
// Events spec implemented in many browsers.  See
// http://www.w3.org/TR/eventsource/#the-eventsource-interface for details.
//
// To use, optionally call Connect(), and then call Next(). If Next() is called
// prior to Connect(), it will connect for you.
//
// Next() is often called asynchronously in a loop so that the event source can
// be closed. Next() will block on reading from the server.
//
// If Close() is called while reading an event, Next() will return early, and
// subsequent calls to Next() will return early. To read new events, Connect()
// must be called.
//
// If an EOF is received, Next() returns io.EOF, and subsequent calls to Next()
// will return early. To read new events, Connect() must be called.
type EventSource struct {
	Client               *http.Client
	CreateRequest        func() *http.Request
	DefaultRetryInterval time.Duration

	currentReadCloser *ReadCloser
	lastEventID       string
	retryInterval     time.Duration
	lock              sync.Mutex
	lastEventIDLock   sync.Mutex
	closed            bool
}

func (source *EventSource) Next() (Event, error) {
	if source.closed {
		return Event{}, ErrReadFromClosedSource
	}

	for {
		err := source.Connect()
		if err != nil {
			return Event{}, err
		}

		event, err := source.currentReadCloser.Next()
		if err == nil {
			source.lastEventIDLock.Lock()
			source.lastEventID = event.ID
			source.lastEventIDLock.Unlock()

			if event.Retry != 0 {
				source.retryInterval = event.Retry
			}

			return event, nil
		}

		if err == io.EOF {
			_ = source.Close()
			return Event{}, err
		}

		source.lock.Lock()
		if source.closed {
			source.lock.Unlock()
			return Event{}, ErrReadFromClosedSource
		}
		source.lock.Unlock()

		source.currentReadCloser = nil

		source.waitForRetry()
	}

	panic("unreachable")
}

func (source *EventSource) Close() error {
	source.lock.Lock()
	defer source.lock.Unlock()
	defer func() { source.currentReadCloser = nil }()

	source.closed = true

	if source.currentReadCloser != nil {
		err := source.currentReadCloser.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

func (source *EventSource) Connect() error {
	source.lock.Lock()
	if source.currentReadCloser != nil {
		source.lock.Unlock()
		return nil
	}
	source.lock.Unlock()

	for {
		source.lock.Lock()
		req := source.CreateRequest()

		source.lastEventIDLock.Lock()
		req.Header.Set("Last-Event-ID", source.lastEventID)
		source.lastEventIDLock.Unlock()

		res, err := source.Client.Do(req)
		if err != nil {
			source.lock.Unlock()
			source.waitForRetry()
			continue
		}

		switch res.StatusCode {
		case http.StatusOK:
			source.currentReadCloser = NewReadCloser(res.Body)
			source.closed = false
			source.lock.Unlock()
			return nil

		// reestablish the connection
		case http.StatusInternalServerError,
			http.StatusBadGateway,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout:
			res.Body.Close()
			source.lock.Unlock()
			source.waitForRetry()
			continue

		// fail the connection
		default:
			source.lock.Unlock()
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
