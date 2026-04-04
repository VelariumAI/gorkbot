package sessions

import "errors"

var (
	ErrSessionNotFound    = errors.New("session not found")
	ErrSessionExpired     = errors.New("session has expired")
	ErrBudgetExhausted    = errors.New("session budget exhausted")
	ErrMaxSessionsExceeded = errors.New("max sessions per user exceeded")
	ErrProviderNotAllowed  = errors.New("provider not in whitelist")
	ErrInvalidSessionID    = errors.New("invalid session ID")
	ErrSessionAlreadyExists = errors.New("session already exists")
)
