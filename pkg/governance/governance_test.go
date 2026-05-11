package governance

import (
	"encoding/json"
	"testing"
	"time"
)

func TestGovernanceJSONRoundTrip(t *testing.T) {
	a := GovernedAction{
		ID:         "a1",
		Actor:      "gorkbot",
		Capability: "tool.read_file",
		ToolName:   "read_file",
		Parameters: map[string]any{"path": "x"},
		RiskClass:  RISK_READ_ONLY,
		CreatedAt:  time.Now().UTC(),
	}
	d := GovernanceDecision{
		ActionID:    "a1",
		Allowed:     true,
		Mode:        GOVERNANCE_AUDIT,
		FinalStatus: GOVERNANCE_AUDIT_ONLY,
		ReasonCode:  REASON_AUDIT_MODE,
		RiskClass:   RISK_READ_ONLY,
		DurationMS:  12,
	}
	blob, err := json.Marshal(struct {
		Action   GovernedAction     `json:"action"`
		Decision GovernanceDecision `json:"decision"`
	}{a, d})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out struct {
		Action   GovernedAction     `json:"action"`
		Decision GovernanceDecision `json:"decision"`
	}
	if err := json.Unmarshal(blob, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Action.ID != a.ID || out.Decision.ActionID != d.ActionID {
		t.Fatalf("unexpected roundtrip: %#v", out)
	}
}

func TestGovernanceConstantsStable(t *testing.T) {
	if GOVERNANCE_OFF != "GOVERNANCE_OFF" {
		t.Fatalf("unexpected constant value: %s", GOVERNANCE_OFF)
	}
	if REASON_POLICY_ALLOWED != "REASON_POLICY_ALLOWED" {
		t.Fatalf("unexpected reason constant: %s", REASON_POLICY_ALLOWED)
	}
}
