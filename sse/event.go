package sse

import (
	"bytes"
	"fmt"
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
