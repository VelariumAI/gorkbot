package releasecheck

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/velariumai/gorkbot/pkg/evidence"
	"github.com/velariumai/gorkbot/pkg/harness"
	"github.com/velariumai/gorkbot/pkg/profile"
	"github.com/velariumai/gorkbot/pkg/replay"
	"github.com/velariumai/gorkbot/pkg/skillruntime"
	"github.com/velariumai/gorkbot/pkg/statelock"
	"github.com/velariumai/gorkbot/pkg/trace"
)

func TestRR003VARSpineFixture(t *testing.T) {
	ctx := context.Background()
	cfg := rr003FixtureConfig()

	happy := rr003HappyPath(t, ctx, cfg)
	t.Logf("RR003_SCENARIO name=happy_path classification=PASS")
	t.Logf("RR003_STORY happy_path: local operator stages a fixture skill candidate after profile, evidence, harness, replay, statelock, and trace refs agree")
	t.Logf("RR003_EXPECTED happy_path: staged, receipted, trace-linked, no provider/tool/network execution")
	t.Logf("RR003_ACTUAL happy_path: status=%s decision=%s receipt=%s refs=%d harness=%s replay=%s statelock=%s", happy.Stage.Status, happy.Stage.Decision, happy.Stage.Receipt.ID, len(happy.Stage.ValidationRefs), happy.Harness.Status, happy.Replay.Verdict, happy.StateLock.Status)
	t.Logf("RR003_EVIDENCE happy_path: config_ref=%s harness_ref=%s replay_ref=%s receipt_ref=%s", profile.ConfigRef(cfg).Ref, happy.HarnessRef.Ref, happy.ReplayRef.Ref, evidence.ReceiptRef(happy.Receipt).Ref)

	rr003NegativeMissingEvidence(t, cfg)
	rr003NegativeVectorTruth(t, cfg)
}

type rr003HappyFixture struct {
	Harness    harness.Report
	HarnessRef trace.Ref
	Replay     replay.Result
	ReplayRef  trace.Ref
	StateLock  statelock.CheckResult
	Receipt    evidence.Receipt
	Stage      skillruntime.Result
}

func rr003FixtureConfig() profile.Config {
	cfg := profile.DefaultConfig(profile.ProfileExpert)
	cfg.ConfiguredCapabilities = map[profile.Capability]bool{
		profile.CapabilitySelfmodValidate: true,
		profile.CapabilitySkillStage:      true,
		profile.CapabilitySkillPromote:    true,
	}
	cfg.Approval.RequireApprovalForSelfmod = false
	cfg.Approval.RequireApprovalForPolicyAbsence = true
	return cfg.Normalized()
}

