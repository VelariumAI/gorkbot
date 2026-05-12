package selfmod

import "testing"

func TestPathStagingAllowed(t *testing.T) {
	_, blocked, reason, _ := validateTargetPath(DynamicArtifactPath{value: ".gorkbot/staging/tools/a.go"})
	if blocked || reason != "" {
		t.Fatalf("expected allowed path, blocked=%v reason=%s", blocked, reason)
	}
}

func TestPathTraversalBlocked(t *testing.T) {
	_, blocked, reason, _ := validateTargetPath(DynamicArtifactPath{value: "../pkg/tools/a.go"})
	if !blocked || reason != REASON_DYNAMIC_PATH_TRAVERSAL {
		t.Fatalf("expected traversal block, got blocked=%v reason=%s", blocked, reason)
	}
}

func TestPathAbsoluteBlocked(t *testing.T) {
	_, blocked, reason, _ := validateTargetPath(DynamicArtifactPath{value: "/tmp/out.go"})
	if !blocked || reason != REASON_DYNAMIC_PATH_OUTSIDE_STAGING {
		t.Fatalf("expected absolute block, got blocked=%v reason=%s", blocked, reason)
	}
}

func TestPathDotGitBlocked(t *testing.T) {
	_, blocked, reason, _ := validateTargetPath(DynamicArtifactPath{value: ".git/config"})
	if !blocked || reason != REASON_DYNAMIC_PROTECTED_TARGET {
		t.Fatalf("expected .git block, got blocked=%v reason=%s", blocked, reason)
	}
}

func TestPathWorkflowRequiresApproval(t *testing.T) {
	req, blocked, reason, _ := validateTargetPath(DynamicArtifactPath{value: ".github/workflows/ci.yml"})
	if blocked || !req || reason != REASON_DYNAMIC_PROMOTION_REQUIRES_APPROVAL {
		t.Fatalf("expected workflow approval, got req=%v blocked=%v reason=%s", req, blocked, reason)
	}
}

func TestPathGovernanceProtected(t *testing.T) {
	_, blocked, reason, _ := validateTargetPath(DynamicArtifactPath{value: "pkg/governance/policy.go"})
	if !blocked || reason != REASON_DYNAMIC_PROTECTED_TARGET {
		t.Fatalf("expected governance protected block, got blocked=%v reason=%s", blocked, reason)
	}
}

func TestPathRegistryProtected(t *testing.T) {
	_, blocked, reason, _ := validateTargetPath(DynamicArtifactPath{value: "pkg/tools/registry.go"})
	if !blocked || reason != REASON_DYNAMIC_PROTECTED_TARGET {
		t.Fatalf("expected registry protected block, got blocked=%v reason=%s", blocked, reason)
	}
}

func TestPathControlCharBlocked(t *testing.T) {
	_, blocked, reason, _ := validateTargetPath(DynamicArtifactPath{value: ".gorkbot/staging/a\n.go"})
	if !blocked || reason != REASON_DYNAMIC_CONTROL_CHAR {
		t.Fatalf("expected control-char block, got blocked=%v reason=%s", blocked, reason)
	}
}

func TestPathNULBlocked(t *testing.T) {
	_, blocked, reason, _ := validateTargetPath(DynamicArtifactPath{value: ".gorkbot/staging/a\x00.go"})
	if !blocked || reason != REASON_DYNAMIC_CONTROL_CHAR {
		t.Fatalf("expected NUL block, got blocked=%v reason=%s", blocked, reason)
	}
}
