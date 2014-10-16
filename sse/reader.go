package sse

import (
	"bufio"
	"io"
	"strings"
)

type Event struct {
	ID   string
	Name string
	Data []byte
}

type Reader struct {
	lastID string

	buf *bufio.Reader
}

func NewReader(source io.Reader) *Reader {
	return &Reader{
		buf: bufio.NewReader(source),
	}
}

func (reader *Reader) Next() (Event, error) {
	var event Event

	// event ID defaults to last ID per the spec
	event.ID = reader.lastID

	// if an empty id is explicitly given, it sets the value and resets the last
	// id; track its presence with a bool to distinguish between zero-value
	idPresent := false

	for {
		line, err := reader.buf.ReadString('\n')
		if err != nil {
			return Event{}, err
		}

		// trim trailing \n
		line = line[0 : len(line)-1]

		// empty line; dispatch event
		if len(line) == 0 {
			if len(event.Data) == 0 {
				// event had no data; skip it per the spec
				continue
			}

			if idPresent {
				// record last ID
				reader.lastID = event.ID
			}

			// trim terminating linebreak
			event.Data = event.Data[0 : len(event.Data)-1]

			// dispatch event
			return event, nil
		}

		if line[0] == ':' {
			// comment; skip
			continue
		}

		var field, value string

		segments := strings.SplitN(line, ":", 2)
		if len(segments) == 1 {
			// line with no colon is just the field, with empty value
			field = segments[0]
		} else {
			field = segments[0]
			value = segments[1]
		}

		if len(value) > 0 {
			// trim only a single leading space
			if value[0] == ' ' {
				value = value[1:]
			}
		}

		switch field {
		case "id":
			idPresent = true
			event.ID = value
		case "event":
			event.Name = value
		case "data":
			event.Data = append(event.Data, []byte(value+"\n")...)
		}
	}
}
