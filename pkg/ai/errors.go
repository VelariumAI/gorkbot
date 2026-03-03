package ai

import (
	"errors"
	"fmt"
)

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

	// ErrNoCredits indicates the account has no remaining credits (HTTP 402).
	ErrNoCredits = errors.New("no credits / payment required (402)")
)

// MapStatusError converts an HTTP status code and response body to the
// appropriate sentinel error so callers can use errors.Is() for failover.
func MapStatusError(code int, body []byte) error {
	switch code {
	case 401, 403:
		return fmt.Errorf("%w: %s", ErrUnauthorized, string(body))
	case 402:
		return fmt.Errorf("%w: %s", ErrNoCredits, string(body))
	case 429:
		return fmt.Errorf("%w: %s", ErrRateLimit, string(body))
	case 500:
		return fmt.Errorf("%w: %s", ErrProviderDown, string(body))
	case 502, 503, 504:
		return fmt.Errorf("%w: %s", ErrBadGateway, string(body))
	default:
		return fmt.Errorf("API error (status %d): %s", code, string(body))
	}
}
