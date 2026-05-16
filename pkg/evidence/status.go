package evidence

import (
	"fmt"
	"strings"
)

type Status string

const (
	StatusPass         Status = "pass"
	StatusFail         Status = "fail"
	StatusWarn         Status = "warn"
	StatusInconclusive Status = "inconclusive"
	StatusUnavailable  Status = "unavailable"
	StatusSkipped      Status = "skipped"
	StatusInvalid      Status = "invalid"
	StatusUnknown      Status = "unknown"
)

func NormalizeStatus(raw string) Status {
	s := Status(strings.ToLower(strings.TrimSpace(raw)))
	switch s {
	case StatusPass, StatusFail, StatusWarn, StatusInconclusive,
		StatusUnavailable, StatusSkipped, StatusInvalid, StatusUnknown:
		return s
	case "":
		return StatusUnknown
	default:
		return StatusInvalid
	}
}

func (s Status) Valid() bool {
	n := NormalizeStatus(string(s))
	return n != StatusInvalid && n != StatusUnknown
}

func (s Status) Validate() error {
	n := NormalizeStatus(string(s))
	if n == StatusInvalid || n == StatusUnknown {
		return fmt.Errorf("%w: %q", ErrInvalidStatus, s)
	}
	return nil
}
