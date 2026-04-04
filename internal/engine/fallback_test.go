package engine

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/velariumai/gorkbot/pkg/ai"
)

type fakeNetErr struct{ timeout bool }

func (e fakeNetErr) Error() string   { return "net err" }
func (e fakeNetErr) Timeout() bool   { return e.timeout }
func (e fakeNetErr) Temporary() bool { return false }

type wrappedErr struct{ inner error }

func (e wrappedErr) Error() string { return "wrapped: " + e.inner.Error() }
func (e wrappedErr) Unwrap() error { return e.inner }

func TestIsProviderOutage_SentinelAndMessageAndNetErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "unauthorized", err: ai.ErrUnauthorized, want: true},
		{name: "provider down", err: ai.ErrProviderDown, want: true},
		{name: "net timeout", err: fakeNetErr{timeout: true}, want: true},
		{name: "wrapped net timeout", err: wrappedErr{inner: fakeNetErr{timeout: true}}, want: true},
		{name: "status code string", err: errors.New("request failed status 503"), want: true},
		{name: "transport pattern", err: errors.New("tls: bad record mac"), want: true},
		{name: "quota string", err: errors.New("insufficient_quota"), want: true},
		{name: "generic", err: errors.New("oops"), want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isProviderOutage(tc.err); got != tc.want {
				t.Fatalf("isProviderOutage(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestIsContextOverflowErr(t *testing.T) {
	if !isContextOverflowErr("Maximum context window exceeded") {
		t.Fatalf("expected context overflow signal")
	}
	if isContextOverflowErr("random failure") {
		t.Fatalf("did not expect context overflow signal")
	}
}

func TestAsNetErr(t *testing.T) {
	var got net.Error
	if !asNetErr(fakeNetErr{timeout: true}, &got) {
		t.Fatalf("expected direct net.Error to match")
	}
	if !got.Timeout() {
		t.Fatalf("expected timeout net error")
	}
	if asNetErr(errors.New("x"), &got) {
		t.Fatalf("expected plain error not to match")
	}
}

func TestBestAndSecondModelFor(t *testing.T) {
	if got := bestModelFor("unknown-provider"); got != "" {
		t.Fatalf("expected empty best model for unknown provider, got %q", got)
	}
	if got := bestModelFor("openai"); got == "" {
		t.Fatalf("expected non-empty best model for openai")
	}
	if got := secondModelFor("openai"); got == "" {
		t.Fatalf("expected non-empty second model for openai")
	}
}

func TestRunProviderCascade(t *testing.T) {
	orch := &Orchestrator{}
	retryable, msg := orch.RunProviderCascade(context.Background(), "xai")
	if retryable {
		t.Fatalf("expected non-retryable when no coordinator")
	}
	if !strings.Contains(msg, "not available") {
		t.Fatalf("unexpected message: %s", msg)
	}
}

func TestAsNetErr_WithWrappedNonNetError(t *testing.T) {
	var got net.Error
	err := fmt.Errorf("outer: %w", wrappedErr{inner: errors.New("inner")})
	if asNetErr(err, &got) {
		t.Fatalf("expected false for wrapped non-net error")
	}
}
