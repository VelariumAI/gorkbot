package trace

import (
	"context"
	"testing"
)

type captureSink struct {
	events []Event
}

func (c *captureSink) Emit(_ context.Context, e Event) error {
	c.events = append(c.events, e)
	return nil
}

func (c *captureSink) Close() error { return nil }

func TestNoopSink(t *testing.T) {
	var n NoopSink
	if err := n.Emit(context.Background(), Event{}); err != nil {
		t.Fatalf("noop emit failed: %v", err)
	}
	if err := n.Close(); err != nil {
		t.Fatalf("noop close failed: %v", err)
	}
}

func TestEmitAppliesMode(t *testing.T) {
	sink := &captureSink{}
	e := NewEvent("tools", "governance_decision")
	e.Operator = OperatorVerify
	e.Metadata = map[string]string{"token": "abc", "ok": "yes"}
	e.Decision = "allowed"
	e.ArtifactRefs = []Ref{{Kind: "file", Ref: "a"}}

	if err := Emit(context.Background(), sink, ModeMinimal, e); err != nil {
		t.Fatalf("emit failed: %v", err)
	}
	if len(sink.events) != 1 {
		t.Fatalf("expected one event, got %d", len(sink.events))
	}
	got := sink.events[0]
	if got.Decision != "" {
		t.Fatalf("minimal mode should clear decision")
	}
	if len(got.ArtifactRefs) != 0 {
		t.Fatalf("minimal mode should clear refs")
	}
	if got.Metadata["token"] != "[REDACTED]" {
		t.Fatalf("expected metadata redaction")
	}
}
