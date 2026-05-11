package vcseclient

import (
	"errors"
	"fmt"
)

var (
	// ErrTimeout indicates a VCSE network timeout.
	ErrTimeout = errors.New("vcse timeout")
	// ErrUnavailable indicates VCSE endpoint is unavailable.
	ErrUnavailable = errors.New("vcse unavailable")
	// ErrInvalidResponse indicates malformed VCSE response payload.
	ErrInvalidResponse = errors.New("vcse invalid response")
)

// HTTPStatusError represents non-2xx responses.
type HTTPStatusError struct {
	StatusCode int
	Endpoint   string
	Body       string
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("vcse %s returned status %d", e.Endpoint, e.StatusCode)
}
