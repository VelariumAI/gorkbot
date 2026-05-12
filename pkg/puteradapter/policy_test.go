package puteradapter

import "testing"

func mustPath(t *testing.T, raw string, manifest PuterWorkspaceManifest) PuterWorkspacePath {
	t.Helper()
	safe, decision := ValidatePuterWorkspacePath(raw, manifest)
	if !decision.Allowed {
		t.Fatalf("path %q rejected: %s", raw, decision.ReasonCode)
	}
	return safe
}

func mustKey(t *testing.T, raw string) PuterKVKey {
	t.Helper()
	safe, decision := ValidatePuterKVKey(raw)
	if !decision.Allowed {
		t.Fatalf("key %q rejected: %s", raw, decision.ReasonCode)
	}
	return safe
}

func TestPolicy_ProtectedDeleteBlockedAndProtectedWriteRequiresApproval(t *testing.T) {
	manifest := DefaultWorkspaceManifest()
	policy := DefaultCapabilityPolicy()

	deleteLogs := policy.EvaluatePathOperation(OpFSDelete, mustPath(t, "/Gorkbot/logs/a.log", manifest), manifest)
	if deleteLogs.Allowed || deleteLogs.RequiresApproval {
		t.Fatalf("expected protected delete blocked: %+v", deleteLogs)
	}
	if deleteLogs.ReasonCode != ReasonProtectedDeleteBlocked {
		t.Fatalf("unexpected reason: %s", deleteLogs.ReasonCode)
	}

	writeReceipts := policy.EvaluatePathOperation(OpFSWrite, mustPath(t, "/Gorkbot/receipts/op.json", manifest), manifest)
	if writeReceipts.Allowed || !writeReceipts.RequiresApproval {
		t.Fatalf("expected write receipts to require approval: %+v", writeReceipts)
	}
}

func TestPolicy_AllowedWritesAndKVRules(t *testing.T) {
	manifest := DefaultWorkspaceManifest()
	policy := DefaultCapabilityPolicy()

	writeScratch := policy.EvaluatePathOperation(OpFSWrite, mustPath(t, "/Gorkbot/scratch/out.txt", manifest), manifest)
	if !writeScratch.Allowed {
		t.Fatalf("expected scratch write allowed: %+v", writeScratch)
	}
	writeExperiments := policy.EvaluatePathOperation(OpFSWrite, mustPath(t, "/Gorkbot/experiments/x/result.json", manifest), manifest)
	if !writeExperiments.Allowed {
		t.Fatalf("expected experiments write allowed: %+v", writeExperiments)
	}

	kvAllowed := policy.EvaluateKVOperation(OpKVSet, mustKey(t, "gorkbot.mission.run42"))
	if !kvAllowed.Allowed {
		t.Fatalf("expected mission KV set allowed: %+v", kvAllowed)
	}
	kvBlocked := policy.EvaluateKVOperation(OpKVSet, mustKey(t, "other.namespace"))
	if kvBlocked.Allowed {
		t.Fatalf("expected non-gorkbot namespace blocked")
	}
	if kvBlocked.ReasonCode != ReasonKVNamespaceBlocked {
		t.Fatalf("unexpected reason: %s", kvBlocked.ReasonCode)
	}
}

func TestPolicy_CapabilityOutcomes(t *testing.T) {
	policy := DefaultCapabilityPolicy()

	allowed := policy.EvaluateStandaloneCapability(OpAppPreview)
	if !allowed.Allowed {
		t.Fatalf("expected app preview allowed: %+v", allowed)
	}

	requiresApproval := policy.EvaluateStandaloneCapability(OpHostingPublish)
	if requiresApproval.Allowed || !requiresApproval.RequiresApproval {
		t.Fatalf("expected hosting publish requires approval: %+v", requiresApproval)
	}

	blocked := policy.EvaluateStandaloneCapability(OpBridgeHost)
	if blocked.Allowed || blocked.RequiresApproval {
		t.Fatalf("expected bridge host blocked: %+v", blocked)
	}
}
