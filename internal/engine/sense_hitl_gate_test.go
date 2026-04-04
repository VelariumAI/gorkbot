package engine

import (
	"context"
	"testing"

	"github.com/velariumai/gorkbot/pkg/hitl"
	"github.com/velariumai/gorkbot/pkg/tools"
)

func TestRequestHITLApproval_AutoApproveHighConfidence(t *testing.T) {
	req := HITLRequest{
		RiskLevel:       hitl.RiskMedium,
		ConfidenceScore: 95,
		Precedent:       2,
	}

	approved, notes := RequestHITLApproval(context.Background(), nil, req, nil)
	if !approved {
		t.Fatal("expected auto-approval")
	}
	if notes == "" {
		t.Fatal("expected auto-approval note")
	}
}

func TestRequestHITLApproval_CallbackReject(t *testing.T) {
	req := HITLRequest{
		RiskLevel:       hitl.RiskHigh,
		ConfidenceScore: 10,
		Precedent:       0,
	}

	approved, notes := RequestHITLApproval(context.Background(), func(r HITLRequest) HITLDecision {
		return HITLDecision{Approval: HITLRejected, Notes: "reject"}
	}, req, nil)
	if approved {
		t.Fatal("expected rejection")
	}
	if notes != "reject" {
		t.Fatalf("unexpected notes: %q", notes)
	}
}

func TestGateToolExecution_CreateToolForcesApproval(t *testing.T) {
	orch := &Orchestrator{}
	guard := NewHITLGuard()
	guard.Enabled = true

	capturedForced := false
	approved, notes := orch.GateToolExecution(
		context.Background(),
		tools.ToolRequest{
			ToolName:   "create_tool",
			Parameters: map[string]interface{}{"tool_name": "x"},
		},
		guard,
		func(req HITLRequest) HITLDecision {
			capturedForced = req.ForcedApproval
			return HITLDecision{Approval: HITLRejected, Notes: "manual deny"}
		},
		"reasoning",
	)
	if approved {
		t.Fatal("expected rejection for callback decision")
	}
	if notes != "manual deny" {
		t.Fatalf("unexpected notes: %q", notes)
	}
	if !capturedForced {
		t.Fatal("expected forced approval flag to be set for create_tool")
	}
}

func TestGateToolExecution_NonHighStakesBypassesHITL(t *testing.T) {
	orch := &Orchestrator{}
	guard := NewHITLGuard()
	guard.Enabled = true

	approved, notes := orch.GateToolExecution(
		context.Background(),
		tools.ToolRequest{
			ToolName:   "read_file",
			Parameters: map[string]interface{}{"path": "README.md"},
		},
		guard,
		func(req HITLRequest) HITLDecision {
			return HITLDecision{Approval: HITLRejected, Notes: "should not be called"}
		},
		"reasoning",
	)
	if !approved {
		t.Fatalf("expected bypass approval, got notes=%q", notes)
	}
}
