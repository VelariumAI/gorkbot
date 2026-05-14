package selfmod

import (
	"context"
	"testing"

	"github.com/velariumai/gorkbot/pkg/trace"
)

type captureTraceSink struct {
	events []trace.Event
}

func (c *captureTraceSink) Emit(_ context.Context, e trace.Event) error {
	c.events = append(c.events, e)
	return nil
}

func (c *captureTraceSink) Close() error { return nil }

func TestValidateDynamicProposalEmitsTrace(t *testing.T) {
	sink := &captureTraceSink{}
	SetTraceSink(sink, trace.ModeAudit)
	t.Cleanup(func() { SetTraceSink(trace.NoopSink{}, trace.ModeOff) })

	res := ValidateDynamicProposal(ValidateInput{OperationID: "op1", Parameters: map[string]any{}})
	if res.Allowed {
		t.Fatalf("expected blocked decision for missing manifest")
	}
	if len(sink.events) == 0 {
		t.Fatalf("expected trace event")
	}
	if sink.events[0].Component != "selfmod" {
		t.Fatalf("unexpected component: %+v", sink.events[0])
	}
}
