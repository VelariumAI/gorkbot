package tools

import (
	"context"
	"testing"
	"time"

	"github.com/velariumai/gorkbot/pkg/governance"
	"github.com/velariumai/gorkbot/pkg/trace"
)

type governanceCaptureSink struct {
	events []trace.Event
}

func (c *governanceCaptureSink) Emit(_ context.Context, e trace.Event) error {
	c.events = append(c.events, e)
	return nil
}

func (c *governanceCaptureSink) Close() error { return nil }

func TestEmitGovernanceTrace(t *testing.T) {
	sink := &governanceCaptureSink{}
	action := governance.GovernedAction{
		ID:        "a1",
		ToolName:  "bash",
		Workspace: "/tmp/work",
		CreatedAt: time.Now().UTC(),
	}
	decision := governance.GovernanceDecision{
		ActionID:    "a1",
		Allowed:     false,
		Mode:        governance.GOVERNANCE_ENFORCE,
		FinalStatus: governance.GOVERNANCE_BLOCKED,
		ReasonCode:  governance.REASON_POLICY_BLOCKED,
	}
	emitGovernanceTrace(context.Background(), sink, trace.ModeAudit, action, decision, "bash", nil)
	if len(sink.events) != 1 {
		t.Fatalf("expected one trace event, got %d", len(sink.events))
	}
	if sink.events[0].EventKind != "governance_decision" {
		t.Fatalf("unexpected event kind: %s", sink.events[0].EventKind)
	}
}
