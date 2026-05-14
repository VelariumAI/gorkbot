package router

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/velariumai/gorkbot/pkg/trace"
)

type feedbackCaptureSink struct {
	events []trace.Event
}

func (c *feedbackCaptureSink) Emit(_ context.Context, e trace.Event) error {
	c.events = append(c.events, e)
	return nil
}

func (c *feedbackCaptureSink) Close() error { return nil }

func TestFeedbackManagerEmitsTrace(t *testing.T) {
	fm := NewFeedbackManager("", slog.New(slog.NewTextHandler(io.Discard, nil)))
	sink := &feedbackCaptureSink{}
	fm.SetTraceSink(sink, trace.ModeAudit)
	fm.RecordOutcome(QueryCategoryCoding, "model-x", 0.75, true)
	if len(sink.events) != 1 {
		t.Fatalf("expected one trace event, got %d", len(sink.events))
	}
	if sink.events[0].EventKind != "provider_feedback" {
		t.Fatalf("unexpected event kind: %s", sink.events[0].EventKind)
	}
}