func rr003HappyPath(t *testing.T, ctx context.Context, cfg profile.Config) rr003HappyFixture {
	t.Helper()

	artifact := harness.Artifact{
		ID:      "rr003-var-spine-artifact",
		Kind:    harness.ArtifactKindSelfmodManifest,
		Name:    "rr003-var-spine",
		Content: "name: rr003-var-spine\nrollback: fixture rollback present\ndisable_path: fixture disable present\nprovider_call: none\n",
		Metadata: map[string]string{
			"rollback_path": "fixture",
			"disable_path":  "fixture",
			"policy_state":  string(evidence.PolicyMatched),
		},
		Refs: []trace.Ref{profile.ConfigRef(cfg)},
	}
	artifact = artifact.Normalized()
	artifactRef := trace.NewRef("fixture_artifact", "artifact:"+artifact.ID, artifact.ContentHash, int64(len(artifact.Content)))

	registry := harness.NewRegistry(harness.WithHarnessID("rr003.fixture.harness"))
	if err := registry.RegisterMany([]harness.Assertion{
		{
			ID:        "rr003-requires-rollback",
			Scope:     artifact.PrimaryScope(),
			Severity:  harness.SeverityHardFail,
			Type:      harness.AssertionTypeRequiredMetadata,
			Condition: "rollback_path",
			Message:   "rollback evidence must be visible",
		},
		{
			ID:        "rr003-requires-disable",
			Scope:     artifact.PrimaryScope(),
			Severity:  harness.SeverityHardFail,
			Type:      harness.AssertionTypeRequiredMetadata,
			Condition: "disable_path",
			Message:   "disable evidence must be visible",
		},
		{
			ID:        "rr003-forbids-provider-call",
			Scope:     artifact.PrimaryScope(),
			Severity:  harness.SeverityHardFail,
			Type:      harness.AssertionTypeStringForbid,
			Condition: "provider_call: live",
			Message:   "fixture must not request live provider execution",
		},
	}); err != nil {
		t.Fatalf("register fixture assertions: %v", err)
	}

	harnessReport := registry.Validate(ctx, artifact)
	if harnessReport.Status != harness.StatusPass {
		t.Fatalf("happy path harness report status=%s, want pass", harnessReport.Status)
	}
	harnessRef := harnessReport.ValidationRef()

	baseline := trace.NewTrajectory("rr003-session", "fixture var spine check", string(cfg.Profile))
	baseline.ArtifactRefs = []trace.Ref{artifactRef}
	baseline.ValidationRefs = []trace.Ref{profile.ConfigRef(cfg), harnessRef}
	baseline.OperatorPath = []trace.Operator{trace.OperatorClassify, trace.OperatorVerify, trace.OperatorStage, trace.OperatorSummarize}
	baseline = trace.FinalizeTrajectory(baseline, trace.StableHash("rr003", "pass"), "fixture_pass", "pass", baseline.StartedAt.Add(time.Millisecond), trace.CostSummary{})

	replayCase, err := replay.CaseFromTrajectory(
		"rr003-happy-case",
		"rr003 happy path replay",
		baseline,
		replay.CandidateSpec{ID: "rr003-candidate", Kind: replay.CandidateKindSkill, Version: "fixture"},
		replay.Expectations{
			RequiredOperators:  []trace.Operator{trace.OperatorVerify, trace.OperatorStage},
			ForbiddenOperators: []trace.Operator{trace.OperatorExecute, trace.OperatorPromote},
		},
		map[string]string{"scenario": "happy_path"},
	)
	if err != nil {
		t.Fatalf("build replay case: %v", err)
	}
	replayResult, err := (replay.Runner{}).Run(ctx, replayCase)
	if err != nil {
		t.Fatalf("run fixture replay: %v", err)
	}
	if replayResult.Verdict != replay.VerdictPass {
		t.Fatalf("happy path replay verdict=%s, want pass", replayResult.Verdict)
	}
	replayRef := trace.NewRef("replay_result", "replay:"+replayResult.CaseID, trace.StableHash(replayResult.CaseID, string(replayResult.Verdict)), 1)

	proposed := statelock.ProposedStateFromReplayResult(replayResult, statelock.ScopeArtifact, "rr003-candidate", statelock.PolicyMatched)
	proposed.EvidenceRefs = []trace.Ref{harnessRef, replayRef}
	stateLock := (&statelock.Evaluator{Store: statelock.NewMemoryStore()}).Check(ctx, proposed)
	if stateLock.Status != statelock.CheckStatusAllowed {
		t.Fatalf("happy path statelock status=%s err=%v, want allowed", stateLock.Status, stateLock.Err)
	}
	stateLockRef := trace.NewRef("statelock_check", "statelock:"+string(stateLock.Status), trace.StableHash(string(stateLock.Status), string(stateLock.PolicyState), string(stateLock.Risk)), int64(len(stateLock.Conflicts)))

	assessment := profile.EvaluateCapability(cfg, profile.CapabilitySkillStage)
	assessment.EvidenceRefs = []trace.Ref{profile.ConfigRef(cfg), harnessRef, replayRef, stateLockRef}
	records := []evidence.Record{
		(evidence.Record{
			Kind:         evidence.KindHarnessReport,
			Status:       evidence.StatusPass,
			Subject:      harnessReport.ArtifactID,
			Summary:      "fixture harness report passed",
			PolicyState:  evidence.PolicyMatched,
			Risk:         evidence.RiskMedium,
			Authority:    evidence.AuthorityPolicyMatch,
			EvidenceRefs: []trace.Ref{harnessRef},
		}).Normalized(),
		(evidence.Record{
			Kind:         evidence.KindReplayResult,
			Status:       evidence.StatusPass,
			Subject:      replayResult.CaseID,
			Summary:      "fixture replay passed without regression",
			PolicyState:  evidence.PolicyMatched,
			Risk:         evidence.RiskLow,
			Authority:    evidence.AuthorityPolicyMatch,
			EvidenceRefs: []trace.Ref{replayRef},
		}).Normalized(),
		(evidence.Record{
			Kind:         evidence.KindStateLock,
			Status:       evidence.StatusPass,
			Subject:      string(stateLock.Status),
			Summary:      "fixture statelock check allowed",
			PolicyState:  evidence.PolicyMatched,
			Risk:         evidence.RiskLow,
			Authority:    evidence.AuthorityPolicyMatch,
			EvidenceRefs: []trace.Ref{stateLockRef},
		}).Normalized(),
	}
	receipt := evidence.Receipt{
		Records:      records,
		Assessment:   assessment,
		Status:       evidence.StatusPass,
		Summary:      "RR-003 happy path fixture evidence linked",
		EvidenceRefs: []trace.Ref{profile.ConfigRef(cfg), harnessRef, replayRef, stateLockRef},
		Metadata: map[string]string{
			"scenario":      "happy_path",
			"rollback_path": "fixture",
			"disable_path":  "fixture",
		},
	}.Normalized()
	if err := receipt.Validate(); err != nil {
		t.Fatalf("happy path receipt invalid: %v", err)
	}
	receiptRef := evidence.ReceiptRef(receipt)

	stage, err := (skillruntime.Facade{Store: skillruntime.NewMemoryStore()}).Stage(ctx, skillruntime.Request{
		Candidate: skillruntime.Candidate{
			Name:           "rr003 fixture skill",
			Source:         "local fixture",
			Risk:           evidence.RiskMedium,
			OperationClass: evidence.SensitiveSelfmodPromotion,
			Profile:        cfg.Profile,
			ArtifactRefs:   []trace.Ref{artifactRef},
			EvidenceRefs:   []trace.Ref{receiptRef},
		},
		Config:          cfg,
		HarnessReport:   &harnessReport,
		ReplayResult:    &replayResult,
		StateLockResult: &stateLock,
		EvidenceRefs:    []trace.Ref{profile.ConfigRef(cfg), harnessRef, replayRef, stateLockRef, receiptRef},
		Metadata: map[string]string{
			"scenario":      "happy_path",
			"rollback_path": "fixture",
			"disable_path":  "fixture",
		},
	})
	if err != nil {
		t.Fatalf("stage fixture candidate: %v", err)
	}
	if stage.Status != skillruntime.StatusStaged {
		t.Fatalf("happy path skillruntime status=%s warnings=%v, want staged", stage.Status, stage.Warnings)
	}
	if err := stage.Receipt.Validate(); err != nil {
		t.Fatalf("stage receipt invalid: %v", err)
	}
	if len(stage.ValidationRefs) < 4 {
		t.Fatalf("stage validation refs=%d, want trace/ref linkage", len(stage.ValidationRefs))
	}

	return rr003HappyFixture{
		Harness:    harnessReport,
		HarnessRef: harnessRef,
		Replay:     replayResult,
		ReplayRef:  replayRef,
		StateLock:  stateLock,
		Receipt:    receipt,
		Stage:      stage,
	}
}

