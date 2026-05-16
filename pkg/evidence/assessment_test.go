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

func TestEvaluateLowRiskPolicyBoundReason(t *testing.T) {
	for _, state := range []PolicyState{PolicyMatched, PolicyEnforced} {
		a := Evaluate(Assessment{
			PolicyState: state,
			Risk:        RiskLow,
			Operation:   "help",
			Authority:   AuthorityPolicyMatch,
		})
		if a.Decision != DecisionAllowLowRisk || a.Status != StatusPass {
			t.Fatalf("state=%s expected allow_low_risk/pass, got %s/%s", state, a.Decision, a.Status)
		}
		if a.ReasonCode != ReasonLowRiskPolicyBound {
			t.Fatalf("state=%s unexpected reason code: %s", state, a.ReasonCode)
		}
	}
}

func TestEvaluateMediumRiskPolicyVariants(t *testing.T) {
	absent := Evaluate(Assessment{
		PolicyState: PolicyNoMatch,
		Risk:        RiskMedium,
		Operation:   "provider_selection",
	})
	if absent.Decision != DecisionAuditOnly || absent.Status != StatusWarn || absent.ReasonCode != ReasonMediumPolicyAbsent {
		t.Fatalf("unexpected medium absent result: %s/%s/%s", absent.Decision, absent.Status, absent.ReasonCode)
	}

	audit := Evaluate(Assessment{
		PolicyState: PolicyAuditOnly,
		Risk:        RiskMedium,
		Operation:   "provider_selection",
	})
	if audit.Decision != DecisionAuditOnly || audit.Status != StatusWarn || audit.ReasonCode != ReasonMediumPolicyAuditOnly {
		t.Fatalf("unexpected medium audit-only result: %s/%s/%s", audit.Decision, audit.Status, audit.ReasonCode)
	}

	for _, state := range []PolicyState{PolicyMatched, PolicyEnforced} {
		bound := Evaluate(Assessment{
			PolicyState: state,
			Risk:        RiskMedium,
			Operation:   "provider_selection",
			Authority:   AuthorityPolicyMatch,
		})
		if bound.Decision != DecisionAuditOnly || bound.Status != StatusWarn || bound.ReasonCode != ReasonMediumPolicyBound {
			t.Fatalf("state=%s unexpected medium policy-bound result: %s/%s/%s", state, bound.Decision, bound.Status, bound.ReasonCode)
		}
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

func TestAssessmentValidateRejectsImpossibleDecisionRiskPairs(t *testing.T) {
	tests := []Assessment{
		{PolicyState: PolicyMatched, Risk: RiskSensitive, Decision: DecisionAllowLowRisk, Authority: AuthorityPolicyMatch},
		{PolicyState: PolicyMatched, Risk: RiskMedium, Decision: DecisionAllowLowRisk, Authority: AuthorityPolicyMatch},
		{PolicyState: PolicyMatched, Risk: RiskUnknown, Decision: DecisionAuditOnly, Authority: AuthorityPolicyMatch},
		{PolicyState: PolicyInvalid, Risk: RiskLow, Decision: DecisionAuditOnly, Authority: AuthorityPolicyMatch},
		{PolicyState: PolicyMatched, Risk: RiskSensitive, Decision: DecisionAuditOnly, Authority: AuthorityNone},
	}
	for i, tc := range tests {
		if err := tc.Validate(); err == nil {
			t.Fatalf("case %d: expected validation error", i)
		}
	}
}
