package harness

import "errors"

var (
	ErrInvalidArtifact         = errors.New("harness: invalid artifact")
	ErrInvalidAssertion        = errors.New("harness: invalid assertion")
	ErrDuplicateAssertion      = errors.New("harness: duplicate assertion")
	ErrUnsupportedAssertion    = errors.New("harness: unsupported assertion")
	ErrArtifactTooLarge        = errors.New("harness: artifact too large")
	ErrTooManyAssertions       = errors.New("harness: too many assertions")
	ErrInvalidHarnessDocument  = errors.New("harness: invalid harness document")
	ErrHarnessRuntimeDisabled  = errors.New("harness: runtime disabled")
	ErrHarnessAuditUnavailable = errors.New("harness: audit unavailable")
	ErrHarnessAuditFailed      = errors.New("harness: audit failed")
)
