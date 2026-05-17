package profile

import (
	"testing"

	"github.com/velariumai/gorkbot/pkg/evidence"
	"github.com/velariumai/gorkbot/pkg/harness"
	"github.com/velariumai/gorkbot/pkg/trace"
)

func TestReleaseReadinessProfileMatrixConservativeDefaults(t *testing.T) {
	tests := []struct {
		name        string
		profile     Profile
		traceModes  []trace.Mode
		harnessMode harness.Mode
	}{
		{name: "beginner", profile: ProfileBeginner, traceModes: []trace.Mode{trace.ModeMinimal}, harnessMode: harness.ModeAudit},
		{name: "standard", profile: ProfileStandard, traceModes: []trace.Mode{trace.ModeAudit}, harnessMode: harness.ModeAudit},
		{name: "power_user", profile: ProfilePowerUser, traceModes: []trace.Mode{trace.ModeAudit}, harnessMode: harness.ModeAudit},
		{name: "expert", profile: ProfileExpert, traceModes: []trace.Mode{trace.ModeDebug}, harnessMode: harness.ModeAudit},
		{name: "lab", profile: ProfileLab, traceModes: []trace.Mode{trace.ModeReplay}, harnessMode: harness.ModeAudit},
		{name: "enterprise", profile: ProfileEnterprise, traceModes: []trace.Mode{trace.ModeAudit}, harnessMode: harness.ModeAudit},
		{name: "custom", profile: ProfileCustom, traceModes: []trace.Mode{trace.ModeAudit}, harnessMode: harness.ModeAudit},
		{name: "unknown", profile: ProfileUnknown, traceModes: []trace.Mode{trace.ModeMinimal}, harnessMode: harness.ModeAudit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig(tt.profile)
			if !containsTraceMode(tt.traceModes, cfg.TraceMode) {
				t.Fatalf("unexpected trace mode for %s: got %q want one of %v", tt.name, cfg.TraceMode, tt.traceModes)
			}
			if cfg.HarnessMode != tt.harnessMode {
				t.Fatalf("unexpected harness mode for %s: got %q want %q", tt.name, cfg.HarnessMode, tt.harnessMode)
			}
			if !cfg.Evidence.VectorCandidateOnly {
				t.Fatalf("profile %s must keep vector retrieval candidate-only", tt.name)
			}
			if tt.profile != ProfileExpert && tt.profile != ProfileLab && tt.profile != ProfileCustom &&
				cfg.Authority.ReleaseAuthority == AuthorityAllowConfigured {
				t.Fatalf("profile %s must not make release authority advanced by default", tt.name)
			}
			if cfg.Automation.ReleaseMode != AutomationDisabled && cfg.Automation.ReleaseMode != AutomationApprovalRequired && cfg.Automation.ReleaseMode != AutomationAllowConfigured {
				t.Fatalf("profile %s has invalid release automation posture: %q", tt.name, cfg.Automation.ReleaseMode)
			}
		})
	}
}

func TestReleaseReadinessPolicyAbsenceAndExplicitAuthority(t *testing.T) {
	cfg := DefaultConfig(ProfileExpert)
	cfg.ConfiguredCapabilities = nil

	release := EvaluateCapability(cfg, CapabilityReleasePublish)
	if release.Decision == evidence.DecisionAuditOnly || release.Decision == evidence.DecisionAllowLowRisk {
		t.Fatalf("release without explicit configured capability must not be allowed, got %+v", release)
	}
	if release.ReasonCode != "release_authority_explicit_required" {
		t.Fatalf("release must receipt explicit authority requirement, got %+v", release)
	}

	network := EvaluateCapability(cfg, CapabilityNetworkEgress)
	if network.Decision == evidence.DecisionAuditOnly {
		t.Fatalf("policy absence must not authorize network egress, got %+v", network)
	}

	vector := EvaluateCapability(cfg, CapabilityVectorRetrieve)
	if vector.Metadata["vector_role"] != "candidate_only" {
		t.Fatalf("vector retrieval must be candidate-only, got %+v", vector)
	}

	assertTruth := EvaluateCapability(cfg, CapabilityVectorAssertTruth)
	if assertTruth.Authority != evidence.AuthorityHardInvariant || assertTruth.Decision != evidence.DecisionDenySensitive {
		t.Fatalf("vector assert truth must remain hard-denied, got %+v", assertTruth)
	}
}

func TestReleaseReadinessTraceAndHarnessModeMatrix(t *testing.T) {
	traceModes := []trace.Mode{
		trace.ModeOff,
		trace.ModeMinimal,
		trace.ModeAudit,
		trace.ModeDebug,
		trace.ModeReplay,
	}
	for _, mode := range traceModes {
		t.Run("trace_"+string(mode), func(t *testing.T) {
			if got := trace.ParseMode(string(mode)); got != mode {
				t.Fatalf("trace mode %q parsed as %q", mode, got)
			}
		})
	}

	harnessModes := []harness.Mode{
		harness.ModeOff,
		harness.ModeAudit,
	}
	for _, mode := range harnessModes {
		t.Run("harness_"+string(mode), func(t *testing.T) {
			if got := harness.ParseMode(string(mode)); got != mode {
				t.Fatalf("harness mode %q parsed as %q", mode, got)
			}
		})
	}
}

func containsTraceMode(modes []trace.Mode, mode trace.Mode) bool {
	for _, candidate := range modes {
		if candidate == mode {
			return true
		}
	}
	return false
}
