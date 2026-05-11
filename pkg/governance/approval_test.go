package governance

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestApprovalGranted(t *testing.T) {
	if !ApprovalGranted(ApprovalResult{Decision: APPROVAL_GRANTED, Scope: APPROVAL_ONCE}) {
		t.Fatal("expected granted for once")
	}
	if !ApprovalGranted(ApprovalResult{Decision: APPROVAL_GRANTED, Scope: APPROVAL_SESSION}) {
		t.Fatal("expected granted for session")
	}
	if !ApprovalGranted(ApprovalResult{Decision: APPROVAL_GRANTED, Scope: APPROVAL_ALWAYS}) {
		t.Fatal("expected granted for always")
	}
	if ApprovalGranted(ApprovalResult{Decision: APPROVAL_DENIED, Scope: APPROVAL_ALWAYS}) {
		t.Fatal("did not expect denied as granted")
	}
	if ApprovalGranted(ApprovalResult{Decision: APPROVAL_GRANTED, Scope: APPROVAL_NEVER}) {
		t.Fatal("did not expect never as granted")
	}
}

func TestApprovalJSONRoundtrip(t *testing.T) {
	req := ApprovalRequest{
		ActionID:   "a1",
		ToolName:   "bash",
		Capability: "tool.bash",
		RiskClass:  RISK_PRIVILEGED_BRIDGE,
		ReasonCode: REASON_POLICY_BLOCKED,
		Summary:    "needs approval",
		CreatedAt:  time.Now().UTC(),
	}
	res := ApprovalResult{ActionID: "a1", Decision: APPROVAL_GRANTED, Scope: APPROVAL_SESSION, DurationMS: 12}
	blob, err := json.Marshal(struct {
		Request ApprovalRequest `json:"request"`
		Result  ApprovalResult  `json:"result"`
	}{req, res})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out struct {
		Request ApprovalRequest `json:"request"`
		Result  ApprovalResult  `json:"result"`
	}
	if err := json.Unmarshal(blob, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Request.ActionID != req.ActionID || out.Result.Decision != APPROVAL_GRANTED {
		t.Fatalf("unexpected roundtrip: %#v", out)
	}
}

func TestApprovalHandlerFunc(t *testing.T) {
	h := ApprovalHandlerFunc(func(ctx context.Context, req ApprovalRequest) (ApprovalResult, error) {
		return ApprovalResult{ActionID: req.ActionID, Decision: APPROVAL_GRANTED, Scope: APPROVAL_ONCE}, nil
	})
	res, err := h.RequestApproval(context.Background(), ApprovalRequest{ActionID: "a1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Decision != APPROVAL_GRANTED {
		t.Fatalf("unexpected decision: %#v", res)
	}
}
