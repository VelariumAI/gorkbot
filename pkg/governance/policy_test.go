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

func validSelfModManifest() map[string]any {
	return map[string]any{
		"name":             "proposal",
		"artifact_kind":    "dynamic_tool",
		"risk_class":       "moderate",
		"capabilities":     []any{"dynamic.skill.stage"},
		"target_paths":     []any{".gorkbot/staging/tools/proposal.go"},
		"expected_effects": []any{"stage only"},
		"rollback_plan":    "delete staged file",
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
	if d.ReasonCode != REASON_DYNAMIC_MANIFEST_MISSING {
		t.Fatalf("unexpected reason: %s", d.ReasonCode)
	}
}

func TestPolicySelfModificationWithManifestRequiresApprovalForToolRegister(t *testing.T) {
	p := DefaultPolicy()
	p.Mode = GOVERNANCE_ENFORCE
	a := action(RISK_SELF_MODIFICATION)
	a.ToolName = "create_tool"
	m := validSelfModManifest()
	m["capabilities"] = []any{"dynamic.tool.register"}
	a.Parameters = map[string]any{"manifest": m}
	d := p.Evaluate(a)
	if d.Allowed {
		t.Fatal("manifested self-modification should still require approval")
	}
	if !d.RequiresHuman {
		t.Fatal("expected human requirement")
	}
	if d.ReasonCode != REASON_DYNAMIC_CAPABILITY_REQUIRES_APPROVAL {
		t.Fatalf("expected approval reason, got %s", d.ReasonCode)
	}
}

func TestPolicySelfModificationAuthorityBlockedInEnforce(t *testing.T) {
	p := DefaultPolicy()
	p.Mode = GOVERNANCE_ENFORCE
	a := action(RISK_SELF_MODIFICATION)
	a.ToolName = "create_tool"
	m := validSelfModManifest()
	m["metadata"] = map[string]any{"verified": true}
	a.Parameters = map[string]any{"manifest": m}
	d := p.Evaluate(a)
	if d.Allowed {
		t.Fatal("authority claim should be blocked")
	}
	if d.ReasonCode != REASON_DYNAMIC_AUTHORITY_FIELD_FORBIDDEN {
		t.Fatalf("expected authority blocked reason, got %s", d.ReasonCode)
	}
	if d.RequiresHuman {
		t.Fatal("hard block must not request approval")
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
	if d.ReasonCode != REASON_AUDIT_MODE {
		t.Fatalf("expected audit reason, got %s", d.ReasonCode)
	}
}

func TestPolicyCorrectnessRequiresRollbackAndEffectsViaManifest(t *testing.T) {
	p := DefaultPolicy()
	p.Mode = GOVERNANCE_CORRECTNESS
	a := action(RISK_SELF_MODIFICATION)
	a.ToolName = "create_tool"
	a.Parameters = map[string]any{"manifest": map[string]any{
		"name":          "x",
		"artifact_kind": "dynamic_tool",
		"risk_class":    "high",
		"capabilities":  []any{"dynamic.skill.stage"},
		"target_paths":  []any{".gorkbot/staging/tools/x.go"},
	}}
	d := p.Evaluate(a)
	if d.Allowed {
		t.Fatal("correctness mode should block incomplete manifest")
	}
	if d.ReasonCode != REASON_DYNAMIC_MANIFEST_MISSING_REQUIRED_FIELD {
		t.Fatalf("expected required-field reason, got %s", d.ReasonCode)
	}
}

func TestHasSelfModificationManifestWithJSONString(t *testing.T) {
	a := action(RISK_SELF_MODIFICATION)
	a.Parameters = map[string]any{
		"tool_manifest": `{"name":"x","artifact_kind":"dynamic_tool","risk_class":"moderate","capabilities":["dynamic.skill.stage"],"target_paths":[".gorkbot/staging/tools/x.go"],"expected_effects":["stage"],"rollback_plan":"delete"}`,
	}
	if !HasSelfModificationManifest(a) {
		t.Fatal("expected manifest detection")
	}
}
