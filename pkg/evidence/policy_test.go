package evidence

import "testing"

func TestPolicyAbsenceStates(t *testing.T) {
	states := []PolicyState{PolicyOff, PolicyNotConfigured, PolicyUnavailable, PolicyNoMatch, PolicyInvalid}
	for _, s := range states {
		if !IsPolicyAbsent(s) {
			t.Fatalf("expected absent state for %s", s)
		}
	}
	if IsPolicyAbsent(PolicyMatched) {
		t.Fatal("matched policy must not be absent")
	}
}

func TestNoPolicyNotPermissionForSensitive(t *testing.T) {
	if AllowsSensitiveOperation(PolicyNoMatch) {
		t.Fatal("no-match policy must not allow sensitive operations")
	}
	if !RequiresApprovalForSensitive(PolicyNoMatch) {
		t.Fatal("no-match policy must require approval")
	}
	if !AllowsSensitiveOperation(PolicyEnforced) {
		t.Fatal("enforced policy should allow sensitive operations")
	}
}

func TestAuditOnlyIsVisibleNotAuthoritativeForSensitive(t *testing.T) {
	if AllowsSensitiveOperation(PolicyAuditOnly) {
		t.Fatal("audit-only policy must not authorize sensitive operations")
	}
	if !RequiresApprovalForSensitive(PolicyAuditOnly) {
		t.Fatal("audit-only policy should require approval for sensitive operations")
	}
}

func TestIsPolicyAuthoritative(t *testing.T) {
	if !IsPolicyAuthoritative(PolicyMatched) {
		t.Fatal("policy_matched should be authoritative")
	}
	if !IsPolicyAuthoritative(PolicyEnforced) {
		t.Fatal("policy_enforced should be authoritative")
	}
	if !IsPolicyAuthoritative(PolicyAuditOnly) {
		t.Fatal("policy_audit_only should be authoritative")
	}
	if IsPolicyAuthoritative(PolicyNoMatch) {
		t.Fatal("policy_no_match should not be authoritative")
	}
}
