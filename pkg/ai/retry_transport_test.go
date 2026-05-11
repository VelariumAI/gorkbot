package ai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type seqRoundTripper struct {
	steps []func(*http.Request) (*http.Response, error)
	i     int
}

func (s *seqRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if s.i >= len(s.steps) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
	}
	fn := s.steps[s.i]
	s.i++
	return fn(req)
}

func TestRetryTransportRetriesOn429ThenSucceeds(t *testing.T) {
	base := &seqRoundTripper{
		steps: []func(*http.Request) (*http.Response, error){
			func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusTooManyRequests,
					Header:     http.Header{"Retry-After": []string{"1"}},
					Body:       io.NopCloser(strings.NewReader("rate limited")),
				}, nil
			},
			func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("ok")),
				}, nil
			},
		},
	}

	rt := &RetryTransport{
		Base:          base,
		MaxRetries:    2,
		BaseDelayBase: time.Millisecond,
		MaxDelay:      2 * time.Millisecond,
	}

	req, _ := http.NewRequest("GET", "https://example.com", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("roundtrip failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected eventual success")
	}
	if base.i < 2 {
		t.Fatalf("expected retry to occur")
	}
}

func TestRetryTransportRetriesTransientError(t *testing.T) {
	base := &seqRoundTripper{
		steps: []func(*http.Request) (*http.Response, error){
			func(*http.Request) (*http.Response, error) { return nil, io.EOF },
			func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("ok")),
				}, nil
			},
		},
	}
	rt := &RetryTransport{
		Base:          base,
		MaxRetries:    2,
		BaseDelayBase: time.Millisecond,
		MaxDelay:      2 * time.Millisecond,
	}
	req, _ := http.NewRequest("GET", "https://example.com", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected transient retry success, resp=%v err=%v", resp, err)
	}
}

func TestRetryTransportNonRetryableStatusReturnsMappedError(t *testing.T) {
	base := &seqRoundTripper{
		steps: []func(*http.Request) (*http.Response, error){
			func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusUnauthorized,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("unauth")),
				}, nil
			},
		},
	}
	rt := &RetryTransport{Base: base, MaxRetries: 1, BaseDelayBase: time.Millisecond, MaxDelay: 2 * time.Millisecond}
	req, _ := http.NewRequest("GET", "https://example.com", nil)
	_, err := rt.RoundTrip(req)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected mapped unauthorized error, got %v", err)
	}
}

func TestRetryTransportContextCancelDuringBackoff(t *testing.T) {
	base := &seqRoundTripper{
		steps: []func(*http.Request) (*http.Response, error){
			func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusTooManyRequests,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("rate limited")),
				}, nil
			},
		},
	}
	rt := &RetryTransport{Base: base, MaxRetries: 2, BaseDelayBase: 50 * time.Millisecond, MaxDelay: 50 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://example.com", nil)
	cancel()
	_, err := rt.RoundTrip(req)
	if err == nil {
		t.Fatalf("expected context cancellation error")
	}
}
