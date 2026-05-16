package profile

import "testing"

func TestApprovalDefaultsConservative(t *testing.T) {
	cfg := DefaultConfig(ProfileBeginner)
	if !cfg.Approval.RequireHumanApprovalForSensitive {
		t.Fatal("beginner must require human approval for sensitive ops")
	}
	if !cfg.Approval.RequireApprovalForPolicyAbsence {
		t.Fatal("policy absence must require approval by default")
	}
	if !cfg.Approval.RequireApprovalForRelease {
		t.Fatal("release should require approval by default")
	}
}

func TestLabMayRelaxButExplicit(t *testing.T) {
	cfg := DefaultConfig(ProfileLab)
	if cfg.Approval.RequireApprovalForShell {
		t.Fatal("lab may relax shell approval requirement")
	}
	if cfg.Authority.ShellAuthority != AuthorityAllowConfigured {
		t.Fatalf("lab shell authority should remain explicit, got %q", cfg.Authority.ShellAuthority)
	}
}
