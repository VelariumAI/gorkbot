package ai

import (
	"bytes"
	"io"
	"math"
	"net/http"
	"time"
)

// RetryTransport implements http.RoundTripper with:
//   - Exponential backoff for HTTP 429 and 5xx responses
//   - Transparent retry on transient network errors (EOF, TLS MAC, RST, etc.)
//     using isTransientNetworkError() from transport.go
//   - Automatic status-code → sentinel-error translation
//
// The Base transport must be a *NewHardenedTransport() instance so TCP
// keep-alives and TLS timeouts are enforced at the connection level.
type RetryTransport struct {
	Base          http.RoundTripper
	MaxRetries    int
	BaseDelayBase time.Duration
	MaxDelay      time.Duration
}

// NewRetryClient returns the standard AI-streaming HTTP client.
// Uses NewHardenedTransport() as the base — replaces http.DefaultTransport.
func NewRetryClient() *http.Client {
	return &http.Client{
		Transport: &RetryTransport{
			Base:          NewHardenedTransport(),
			MaxRetries:    4,
			BaseDelayBase: 2 * time.Second,
			MaxDelay:      30 * time.Second,
		},
		// Per-request context controls the total deadline; no client-level timeout.
		Timeout: 0,
	}
}

// NewPingClient returns a lightweight client for API-key validation.
//   - Hardened transport (keep-alives, TLS timeout) for correctness.
//   - Hard 8-second wall-clock timeout — auth failures must surface fast.
//   - No retry — a single probe is sufficient.
func NewPingClient() *http.Client {
	return &http.Client{
		Transport: NewHardenedTransport(),
		Timeout:   8 * time.Second,
	}
}

// NewFetchClient returns an HTTP client for polling provider model lists.
// Stricter than the full retry client: 10-second timeout, one retry max.
func NewFetchClient() *http.Client {
	return &http.Client{
		Transport: &RetryTransport{
			Base:          NewHardenedTransport(),
			MaxRetries:    1,
			BaseDelayBase: 1 * time.Second,
			MaxDelay:      3 * time.Second,
		},
		Timeout: 10 * time.Second,
	}
}

// RoundTrip executes the request with retry logic for:
//  1. Transient network errors (EOF, TLS MAC, RST) → fresh-connection retry
//  2. HTTP 429 Too Many Requests → backoff retry
//  3. HTTP 5xx Server Error → backoff retry
//
// Non-retryable 4xx errors (except 429) are returned immediately with the
// appropriate sentinel error from translateStatusCode.
func (t *RetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var bodyBytes []byte
	var readErr error

	// Buffer the request body once so it can be replayed on retries.
	if req.Body != nil {
		bodyBytes, readErr = io.ReadAll(req.Body)
		if readErr != nil {
			return nil, readErr
		}
		req.Body.Close()
	}

	base := t.Base
	if base == nil {
		base = NewHardenedTransport()
	}

	var lastResp *http.Response
	var lastErr error

	for attempt := 0; attempt <= t.MaxRetries; attempt++ {
		// Always clone the request so headers and context are correct.
		reqClone := req.Clone(req.Context())
		if bodyBytes != nil {
			reqClone.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}

		resp, err := base.RoundTrip(reqClone)
		if err != nil {
			lastErr = err
			// Transient network errors (dropped connection, TLS corruption
			// after cell handoff, RST after NAT eviction) → retry with a
			// fresh connection. The hardened transport will open a new TCP
			// socket automatically on the next RoundTrip call.
			if isTransientNetworkError(err) {
				goto backoff
			}
			// Non-transient error (DNS failure, cert error, etc.) → give up.
			return nil, err
		}

		lastResp = resp
		lastErr = nil

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp, nil // Success — no retry needed.
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			// Drain body to allow connection reuse, then retry with backoff.
			io.Copy(io.Discard, resp.Body) //nolint:errcheck
			resp.Body.Close()
			goto backoff
		}

		// Non-retryable 4xx (401, 402, 403, 400, 413 …)
		return resp, t.translateStatusCode(resp.StatusCode)

	backoff:
		if attempt == t.MaxRetries {
			break
		}

		delay := t.BaseDelayBase * time.Duration(math.Pow(2, float64(attempt)))
		if delay > t.MaxDelay {
			delay = t.MaxDelay
		}

		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(delay):
		}
	}

	// All retries exhausted.
	if lastResp != nil {
		return lastResp, t.translateStatusCode(lastResp.StatusCode)
	}
	return nil, lastErr
}

func (t *RetryTransport) translateStatusCode(code int) error {
	switch code {
	case 401, 403:
		return ErrUnauthorized
	case 402:
		return ErrNoCredits
	case 413, 400:
		return ErrContextExceeded
	case 429:
		return ErrRateLimit
	case 502, 503, 504:
		return ErrBadGateway
	case 500:
		return ErrProviderDown
	default:
		return nil
	}
}