func rr003NegativeMissingEvidence(t *testing.T, cfg profile.Config) {
	t.Helper()

	result := skillruntime.Evaluate(skillruntime.Request{
		Operation: skillruntime.OperationStage,
		Candidate: skillruntime.Candidate{
			Name:           "rr003 missing evidence candidate",
			Source:         "local fixture",
			Risk:           evidence.RiskMedium,
			OperationClass: evidence.SensitiveSelfmodPromotion,
			Profile:        cfg.Profile,
		},
		Config: cfg,
	})
	if result.Status != skillruntime.StatusApprovalRequired {
		t.Fatalf("missing evidence status=%s warnings=%v, want approval_required", result.Status, result.Warnings)
	}
	joined := strings.Join(result.Warnings, "|")
	for _, want := range []string{"missing harness report", "missing replay result", "missing state lock check"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing evidence warnings=%v, want %q", result.Warnings, want)
		}
	}
	if err := result.Receipt.Validate(); err != nil {
		t.Fatalf("missing evidence receipt invalid: %v", err)
	}
	t.Logf("RR003_SCENARIO name=missing_evidence classification=PASS")
	t.Logf("RR003_STORY missing_evidence: local operator tries to stage without harness, replay, and statelock evidence")
	t.Logf("RR003_EXPECTED missing_evidence: not silently allowed; approval or config required with receipted warnings")
	t.Logf("RR003_ACTUAL missing_evidence: status=%s decision=%s warnings=%s receipt=%s", result.Status, result.Decision, joined, result.Receipt.ID)
}

func rr003NegativeVectorTruth(t *testing.T, cfg profile.Config) {
	t.Helper()

	assessment := profile.EvaluateCapability(cfg, profile.CapabilityVectorAssertTruth)
	if assessment.Status != evidence.StatusFail {
		t.Fatalf("vector_assert_truth status=%s, want fail", assessment.Status)
	}
	if assessment.Decision != evidence.DecisionDenySensitive {
		t.Fatalf("vector_assert_truth decision=%s, want deny_sensitive", assessment.Decision)
	}
	if assessment.ReasonCode != "vector_assert_truth_forbidden" {
		t.Fatalf("vector_assert_truth reason=%s, want vector_assert_truth_forbidden", assessment.ReasonCode)
	}
	receipt := evidence.Receipt{
		Assessment: assessment,
		Status:     evidence.StatusFail,
		Summary:    "vector_assert_truth remains forbidden as authority",
		Records: []evidence.Record{(evidence.Record{
			Kind:        evidence.KindHardInvariant,
			Status:      evidence.StatusFail,
			Subject:     "vector_assert_truth",
			Summary:     "vector retrieval cannot act as truth authority",
			PolicyState: assessment.PolicyState,
			Risk:        assessment.Risk,
			Authority:   assessment.Authority,
		}).Normalized()},
	}.Normalized()
	if err := receipt.Validate(); err != nil {
		t.Fatalf("vector truth receipt invalid: %v", err)
	}
	t.Logf("RR003_SCENARIO name=vector_truth_authority classification=PASS")
	t.Logf("RR003_STORY vector_truth_authority: local operator attempts to use vector_assert_truth as authority")
	t.Logf("RR003_EXPECTED vector_truth_authority: hard invariant denies sensitive action and records reason")
	t.Logf("RR003_ACTUAL vector_truth_authority: status=%s decision=%s reason=%s receipt=%s", assessment.Status, assessment.Decision, assessment.ReasonCode, receipt.ID)
}
