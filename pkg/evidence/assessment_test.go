package evidence

import "testing"

func TestEvaluateLowRiskAbsentPolicyRequiresExplicitLowRisk(t *testing.T) {
	allowed := Evaluate(Assessment{
		PolicyState:     PolicyNoMatch,
		Risk:            RiskLow,
		Operation:       "help",
		ExplicitLowRisk: true,
		Authority:       AuthorityNone,
	})
	if allowed.Decision != DecisionAllowLowRisk || allowed.Status != StatusPass {
		t.Fatalf("expected allow_low_risk/pass, got %s/%s", allowed.Decision, allowed.Status)
	}
	if allowed.ReasonCode != ReasonLowRiskExplicitAbsent {
		t.Fatalf("unexpected reason code: %s", allowed.ReasonCode)
	}

	audit := Evaluate(Assessment{
		PolicyState:     PolicyNoMatch,
		Risk:            RiskLow,
		Operation:       "help",
		ExplicitLowRisk: false,
		Authority:       AuthorityNone,
	})
	if audit.Decision != DecisionAuditOnly {
		t.Fatalf("expected audit_only, got %s", audit.Decision)
	}
}

func TestEvaluateSensitiveAbsentPolicy(t *testing.T) {
	a := Evaluate(Assessment{
		PolicyState: PolicyNoMatch,
		Risk:        RiskSensitive,
		Operation:   string(SensitiveShellExecution),
		Authority:   AuthorityNone,
	})
	if a.Decision != DecisionRequireApproval {
		t.Fatalf("expected require_approval, got %s", a.Decision)
	}
	if a.ReasonCode != ReasonPolicyAbsentSensitive {
		t.Fatalf("unexpected reason code: %s", a.ReasonCode)
	}
}

func TestPolicyAuditOnlyDoesNotAuthorizeSensitive(t *testing.T) {
	a := Evaluate(Assessment{
		PolicyState: PolicyAuditOnly,
		Risk:        RiskSensitive,
		Operation:   string(SensitiveNetworkEgress),
		Authority:   AuthorityPolicyMatch,
	})
	if a.Decision != DecisionRequireApproval {
		t.Fatalf("expected require_approval under audit_only for sensitive risk, got %s", a.Decision)
	}
}

func TestUnknownRiskDenied(t *testing.T) {
	a := Evaluate(Assessment{
		PolicyState: PolicyMatched,
		Risk:        RiskUnknown,
		Operation:   "mystery",
	})
	if a.Decision != DecisionDenyInvalid || a.Status != StatusInvalid {
		t.Fatalf("expected deny_invalid/invalid, got %s/%s", a.Decision, a.Status)
	}
	if a.ReasonCode != ReasonUnknownRisk {
		t.Fatalf("unexpected reason code: %s", a.ReasonCode)
	}
}

func TestHardInvariantAuthorityBehavior(t *testing.T) {
	a := Evaluate(Assessment{
		PolicyState: PolicyOff,
		Risk:        RiskSensitive,
		Operation:   string(SensitiveReleasePublish),
		Authority:   AuthorityHardInvariant,
	})
	if a.Decision != DecisionDenySensitive {
		t.Fatalf("expected deny_sensitive, got %s", a.Decision)
	}
	if a.ReasonCode != ReasonHardInvariant {
		t.Fatalf("unexpected reason code: %s", a.ReasonCode)
	}
}

func TestAssessmentMalformedInputNoPanic(t *testing.T) {
	_ = Evaluate(Assessment{})
	if err := (Assessment{}).Validate(); err != nil {
		// Acceptable. Assert only no panic path.
	}
}
