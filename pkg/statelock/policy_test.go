package statelock

import "testing"

func TestPolicyAbsenceStates(t *testing.T) {
	states := []PolicyState{PolicyOff, PolicyNotConfigured, PolicyUnavailable, PolicyNoMatch, PolicyInvalid}
	for _, s := range states {
		if !IsPolicyAbsent(s) {
			t.Fatalf("expected absent for %s", s)
		}
	}
	if IsPolicyAbsent(PolicyMatched) {
		t.Fatal("policy_matched must not be absent")
	}
}

func TestNoPolicyNotPermissionForSensitive(t *testing.T) {
	if AllowsSensitiveOperation(PolicyNoMatch) {
		t.Fatal("no-match policy must not allow sensitive operation")
	}
	if !RequiresApprovalForSensitive(PolicyNoMatch) {
		t.Fatal("no-match policy should require approval for sensitive")
	}
	if !AllowsSensitiveOperation(PolicyEnforced) {
		t.Fatal("enforced policy should allow sensitive operations")
	}
}

func TestLowRiskPolicyAbsenceHandling(t *testing.T) {
	if got := ClassifyOperationRisk("read_metadata"); got != RiskLow {
		t.Fatalf("expected low risk, got %s", got)
	}
	if got := ClassifyOperationRisk("provider_selection"); got != RiskMedium {
		t.Fatalf("expected medium risk, got %s", got)
	}
	if got := ClassifyOperationRisk(string(SensitiveShellExecution)); got != RiskSensitive {
		t.Fatalf("expected sensitive risk, got %s", got)
	}
}

func TestSensitiveOperationClasses(t *testing.T) {
	ops := []string{
		string(SensitiveCredentialAccess),
		string(SensitiveNetworkEgress),
		string(SensitivePrivateNetwork),
		string(SensitiveFileMutation),
		string(SensitiveSelfmodPromotion),
		string(SensitiveToolInstallation),
		string(SensitiveShellExecution),
		string(SensitiveReleasePublish),
		string(SensitiveHostBridge),
		string(SensitiveWorkspaceEscape),
	}
	for _, op := range ops {
		if got := ClassifyOperationRisk(op); got != RiskSensitive {
			t.Fatalf("expected sensitive for %s, got %s", op, got)
		}
	}
}
