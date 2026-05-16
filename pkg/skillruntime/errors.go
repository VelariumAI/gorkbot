package skillruntime

import "errors"

var (
	ErrInvalidOperation = errors.New("skillruntime: invalid operation")
	ErrInvalidCandidate = errors.New("skillruntime: invalid candidate")
	ErrInvalidRequest   = errors.New("skillruntime: invalid request")
	ErrInvalidResult    = errors.New("skillruntime: invalid result")
	ErrNotFound         = errors.New("skillruntime: not found")
)
