package evidence

import "errors"

var (
	ErrInvalidStatus     = errors.New("invalid evidence status")
	ErrInvalidKind       = errors.New("invalid evidence kind")
	ErrInvalidPolicy     = errors.New("invalid policy state")
	ErrInvalidRisk       = errors.New("invalid risk")
	ErrInvalidAuthority  = errors.New("invalid authority")
	ErrInvalidDecision   = errors.New("invalid assessment decision")
	ErrInvalidRecord     = errors.New("invalid evidence record")
	ErrInvalidReceipt    = errors.New("invalid evidence receipt")
	ErrInvalidAssessment = errors.New("invalid assessment")
)
