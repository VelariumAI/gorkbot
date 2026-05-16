package skillruntime

import (
	"context"
	"strings"
	"testing"

	"github.com/velariumai/gorkbot/pkg/evidence"
	"github.com/velariumai/gorkbot/pkg/harness"
	"github.com/velariumai/gorkbot/pkg/profile"
	"github.com/velariumai/gorkbot/pkg/replay"
	"github.com/velariumai/gorkbot/pkg/statelock"
)

func testCandidate() Candidate {
	return Candidate{
		Name:           "skill-candidate",
		Source:         "local-analysis",
		Risk:           evidence.RiskMedium,
		OperationClass: evidence.SensitiveSelfmodPromotion,
	}
}

func testConfigWithStageCapability() profile.Config {
	cfg := profile.DefaultConfig(profile.ProfileExpert)
	cfg.CustomProfileConfigured = true
	cfg.ConfiguredCapabilities = map[profile.Capability]bool{
		profile.CapabilitySkillStage:   true,
		profile.CapabilitySkillPromote: true,
	}
	cfg.Approval.RequireHumanApprovalForSensitive = false
	cfg.Approval.RequireApprovalForPromotion = false
	cfg.Approval.RequireApprovalForIrreversibleMutat = false
	cfg.Approval.RequireApprovalForPolicyAbsence = true
	return cfg
}

func TestEvaluateUnknownProfileConservative(t *testing.T) {
	req := Request{
		Operation: OperationStage,
		Candidate: testCandidate(),
		Config:    profile.DefaultConfig(profile.ProfileUnknown),
	}
	res := Evaluate(req)
	if res.Status != StatusApprovalRequired {
		t.Fatalf("unknown profile should be conservative, got status=%q", res.Status)
	}
}

func TestEvaluateCustomProfileRequiresMarker(t *testing.T) {
	cfg := profile.DefaultConfig(profile.ProfileCustom)
	cfg.CustomProfileConfigured = false
	req := Request{Operation: OperationStage, Candidate: testCandidate(), Config: cfg}
	res := Evaluate(req)
	if res.Status != StatusConfigRequired {
		t.Fatalf("expected config_required for unmarked custom profile, got %q", res.Status)
	}
}

func TestAllowConfiguredRequiresConfiguredCapability(t *testing.T) {
	cfg := profile.DefaultConfig(profile.ProfileExpert)
	cfg.ConfiguredCapabilities = nil
	req := Request{Operation: OperationStage, Candidate: testCandidate(), Config: cfg}
	res := Evaluate(req)
	if res.Status != StatusConfigRequired {
		t.Fatalf("expected config_required when allow_configured capability missing, got %q", res.Status)
	}
}

func TestPolicyAbsenceNotPermission(t *testing.T) {
	cfg := profile.DefaultConfig(profile.ProfileStandard)
	req := Request{Operation: OperationStage, Candidate: testCandidate(), Config: cfg}
	res := Evaluate(req)
	if res.Status != StatusApprovalRequired {
		t.Fatalf("policy absence must not auto-permit stage, got %q", res.Status)
	}
}

func TestVectorRetrieveCandidateOnlyNotTruth(t *testing.T) {
	cfg := profile.DefaultConfig(profile.ProfileStandard)
	req := Request{Operation: OperationRetrieve, Candidate: testCandidate(), Config: cfg}
	res := Evaluate(req)
	if res.Status != StatusAllowed {
		t.Fatalf("retrieve should remain low-risk allowed, got %q", res.Status)
	}
	joined := strings.Join(res.Warnings, "|")
	if !strings.Contains(joined, "candidate-only") {
		t.Fatalf("expected candidate-only warning, got %v", res.Warnings)
	}
	if !strings.Contains(joined, "not truth") {
		t.Fatalf("expected not-truth warning, got %v", res.Warnings)
	}
}

