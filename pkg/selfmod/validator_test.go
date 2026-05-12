package selfmod

import "testing"

func validManifestMap() map[string]any {
	return map[string]any{
		"name":             "proposal",
		"artifact_kind":    "dynamic_tool",
		"risk_class":       "moderate",
		"capabilities":     []any{"dynamic.skill.stage"},
		"target_paths":     []any{".gorkbot/staging/tools/proposal.go"},
		"expected_effects": []any{"stage only"},
		"rollback_plan":    "remove staged file",
	}
}

func TestValidatorAuthorityTopLevelBlocked(t *testing.T) {
	m := validManifestMap()
	m["verified"] = true
	res := ValidateDynamicProposal(ValidateInput{Parameters: map[string]any{"manifest": m}, Mode: "GOVERNANCE_ENFORCE"})
	if res.Allowed || res.ReasonCode != REASON_DYNAMIC_AUTHORITY_FIELD_FORBIDDEN {
		t.Fatalf("expected authority block, got %+v", res)
	}
}

func TestValidatorAuthorityNestedBlocked(t *testing.T) {
	m := validManifestMap()
	m["vcse"] = map[string]any{"certified": true}
	res := ValidateDynamicProposal(ValidateInput{Parameters: map[string]any{"manifest": m}, Mode: "GOVERNANCE_ENFORCE"})
	if res.Allowed || res.ReasonCode != REASON_DYNAMIC_AUTHORITY_FIELD_FORBIDDEN {
		t.Fatalf("expected nested authority block, got %+v", res)
	}
}

func TestValidatorGovernanceExemptBlocked(t *testing.T) {
	m := validManifestMap()
	m["policy"] = map[string]any{"governance_exempt": true}
	res := ValidateDynamicProposal(ValidateInput{Parameters: map[string]any{"manifest": m}, Mode: "GOVERNANCE_FAST"})
	if res.Allowed || res.ReasonCode != REASON_DYNAMIC_AUTHORITY_FIELD_FORBIDDEN {
		t.Fatalf("expected governance_exempt block, got %+v", res)
	}
}

func TestValidatorAllowHostBridgeBlocked(t *testing.T) {
	m := validManifestMap()
	m["capabilities"] = map[string]any{"allow-host-bridge": true}
	res := ValidateDynamicProposal(ValidateInput{Parameters: map[string]any{"manifest": m}, Mode: "GOVERNANCE_ENFORCE"})
	if res.Allowed || res.ReasonCode != REASON_DYNAMIC_AUTHORITY_FIELD_FORBIDDEN {
		t.Fatalf("expected malformed caps block, got %+v", res)
	}
}

func TestValidatorDisableAuditBlocked(t *testing.T) {
	m := validManifestMap()
	m["disable_audit"] = true
	res := ValidateDynamicProposal(ValidateInput{Parameters: map[string]any{"manifest": m}, Mode: "GOVERNANCE_ENFORCE"})
	if res.Allowed || res.ReasonCode != REASON_DYNAMIC_AUTHORITY_FIELD_FORBIDDEN {
		t.Fatalf("expected disable_audit block, got %+v", res)
	}
}

func TestValidatorCapabilityPolicies(t *testing.T) {
	base := validManifestMap()
	res := ValidateDynamicProposal(ValidateInput{Parameters: map[string]any{"manifest": base}, Mode: "GOVERNANCE_FAST"})
	if !res.Allowed || res.RequiresApproval {
		t.Fatalf("expected low-risk capability allowed, got %+v", res)
	}

	m2 := validManifestMap()
	m2["capabilities"] = []any{"dynamic.tool.register"}
	res2 := ValidateDynamicProposal(ValidateInput{Parameters: map[string]any{"manifest": m2}, Mode: "GOVERNANCE_FAST"})
	if !res2.Allowed || !res2.RequiresApproval || res2.ReasonCode != REASON_DYNAMIC_CAPABILITY_REQUIRES_APPROVAL {
		t.Fatalf("expected tool register approval required, got %+v", res2)
	}

	m3 := validManifestMap()
	m3["capabilities"] = []any{"dynamic.network.fetch"}
	res3 := ValidateDynamicProposal(ValidateInput{Parameters: map[string]any{"manifest": m3}, Mode: "GOVERNANCE_FAST"})
	if !res3.Allowed || !res3.RequiresApproval {
		t.Fatalf("expected network fetch approval required, got %+v", res3)
	}

	m4 := validManifestMap()
	m4["capabilities"] = []any{"dynamic.network.private"}
	res4 := ValidateDynamicProposal(ValidateInput{Parameters: map[string]any{"manifest": m4}, Mode: "GOVERNANCE_FAST"})
	if res4.Allowed || res4.ReasonCode != REASON_DYNAMIC_CAPABILITY_FORBIDDEN {
		t.Fatalf("expected private network blocked, got %+v", res4)
	}

	m5 := validManifestMap()
	m5["capabilities"] = []any{"dynamic.credentials.read"}
	res5 := ValidateDynamicProposal(ValidateInput{Parameters: map[string]any{"manifest": m5}, Mode: "GOVERNANCE_FAST"})
	if res5.Allowed || res5.ReasonCode != REASON_DYNAMIC_CAPABILITY_FORBIDDEN {
		t.Fatalf("expected credentials blocked, got %+v", res5)
	}

	m6 := validManifestMap()
	m6["capabilities"] = []any{"dynamic.host.bridge"}
	res6 := ValidateDynamicProposal(ValidateInput{Parameters: map[string]any{"manifest": m6}, Mode: "GOVERNANCE_FAST"})
	if res6.Allowed || res6.ReasonCode != REASON_DYNAMIC_CAPABILITY_FORBIDDEN {
		t.Fatalf("expected host bridge blocked, got %+v", res6)
	}

	m7 := validManifestMap()
	m7["capabilities"] = []any{"dynamic.policy.modify"}
	res7 := ValidateDynamicProposal(ValidateInput{Parameters: map[string]any{"manifest": m7}, Mode: "GOVERNANCE_FAST"})
	if res7.Allowed || res7.ReasonCode != REASON_DYNAMIC_CAPABILITY_FORBIDDEN {
		t.Fatalf("expected policy modify blocked, got %+v", res7)
	}

	m8 := validManifestMap()
	m8["capabilities"] = []any{"dynamic.vcse.promote"}
	res8 := ValidateDynamicProposal(ValidateInput{Parameters: map[string]any{"manifest": m8}, Mode: "GOVERNANCE_FAST"})
	if res8.Allowed || res8.ReasonCode != REASON_DYNAMIC_CAPABILITY_FORBIDDEN {
		t.Fatalf("expected vcse promote blocked, got %+v", res8)
	}
}
