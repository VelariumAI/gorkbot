package ai

import "errors"

var (
	// ErrContextExceeded indicates the prompt exceeded the model's token limit.
	ErrContextExceeded = errors.New("context window exceeded")

	// ErrRateLimit indicates the provider returned a 429 Too Many Requests.
	ErrRateLimit = errors.New("rate limit exceeded (429)")

	// ErrUnauthorized indicates an invalid API key or insufficient permissions.
	ErrUnauthorized = errors.New("unauthorized (invalid api key)")

	// ErrBadGateway indicates an upstream provider issue (502, 503, 504).
	ErrBadGateway = errors.New("bad gateway (502/503/504)")

	// ErrProviderDown indicates the provider is unreachable or returning 500s.
	ErrProviderDown = errors.New("provider API is unreachable")
)
