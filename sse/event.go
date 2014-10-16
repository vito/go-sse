package sse

import (
	"bytes"
	"fmt"
	"io"
)

type Event struct {
	ID   string
	Name string
	Data []byte
}

func (event Event) Encode() string {
	enc := fmt.Sprintf("id: %s\nevent: %s\n", event.ID, event.Name)

	for _, line := range bytes.Split(event.Data, []byte("\n")) {
		if len(line) == 0 {
			enc += "data\n"
		} else {
			enc += fmt.Sprintf("data: %s\n", line)
		}
	}

	enc += "\n"

	return enc
}

func (event Event) Write(destination io.Writer) error {
	_, err := fmt.Fprintf(destination, "id: %s\n", event.ID)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(destination, "event: %s\n", event.Name)
	if err != nil {
		return err
	}

	for _, line := range bytes.Split(event.Data, []byte("\n")) {
		var err error

		if len(line) == 0 {
			_, err = fmt.Fprintf(destination, "data\n")
		} else {
			_, err = fmt.Fprintf(destination, "data: %s\n", line)
		}

		if err != nil {
			return err
		}
	}

	_, err = fmt.Fprintf(destination, "\n")
	return err
}
