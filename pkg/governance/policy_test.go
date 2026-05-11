package governance

import (
	"testing"
	"time"
)

func action(r RiskClass) GovernedAction {
	return GovernedAction{
		ID:         "a1",
		Actor:      "gorkbot",
		Capability: "tool.x",
		ToolName:   "x",
		Parameters: map[string]any{},
		RiskClass:  r,
		CreatedAt:  time.Now(),
	}
}

func TestPolicyOffAllows(t *testing.T) {
	p := DefaultPolicy()
	p.Mode = GOVERNANCE_OFF
	d := p.Evaluate(action(RISK_EXTERNAL_SIDE_EFFECT))
	if !d.Allowed {
		t.Fatal("off mode should allow")
	}
}

func TestPolicyAuditAllowsRiskyWithIssues(t *testing.T) {
	p := DefaultPolicy()
	p.Mode = GOVERNANCE_AUDIT
	d := p.Evaluate(action(RISK_EXTERNAL_SIDE_EFFECT))
	if !d.Allowed {
		t.Fatal("audit mode should allow risky action")
	}
	if len(d.Issues) == 0 {
		t.Fatal("expected audit issues")
	}
}

func TestPolicyEnforceBlocksExternalSideEffect(t *testing.T) {
	p := DefaultPolicy()
	p.Mode = GOVERNANCE_ENFORCE
	d := p.Evaluate(action(RISK_EXTERNAL_SIDE_EFFECT))
	if d.Allowed {
		t.Fatal("expected external side effect blocked")
	}
	if !d.RequiresHuman {
		t.Fatal("expected human requirement")
	}
}

func TestPolicyReadOnlyFastPath(t *testing.T) {
	p := DefaultPolicy()
	p.Mode = GOVERNANCE_FAST
	d := p.Evaluate(action(RISK_READ_ONLY))
	if !d.Allowed {
		t.Fatal("read-only should be allowed")
	}
}

func TestPolicyUnknownBlockedInEnforce(t *testing.T) {
	p := DefaultPolicy()
	p.Mode = GOVERNANCE_ENFORCE
	d := p.Evaluate(action(RISK_UNKNOWN))
	if d.Allowed {
		t.Fatal("unknown should be blocked in enforce")
	}
}

func TestPolicySelfModificationManifestMissingBlockedInEnforce(t *testing.T) {
	p := DefaultPolicy()
	p.Mode = GOVERNANCE_ENFORCE
	a := action(RISK_SELF_MODIFICATION)
	a.ToolName = "create_tool"
	d := p.Evaluate(a)
	if d.Allowed {
		t.Fatal("self-modification should be blocked without manifest")
	}
	if d.ReasonCode != REASON_SELF_MODIFICATION_REQUIRES_MANIFEST {
		t.Fatalf("unexpected reason: %s", d.ReasonCode)
	}
}

func TestPolicySelfModificationWithManifestRequiresApproval(t *testing.T) {
	p := DefaultPolicy()
	p.Mode = GOVERNANCE_ENFORCE
	a := action(RISK_SELF_MODIFICATION)
	a.ToolName = "create_tool"
	a.Parameters = map[string]any{
		"manifest": map[string]any{
			"name":         "x",
			"risk_class":   "RISK_SELF_MODIFICATION",
			"capabilities": []any{"write"},
		},
	}
	d := p.Evaluate(a)
	if d.Allowed {
		t.Fatal("manifested self-modification should still require approval")
	}
	if !d.RequiresHuman {
		t.Fatal("expected human requirement")
	}
}

func TestPolicySelfModificationAuditAllowsWithoutManifest(t *testing.T) {
	p := DefaultPolicy()
	p.Mode = GOVERNANCE_AUDIT
	a := action(RISK_SELF_MODIFICATION)
	a.ToolName = "create_tool"
	d := p.Evaluate(a)
	if !d.Allowed {
		t.Fatal("audit mode should allow missing manifest")
	}
	if len(d.Issues) == 0 {
		t.Fatal("expected audit issue")
	}
}

func TestHasSelfModificationManifestWithJSONString(t *testing.T) {
	a := action(RISK_SELF_MODIFICATION)
	a.Parameters = map[string]any{
		"tool_manifest": `{"name":"x","risk_class":"RISK_SELF_MODIFICATION","capabilities":["write"]}`,
	}
	if !HasSelfModificationManifest(a) {
		t.Fatal("expected manifest detection")
	}
}
