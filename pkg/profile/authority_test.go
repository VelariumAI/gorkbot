package profile

import (
	"testing"

	"github.com/velariumai/gorkbot/pkg/evidence"
)

func TestAuthoritySurfaceNormalizeSafely(t *testing.T) {
	cfg := AuthorityConfig{ToolAuthority: AuthorityMode("WILD")}.Normalized()
	if cfg.ToolAuthority != AuthorityUnknown {
		t.Fatalf("unknown authority should normalize to unknown, got %q", cfg.ToolAuthority)
	}
}

func TestUnknownAuthorityDoesNotAuthorize(t *testing.T) {
	cfg := DefaultConfig(ProfileStandard)
	cfg.Authority.NetworkAuthority = AuthorityUnknown
	cfg.CustomProfileConfigured = true
	cfg.ConfiguredCapabilities = map[Capability]bool{CapabilityNetworkEgress: true}

	assessment := EvaluateCapability(cfg, CapabilityNetworkEgress)
	if assessment.Decision == evidence.DecisionAuditOnly || assessment.Decision == evidence.DecisionAllowLowRisk {
		t.Fatalf("unknown authority must not authorize sensitive capability: %+v", assessment)
	}
}

func TestAuditOnlyDoesNotAuthorizeSensitive(t *testing.T) {
	cfg := DefaultConfig(ProfileStandard)
	cfg.Authority.ShellAuthority = AuthorityAuditOnly
	assessment := EvaluateCapability(cfg, CapabilityShellExecute)
	if assessment.Decision == evidence.DecisionAuditOnly {
		t.Fatalf("audit_only must not authorize sensitive operations: %+v", assessment)
	}
}

func TestAllowConfiguredDistinctFromAllow(t *testing.T) {
	cfg := DefaultConfig(ProfilePowerUser)
	cfg.Authority.FileAuthority = AuthorityAllowConfigured
	assessment1 := EvaluateCapability(cfg, CapabilityFileMutate)
	if assessment1.Decision == evidence.DecisionAuditOnly {
		t.Fatalf("allow_configured without explicit capability should not authorize: %+v", assessment1)
	}

	cfg.ConfiguredCapabilities = map[Capability]bool{CapabilityFileMutate: true}
	assessment2 := EvaluateCapability(cfg, CapabilityFileMutate)
	if assessment2.Decision != evidence.DecisionRequireApproval {
		t.Fatalf("configured sensitive operation should remain explicit/approval-based, got %+v", assessment2)
	}
}