func TestMissingEvidenceBehavior(t *testing.T) {
	cfg := testConfigWithStageCapability()
	req := Request{Operation: OperationValidate, Candidate: testCandidate(), Config: cfg}
	res := Evaluate(req)
	if res.Status != StatusApprovalRequired {
		t.Fatalf("missing harness/replay/statelock should require approval, got %q", res.Status)
	}
	if len(res.Warnings) == 0 {
		t.Fatalf("expected warnings for missing evidence")
	}
}

func TestReplayRegressionBehavior(t *testing.T) {
	cfg := testConfigWithStageCapability()
	req := Request{
		Operation: OperationPromote,
		Candidate: testCandidate(),
		Config:    cfg,
		ReplayResult: &replay.Result{
			CaseID:   "c1",
			Verdict:  replay.VerdictRegression,
			Duration: 1,
		},
		HarnessReport:   &harness.Report{HarnessID: "h1", ArtifactID: "a1", Status: harness.StatusPass},
		StateLockResult: &statelock.CheckResult{Status: statelock.CheckStatusAllowed, PolicyState: statelock.PolicyMatched, Risk: statelock.RiskLow},
		Metadata:        map[string]string{"rollback_path": "yes", "disable_path": "yes"},
	}
	res := Evaluate(req)
	if res.Status != StatusDenied && res.Status != StatusApprovalRequired {
		t.Fatalf("replay regression should deny or require approval, got %q", res.Status)
	}
}

func TestStateLockConflictParadoxBehavior(t *testing.T) {
	cfg := testConfigWithStageCapability()
	p := statelock.ParadoxReport{Status: statelock.ParadoxConfirmed, Summary: "no valid path"}
	req := Request{
		Operation:       OperationPromote,
		Candidate:       testCandidate(),
		Config:          cfg,
		HarnessReport:   &harness.Report{HarnessID: "h1", ArtifactID: "a1", Status: harness.StatusPass},
		ReplayResult:    &replay.Result{CaseID: "c1", Verdict: replay.VerdictPass, Duration: 1},
		StateLockResult: &statelock.CheckResult{Status: statelock.CheckStatusConflict, PolicyState: statelock.PolicyMatched, Risk: statelock.RiskSensitive, Paradox: &p},
		Metadata:        map[string]string{"rollback_path": "yes", "disable_path": "yes"},
	}
	res := Evaluate(req)
	if res.Status != StatusDenied && res.Status != StatusApprovalRequired {
		t.Fatalf("statelock conflict/paradox should deny or require approval, got %q", res.Status)
	}
}

func TestPromotionRequiresReceiptAndRollbackDisablePaths(t *testing.T) {
	cfg := testConfigWithStageCapability()
	req := Request{
		Operation:       OperationPromote,
		Candidate:       testCandidate(),
		Config:          cfg,
		HarnessReport:   &harness.Report{HarnessID: "h1", ArtifactID: "a1", Status: harness.StatusPass},
		ReplayResult:    &replay.Result{CaseID: "c1", Verdict: replay.VerdictPass, Duration: 1},
		StateLockResult: &statelock.CheckResult{Status: statelock.CheckStatusAllowed, PolicyState: statelock.PolicyMatched, Risk: statelock.RiskLow},
		Metadata:        map[string]string{},
	}
	res := Evaluate(req)
	if res.Status != StatusApprovalRequired {
		t.Fatalf("promotion without rollback/disable evidence must not pass, got %q", res.Status)
	}
}

func TestSensitiveMetadataRedaction(t *testing.T) {
	cfg := testConfigWithStageCapability()
	req := Request{
		Operation: OperationPropose,
		Candidate: Candidate{
			Name:   "x",
			Source: "s",
			Metadata: map[string]string{
				"token": "abc",
			},
		},
		Config: cfg,
		Metadata: map[string]string{
			"secret": "abc",
		},
	}
	res := Evaluate(req)
	if res.Metadata["secret"] != "[REDACTED]" {
		t.Fatalf("result metadata should redact secret")
	}
}

func TestMalformedInputNoPanic(t *testing.T) {
	f := Facade{Store: NewMemoryStore()}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected panic: %v", r)
		}
	}()
	_, _ = f.Run(context.Background(), Request{Operation: Operation("???")})
}
