package evidence

import "testing"

func TestClassifyOperationRisk(t *testing.T) {
	if got := ClassifyOperationRisk("help"); got != RiskLow {
		t.Fatalf("expected low for help, got %s", got)
	}
	if got := ClassifyOperationRisk("provider_selection"); got != RiskMedium {
		t.Fatalf("expected medium for provider_selection, got %s", got)
	}
	if got := ClassifyOperationRisk(string(SensitiveShellExecution)); got != RiskSensitive {
		t.Fatalf("expected sensitive for shell_execution, got %s", got)
	}
	if got := ClassifyOperationRisk("unknown_operation"); got != RiskUnknown {
		t.Fatalf("expected unknown for unknown operation, got %s", got)
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
		if !IsSensitiveOperation(op) {
			t.Fatalf("expected sensitive class for %s", op)
		}
	}
}
