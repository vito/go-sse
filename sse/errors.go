package sse

import "errors"

var ErrReadFromClosedSource = errors.New("read from closed source")
