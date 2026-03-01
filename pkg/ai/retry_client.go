package ai

import (
	"bytes"
	"io"
	"math"
	"net/http"
	"time"
)

// RetryTransport implements http.RoundTripper with exponential backoff
// for 429 and 5xx errors, and translates status codes to sentinel errors.
type RetryTransport struct {
	Base          http.RoundTripper
	MaxRetries    int
	BaseDelayBase time.Duration
	MaxDelay      time.Duration
}

func NewRetryClient() *http.Client {
	return &http.Client{
		Transport: &RetryTransport{
			Base:          http.DefaultTransport,
			MaxRetries:    4,
			BaseDelayBase: 2 * time.Second,
			MaxDelay:      30 * time.Second,
		},
		// Dynamic timeout is handled per request via context
		Timeout: 0,
	}
}

// NewPingClient returns a plain HTTP client suited for key validation.
// Rules:
//   - Hard 5-second total timeout (connection + TLS + read combined).
//   - No retry logic — a single attempt is sufficient; auth failures should
//     surface immediately, and network failures should fail fast.
//   - No custom transport so context cancellation is fully respected.
func NewPingClient() *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
	}
}

// NewFetchClient returns an HTTP client for polling model lists.
// Stricter than the full retry client: 8-second timeout, one retry max.
func NewFetchClient() *http.Client {
	return &http.Client{
		Transport: &RetryTransport{
			Base:          http.DefaultTransport,
			MaxRetries:    1,
			BaseDelayBase: 1 * time.Second,
			MaxDelay:      3 * time.Second,
		},
		Timeout: 8 * time.Second,
	}
}

func (t *RetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var bodyBytes []byte
	var err error

	// If request has a body, read it so we can re-create it on retries
	if req.Body != nil {
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		req.Body.Close()
	}

	var lastResp *http.Response
	var lastErr error

	for attempt := 0; attempt <= t.MaxRetries; attempt++ {
		// Clone request for retry
		reqClone := req.Clone(req.Context())
		if bodyBytes != nil {
			reqClone.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}

		base := t.Base
		if base == nil {
			base = http.DefaultTransport
		}

		resp, err := base.RoundTrip(reqClone)
		if err != nil {
			lastErr = err
			// Network error, maybe retry
		} else {
			lastResp = resp
			lastErr = nil

			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return resp, nil // Success
			}

			// Handle rate limits and server errors with backoff
			if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
				// We will retry
				// Must close response body to reuse connection
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			} else {
				// 4xx errors (except 429) should not be retried
				return resp, t.translateStatusCode(resp.StatusCode)
			}
		}

		if attempt == t.MaxRetries {
			break
		}

		// Calculate backoff
		delay := t.BaseDelayBase * time.Duration(math.Pow(2, float64(attempt)))
		if delay > t.MaxDelay {
			delay = t.MaxDelay
		}

		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(delay):
			// Proceed to next attempt
		}
	}

	if lastResp != nil {
		return lastResp, t.translateStatusCode(lastResp.StatusCode)
	}

	return nil, lastErr
}

func (t *RetryTransport) translateStatusCode(code int) error {
	switch code {
	case 401, 403:
		return ErrUnauthorized
	case 413, 400: // 400 is sometimes context length in OpenAI API
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
