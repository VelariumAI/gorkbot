package profile

import (
	"testing"

	"github.com/velariumai/gorkbot/pkg/evidence"
)

func TestVectorRetrieveCandidateOnly(t *testing.T) {
	cfg := DefaultConfig(ProfileStandard)
	assessment := EvaluateCapability(cfg, CapabilityVectorRetrieve)
	if assessment.Metadata["vector_role"] != "candidate_only" {
		t.Fatalf("vector retrieval must be candidate-only, got metadata=%v", assessment.Metadata)
	}
}

func TestVectorAssertTruthDenied(t *testing.T) {
	cfg := DefaultConfig(ProfileExpert)
	assessment := EvaluateCapability(cfg, CapabilityVectorAssertTruth)
	if assessment.Decision != evidence.DecisionDenySensitive {
		t.Fatalf("vector assert truth must be denied, got %+v", assessment)
	}
	if assessment.Authority != evidence.AuthorityHardInvariant {
		t.Fatalf("vector assert truth must use hard invariant authority, got %q", assessment.Authority)
	}
}

func TestNoPolicyAbsenceAsPermission(t *testing.T) {
	cfg := DefaultConfig(ProfileStandard)
	cfg.Authority.NetworkAuthority = AuthorityAllowConfigured
	cfg.ConfiguredCapabilities = nil
	assessment := EvaluateCapability(cfg, CapabilityNetworkEgress)
	if assessment.Decision == evidence.DecisionAuditOnly {
		t.Fatalf("policy absence must not become permission: %+v", assessment)
	}
}

func TestReleaseAuthorityRequiresExplicitConfig(t *testing.T) {
	cfg := DefaultConfig(ProfileLab)
	cfg.Authority.ReleaseAuthority = AuthorityAllowConfigured
	cfg.ConfiguredCapabilities = nil
	assessment := EvaluateCapability(cfg, CapabilityReleasePublish)
	if assessment.ReasonCode != "release_authority_explicit_required" {
		t.Fatalf("release should require explicit authority, got %+v", assessment)
	}
}

func TestSensitiveNeedsEvidenceForPromotionAndRelease(t *testing.T) {
	cfg := DefaultConfig(ProfileExpert)
	cfg.CustomProfileConfigured = true
	cfg.ConfiguredCapabilities = map[Capability]bool{
		CapabilitySelfmodPromote: true,
		CapabilityReleasePublish: true,
	}
	cfg.Evidence.RequireEvidenceReceipt = false
	assessment := EvaluateCapability(cfg, CapabilitySelfmodPromote)
	if assessment.ReasonCode != "evidence_receipt_required" {
		t.Fatalf("promotion should require evidence receipts, got %+v", assessment)
	}
}

func TestUnknownAuthorityDoesNotAuthorizeSensitive(t *testing.T) {
	cfg := DefaultConfig(ProfileExpert)
	cfg.Authority.ReleaseAuthority = AuthorityUnknown
	assessment := EvaluateCapability(cfg, CapabilityReleasePublish)
	if assessment.Decision == evidence.DecisionAuditOnly {
		t.Fatalf("unknown authority must not authorize sensitive capability: %+v", assessment)
	}
}

func TestStandardSensitiveRemainsApprovalRequired(t *testing.T) {
	cfg := DefaultConfig(ProfileStandard)
	assessment := EvaluateCapability(cfg, CapabilityShellExecute)
	if assessment.Decision != evidence.DecisionRequireApproval {
		t.Fatalf("standard sensitive capability should require approval, got %+v", assessment)
	}
}
