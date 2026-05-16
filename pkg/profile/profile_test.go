package profile

import (
	"testing"

	"github.com/velariumai/gorkbot/pkg/harness"
	"github.com/velariumai/gorkbot/pkg/trace"
)

func TestNormalizeProfile(t *testing.T) {
	if got := NormalizeProfile("PoWeR_User"); got != ProfilePowerUser {
		t.Fatalf("expected power_user, got %q", got)
	}
	if got := NormalizeProfile("not-real"); got != ProfileUnknown {
		t.Fatalf("expected unknown, got %q", got)
	}
}

func TestUnknownProfileConservativeDefault(t *testing.T) {
	cfg := DefaultConfig(ProfileUnknown)
	if cfg.TraceMode != trace.ModeMinimal {
		t.Fatalf("unknown profile must default conservatively, got trace=%q", cfg.TraceMode)
	}
	if cfg.Automation.AutoPromotionMode != AutomationDisabled {
		t.Fatalf("unknown profile must disable auto promotion, got %q", cfg.Automation.AutoPromotionMode)
	}
	if cfg.Authority.ReleaseAuthority != AuthorityDeny {
		t.Fatalf("unknown profile must deny release authority, got %q", cfg.Authority.ReleaseAuthority)
	}
}

func TestBeginnerDefaultsConservative(t *testing.T) {
	cfg := DefaultConfig(ProfileBeginner)
	if cfg.TraceMode != trace.ModeMinimal {
		t.Fatalf("beginner trace should be minimal, got %q", cfg.TraceMode)
	}
	if cfg.HarnessMode != harness.ModeAudit {
		t.Fatalf("beginner harness should be audit, got %q", cfg.HarnessMode)
	}
	if cfg.Automation.AutoPromotionMode != AutomationDisabled {
		t.Fatalf("beginner auto promotion should be disabled, got %q", cfg.Automation.AutoPromotionMode)
	}
	if !cfg.Approval.RequireHumanApprovalForSensitive {
		t.Fatal("beginner should require approval for sensitive ops")
	}
}

func TestStandardDefaultsBalancedNotSensitiveAuthorizing(t *testing.T) {
	cfg := DefaultConfig(ProfileStandard)
	if cfg.TraceMode != trace.ModeAudit {
		t.Fatalf("standard trace should be audit, got %q", cfg.TraceMode)
	}
	if cfg.Automation.PlannerMutation != AutomationDisabled {
		t.Fatalf("standard planner mutation should be disabled, got %q", cfg.Automation.PlannerMutation)
	}
	if cfg.Authority.ShellAuthority == AuthorityAllow {
		t.Fatalf("standard shell authority must not be unconditional allow")
	}
}

func TestPowerUserDefaultsBroaderAuditableReversible(t *testing.T) {
	cfg := DefaultConfig(ProfilePowerUser)
	if cfg.Authority.FileAuthority != AuthorityAllowConfigured {
		t.Fatalf("power user file authority should be allow_configured, got %q", cfg.Authority.FileAuthority)
	}
	if cfg.Automation.PlannerMutation != AutomationSessionLocal {
		t.Fatalf("power user planner mutation should be session_local, got %q", cfg.Automation.PlannerMutation)
	}
	if !cfg.Evidence.RequireRollbackPlan || !cfg.Evidence.RequireDisablePath {
		t.Fatal("power user should require rollback and disable path")
	}
}

func TestExpertDefaultsConfigurableExplicit(t *testing.T) {
	cfg := DefaultConfig(ProfileExpert)
	if cfg.Authority.NetworkAuthority != AuthorityAllowConfigured {
		t.Fatalf("expert network authority should be allow_configured, got %q", cfg.Authority.NetworkAuthority)
	}
	if cfg.Automation.AutoPromotionMode != AutomationAllowConfigured {
		t.Fatalf("expert auto promotion should be allow_configured, got %q", cfg.Automation.AutoPromotionMode)
	}
}

func TestLabDefaultsConfigurableButReceiptsRequired(t *testing.T) {
	cfg := DefaultConfig(ProfileLab)
	if cfg.Automation.AutoPromotionMode != AutomationAllowConfigured {
		t.Fatalf("lab auto promotion should be allow_configured, got %q", cfg.Automation.AutoPromotionMode)
	}
	if !cfg.Evidence.RequireEvidenceReceipt {
		t.Fatal("lab must keep evidence receipts required")
	}
}

func TestEnterpriseDefaultsStrictApprovalHeavy(t *testing.T) {
	cfg := DefaultConfig(ProfileEnterprise)
	if cfg.Authority.ReleaseAuthority != AuthorityDeny {
		t.Fatalf("enterprise release authority must be deny, got %q", cfg.Authority.ReleaseAuthority)
	}
	if cfg.Automation.AutoPromotionMode != AutomationDisabled {
		t.Fatalf("enterprise auto promotion must be disabled, got %q", cfg.Automation.AutoPromotionMode)
	}
	if !cfg.Approval.RequireApprovalForNetwork {
		t.Fatal("enterprise should require network approval")
	}
}
