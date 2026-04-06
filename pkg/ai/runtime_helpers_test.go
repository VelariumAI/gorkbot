package ai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

type timeoutNetErr struct{}

func (timeoutNetErr) Error() string   { return "timeout" }
func (timeoutNetErr) Timeout() bool   { return true }
func (timeoutNetErr) Temporary() bool { return true }

func TestStreamGuardTerminalDetection(t *testing.T) {
	sg := NewStreamGuard()
	sg.ObserveContent("partial")
	sg.ObserveLine(`data: {"finish_reason":"stop"}`)
	if !sg.WasComplete() {
		t.Fatalf("expected complete stream after stop finish_reason")
	}
	if sg.PartialContent() != "partial" {
		t.Fatalf("unexpected partial content")
	}

	sg2 := NewStreamGuard()
	sg2.ObserveLine(`data: {"type":"message_stop"}`)
	if !sg2.WasComplete() {
		t.Fatalf("expected complete stream for anthropic message_stop")
	}
}

func TestTokenBucketWaitCancellation(t *testing.T) {
	tb := NewTokenBucket(60)
	// Force empty bucket so Wait must block.
	tb.mu.Lock()
	tb.tokens = 0
	tb.lastFill = time.Now()
	tb.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := tb.Wait(ctx); err == nil {
		t.Fatalf("expected cancellation error")
	}
}

func TestTransportTransientErrorDetection(t *testing.T) {
	if !isTransientNetworkError(io.EOF) {
		t.Fatalf("expected EOF as transient")
	}
	if !isTransientNetworkError(timeoutNetErr{}) {
		t.Fatalf("expected timeout net.Error as transient")
	}
	if !isTransientNetworkError(errors.New("connection reset by peer")) {
		t.Fatalf("expected reset-by-peer as transient")
	}
	if isTransientNetworkError(errors.New("permanent auth error")) {
		t.Fatalf("did not expect arbitrary error as transient")
	}
}

func TestAsNetErrorUnwrap(t *testing.T) {
	var target net.Error
	wrapped := errors.New("outer")
	if asNetError(wrapped, &target) {
		t.Fatalf("did not expect plain error to satisfy net.Error")
	}

	err := fmt.Errorf("wrapped: %w", timeoutNetErr{})
	if !asNetError(err, &target) {
		t.Fatalf("expected wrapped timeout net error")
	}
}
