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

func TestRR004PolicyStatelockSmoke(t *testing.T) {
	ctx := context.Background()
	cfg := rr004FixtureConfig()

	sensitive := rr004PolicyAbsenceSensitiveAction(t, cfg)
	lowRisk := rr004PolicyAbsenceLowRiskAction(t)
	conflict := rr004StateLockConflict(t, ctx, lowRisk.ReceiptRef)
	possible, confirmed, inconclusive := rr004ParadoxReport(t, conflict.Result)
	linkage := rr004SkillruntimeProfileLinkage(t, cfg, conflict.Result, sensitive.ReceiptRef)

	t.Logf("RR004_SUMMARY scenarios=5 sensitive=%s low_risk=%s statelock=%s paradox_possible=%s paradox_confirmed=%s paradox_inconclusive=%s skillruntime=%s", sensitive.Classification, lowRisk.Classification, conflict.Classification, possible.Status, confirmed.Status, inconclusive.Status, linkage.Classification)
	t.Logf("RR004_OPERATOR_SUMMARY no policy is not permission; sensitive paths require approval or denial, low-risk paths must be explicit, conflicts surface with refs, paradox remediation stays descriptive")
}

type rr004ScenarioResult struct {
	Classification string
	Assessment     evidence.Assessment
	Receipt        evidence.Receipt
	ReceiptRef     trace.Ref
	Result         statelock.CheckResult
	SkillResult    skillruntime.Result
}

func rr004FixtureConfig() profile.Config {
	cfg := profile.DefaultConfig(profile.ProfileExpert)
	cfg.ConfiguredCapabilities = map[profile.Capability]bool{
		profile.CapabilitySkillStage:   true,
		profile.CapabilitySkillPromote: true,
	}
	cfg.Approval.RequireApprovalForSelfmod = false
	cfg.Approval.RequireApprovalForPolicyAbsence = true
	return cfg.Normalized()
}

func rr004PolicyAbsenceSensitiveAction(t *testing.T, cfg profile.Config) rr004ScenarioResult {
	t.Helper()

	assessment := evidence.Evaluate(evidence.Assessment{
		PolicyState:    evidence.PolicyNoMatch,
		Risk:           evidence.RiskSensitive,
		Operation:      string(evidence.SensitiveNetworkEgress),
		SensitiveClass: evidence.SensitiveNetworkEgress,
		Authority:      evidence.AuthorityNone,
	})
	if assessment.Decision == evidence.DecisionAllowLowRisk || assessment.Status == evidence.StatusPass {
		t.Fatalf("policy absence sensitive action silently allowed: status=%s decision=%s", assessment.Status, assessment.Decision)
	}
	if assessment.Decision != evidence.DecisionRequireApproval {
		t.Fatalf("sensitive absent policy decision=%s, want require_approval", assessment.Decision)
	}
	if assessment.ReasonCode != evidence.ReasonPolicyAbsentSensitive {
		t.Fatalf("sensitive absent policy reason=%s, want %s", assessment.ReasonCode, evidence.ReasonPolicyAbsentSensitive)
	}

	profileAssessment := profile.EvaluateCapability(cfg, profile.CapabilityNetworkEgress)
	if profileAssessment.Decision == evidence.DecisionAllowLowRisk || profileAssessment.Status == evidence.StatusPass {
		t.Fatalf("profile network egress under absent policy silently allowed: status=%s decision=%s", profileAssessment.Status, profileAssessment.Decision)
	}

	receipt := evidence.Receipt{
		Assessment: assessment,
		Status:     evidence.StatusWarn,
		Summary:    "policy_absence_sensitive_action blocked: no policy is not permission",
		Records: []evidence.Record{(evidence.Record{
			Kind:        evidence.KindPolicyAbsence,
			Status:      evidence.StatusWarn,
			Subject:     string(evidence.SensitiveNetworkEgress),
			Summary:     "network egress has no matching policy; request approval or provide policy before proceeding",
			PolicyState: evidence.PolicyNoMatch,
			Risk:        evidence.RiskSensitive,
			Authority:   evidence.AuthorityNone,
		}).Normalized()},
		Metadata: map[string]string{
			"scenario":     "policy_absence_sensitive_action",
			"operator":     "attempted network egress",
			"next_step":    "provide matching policy or request explicit approval",
			"profile_link": profileAssessment.ID,
		},
	}.Normalized()
	if err := receipt.Validate(); err != nil {
		t.Fatalf("sensitive absence receipt invalid: %v", err)
	}

	ref := evidence.ReceiptRef(receipt)
	t.Logf("RR004_SCENARIO name=policy_absence_sensitive_action classification=PASS")
	t.Logf("RR004_STORY policy_absence_sensitive_action: operator attempts network egress with no matching policy")
	t.Logf("RR004_EXPECTED policy_absence_sensitive_action: no silent allow; no policy is not permission; approval or policy required")
	t.Logf("RR004_ACTUAL policy_absence_sensitive_action: status=%s decision=%s reason=%s profile_status=%s profile_decision=%s", assessment.Status, assessment.Decision, assessment.ReasonCode, profileAssessment.Status, profileAssessment.Decision)
	t.Logf("RR004_EVIDENCE policy_absence_sensitive_action: receipt=%s ref=%s next_step=%s", receipt.ID, ref.Ref, receipt.Metadata["next_step"])
	t.Logf("RR004_WEAK_SEAM policy_absence_sensitive_action: profile exposes approval_required but does not name a concrete policy file path")

	return rr004ScenarioResult{Classification: "PASS", Assessment: assessment, Receipt: receipt, ReceiptRef: ref}
}

func rr004PolicyAbsenceLowRiskAction(t *testing.T) rr004ScenarioResult {
	t.Helper()

	assessment := evidence.Evaluate(evidence.Assessment{
		PolicyState:     evidence.PolicyNoMatch,
		Risk:            evidence.RiskLow,
		Operation:       "manifest_count",
		ExplicitLowRisk: true,
		Authority:       evidence.AuthorityNone,
	})
	if assessment.Decision != evidence.DecisionAllowLowRisk || assessment.Status != evidence.StatusPass {
		t.Fatalf("explicit low-risk absent policy result status=%s decision=%s, want pass allow_low_risk", assessment.Status, assessment.Decision)
	}
	if assessment.ReasonCode != evidence.ReasonLowRiskExplicitAbsent {
		t.Fatalf("low-risk absent reason=%s, want %s", assessment.ReasonCode, evidence.ReasonLowRiskExplicitAbsent)
	}

	implicit := evidence.Evaluate(evidence.Assessment{
		PolicyState: evidence.PolicyNoMatch,
		Risk:        evidence.RiskLow,
		Operation:   "manifest_count",
		Authority:   evidence.AuthorityNone,
	})
	if implicit.Decision == evidence.DecisionAllowLowRisk {
		t.Fatalf("implicit low-risk classification allowed without explicit low-risk marker")
	}

	receipt := evidence.Receipt{
		Assessment: assessment,
		Status:     evidence.StatusPass,
		Summary:    "policy_absence_low_risk_action allowed only because risk class is explicit",
		Records: []evidence.Record{(evidence.Record{
			Kind:        evidence.KindValidationReport,
			Status:      evidence.StatusPass,
			Subject:     "manifest_count",
			Summary:     "read-only local manifest count classified as explicit low-risk",
			PolicyState: evidence.PolicyNoMatch,
			Risk:        evidence.RiskLow,
			Authority:   evidence.AuthorityNone,
		}).Normalized()},
		Metadata: map[string]string{
			"scenario":       "policy_absence_low_risk_action",
			"risk_class":     "explicit_low_risk",
			"not_permission": "no broad permission inferred from absent policy",
		},
	}.Normalized()
	if err := receipt.Validate(); err != nil {
		t.Fatalf("low-risk absence receipt invalid: %v", err)
	}

	ref := evidence.ReceiptRef(receipt)
	t.Logf("RR004_SCENARIO name=policy_absence_low_risk_action classification=PASS")
	t.Logf("RR004_STORY policy_absence_low_risk_action: operator requests read-only manifest count while no matching policy exists")
	t.Logf("RR004_EXPECTED policy_absence_low_risk_action: may proceed only as explicit low-risk; no broad permission inferred")
	t.Logf("RR004_ACTUAL policy_absence_low_risk_action: explicit_status=%s explicit_decision=%s implicit_decision=%s", assessment.Status, assessment.Decision, implicit.Decision)
	t.Logf("RR004_EVIDENCE policy_absence_low_risk_action: receipt=%s ref=%s note=%s", receipt.ID, ref.Ref, receipt.Metadata["not_permission"])
	t.Logf("RR004_WEAK_SEAM policy_absence_low_risk_action: distinction relies on explicit_low_risk bit rather than richer operator explanation")

	return rr004ScenarioResult{Classification: "PASS", Assessment: assessment, Receipt: receipt, ReceiptRef: ref}
}

func rr004StateLockConflict(t *testing.T, ctx context.Context, receiptRef trace.Ref) rr004ScenarioResult {
	t.Helper()

	store := statelock.NewMemoryStore()
	lock := statelock.Lock{
		Scope:        statelock.ScopeRepository,
		Dimension:    statelock.DimensionPermissionScope,
		Subject:      "release/pr-019",
		StateHash:    "perm:read",
		Status:       statelock.StatusActive,
		Source:       statelock.SourceHarness,
		PolicyState:  statelock.PolicyMatched,
		EvidenceRefs: []trace.Ref{receiptRef},
	}.Normalized()
	if err := store.SaveLock(ctx, lock); err != nil {
		t.Fatalf("save fixture lock: %v", err)
	}

	proposed := statelock.ProposedState{
		Scope:        statelock.ScopeRepository,
		Dimension:    statelock.DimensionPermissionScope,
		Subject:      "release/pr-019",
		StateHash:    "perm:admin",
		PolicyState:  statelock.PolicyMatched,
		Risk:         statelock.RiskLow,
		EvidenceRefs: []trace.Ref{receiptRef},
		Metadata: map[string]string{
			"scenario": "statelock_conflict",
		},
	}
	result := (&statelock.Evaluator{Store: store}).Check(ctx, proposed)
	if result.Status != statelock.CheckStatusConflict {
		t.Fatalf("statelock result=%s err=%v, want conflict", result.Status, result.Err)
	}
	if len(result.Conflicts) == 0 {
		t.Fatalf("statelock conflict missing conflict list")
	}
	if result.Paradox == nil {
		t.Fatalf("statelock conflict missing paradox report")
	}
	joinedReasons := rr004ConflictReasons(result.Conflicts)
	if !strings.Contains(joinedReasons, statelock.ReasonPermissionScopeWidened) {
		t.Fatalf("statelock conflict reasons=%s, want permission_scope_widened", joinedReasons)
	}
	if result.Paradox.Status == statelock.ParadoxConfirmed {
		t.Fatalf("permission widening fixture should be possible, not confirmed")
	}

	receipt := evidence.Receipt{
		Assessment: evidence.Evaluate(evidence.Assessment{
			PolicyState: evidence.PolicyMatched,
			Risk:        evidence.RiskLow,
			Operation:   "statelock_conflict",
			Authority:   evidence.AuthorityPolicyMatch,
		}),
		Status:  evidence.StatusWarn,
		Summary: "statelock_conflict blocked unsafe widening",
		Records: []evidence.Record{(evidence.Record{
			Kind:         evidence.KindStateLock,
			Status:       evidence.StatusWarn,
			Subject:      "release/pr-019",
			Summary:      "permission widening conflict visible to operator",
			PolicyState:  evidence.PolicyMatched,
			Risk:         evidence.RiskLow,
			Authority:    evidence.AuthorityPolicyMatch,
			EvidenceRefs: []trace.Ref{statelock.ParadoxRef(*result.Paradox)},
		}).Normalized()},
		EvidenceRefs: []trace.Ref{statelock.ParadoxRef(*result.Paradox)},
		Metadata: map[string]string{
			"scenario": "statelock_conflict",
			"reasons":  joinedReasons,
		},
	}.Normalized()
	if err := receipt.Validate(); err != nil {
		t.Fatalf("statelock conflict receipt invalid: %v", err)
	}

	t.Logf("RR004_SCENARIO name=statelock_conflict classification=PASS")
	t.Logf("RR004_STORY statelock_conflict: operator attempts to widen repository permission scope while an active lock records read-only state")
	t.Logf("RR004_EXPECTED statelock_conflict: conflict detected; unsafe path not silently allowed; severity and reason visible")
	t.Logf("RR004_ACTUAL statelock_conflict: status=%s conflicts=%d reasons=%s paradox=%s", result.Status, len(result.Conflicts), joinedReasons, result.Paradox.Status)
	t.Logf("RR004_EVIDENCE statelock_conflict: receipt=%s paradox_ref=%s remediation_count=%d", receipt.ID, statelock.ParadoxRef(*result.Paradox).Ref, len(result.Recommendations))
	t.Logf("RR004_WEAK_SEAM statelock_conflict: conflict summaries are reason-code based and could use richer operator text")

	return rr004ScenarioResult{Classification: "PASS", Receipt: receipt, ReceiptRef: evidence.ReceiptRef(receipt), Result: result}
}

func rr004ParadoxReport(t *testing.T, possibleResult statelock.CheckResult) (statelock.ParadoxReport, statelock.ParadoxReport, statelock.ParadoxReport) {
	t.Helper()
	if possibleResult.Paradox == nil {
		t.Fatalf("possible paradox input missing report")
	}
	possible := possibleResult.Paradox.Normalized()
	if possible.Status != statelock.ParadoxPossible {
		t.Fatalf("possible paradox status=%s, want possible", possible.Status)
	}
	if strings.Contains(strings.ToLower(possible.Summary), "no valid path") {
		t.Fatalf("possible paradox overclaims summary: %s", possible.Summary)
	}

	confirmedConflict := statelock.DetectConflicts(nil, statelock.ProposedState{
		Scope:       statelock.ScopeRelease,
		Dimension:   statelock.DimensionPolicyState,
		Subject:     "release_publish",
		StateHash:   "publish",
		PolicyState: statelock.PolicyNoMatch,
		Risk:        statelock.RiskSensitive,
	})
	confirmedResult := (&statelock.Evaluator{}).Check(context.Background(), statelock.ProposedState{
		Scope:       statelock.ScopeRelease,
		Dimension:   statelock.DimensionPolicyState,
		Subject:     "release_publish",
		StateHash:   "publish",
		PolicyState: statelock.PolicyNoMatch,
		Risk:        statelock.RiskSensitive,
	})
	if len(confirmedConflict) == 0 || confirmedResult.Paradox == nil {
		t.Fatalf("confirmed paradox fixture failed to produce conflict/report")
	}
	confirmed := confirmedResult.Paradox.Normalized()
	if confirmed.Status != statelock.ParadoxConfirmed {
		t.Fatalf("confirmed paradox status=%s, want confirmed", confirmed.Status)
	}
	if !strings.Contains(strings.ToLower(confirmed.Summary), "no valid path") {
		t.Fatalf("confirmed paradox summary did not clearly state no valid path: %s", confirmed.Summary)
	}

	now := time.Date(2026, 5, 17, 0, 0, 0, 0, time.UTC)
	inconclusive := statelock.ParadoxReport{
		Status:      statelock.ParadoxInconclusive,
		Summary:     "insufficient data to confirm policy and lock compatibility",
		PolicyState: statelock.PolicyUnavailable,
		Risk:        statelock.RiskSensitive,
		StartedAt:   now,
		FinishedAt:  now,
		Remediation: []statelock.Remediation{
			{Kind: statelock.RemediationProvidePolicy, Description: "provide missing policy evidence"},
			{Kind: statelock.RemediationRequestApproval, Description: "request explicit approval before execution"},
		},
		Metadata: map[string]string{
			"scenario": "paradox_report",
		},
	}.Normalized()
	if err := inconclusive.Validate(); err != nil {
		t.Fatalf("inconclusive paradox invalid: %v", err)
	}
	if strings.Contains(strings.ToLower(inconclusive.Summary), "no valid path") {
		t.Fatalf("inconclusive paradox overclaims summary: %s", inconclusive.Summary)
	}

	t.Logf("RR004_SCENARIO name=paradox_report classification=PASS")
	t.Logf("RR004_STORY paradox_report: operator receives possible, confirmed, and inconclusive paradox summaries from fixture constraints")
	t.Logf("RR004_EXPECTED paradox_report: confirmed only for critical constraints; possible/inconclusive do not overclaim no valid path; remediation descriptive only")
	t.Logf("RR004_ACTUAL paradox_report: possible=%s confirmed=%s inconclusive=%s remediation_possible=%d remediation_confirmed=%d remediation_inconclusive=%d", possible.Status, confirmed.Status, inconclusive.Status, len(possible.Remediation), len(confirmed.Remediation), len(inconclusive.Remediation))
	t.Logf("RR004_EVIDENCE paradox_report: possible_ref=%s confirmed_ref=%s inconclusive_ref=%s", statelock.ParadoxRef(possible).Ref, statelock.ParadoxRef(confirmed).Ref, statelock.ParadoxRef(inconclusive).Ref)
	t.Logf("RR004_WEAK_SEAM paradox_report: remediation is descriptive; no executor hook exists in this fixture, which is intentional for RR-004")

	return possible, confirmed, inconclusive
}

func rr004SkillruntimeProfileLinkage(t *testing.T, cfg profile.Config, conflict statelock.CheckResult, receiptRef trace.Ref) rr004ScenarioResult {
	t.Helper()

	harnessReport := harness.Report{HarnessID: "rr004.fixture.harness", ArtifactID: "rr004-artifact", Status: harness.StatusPass}.Normalized()
	replayResult := replay.Result{CaseID: "rr004-replay", CandidateID: "rr004-candidate", Verdict: replay.VerdictPass, Duration: time.Millisecond}
	paradoxRef := trace.Ref{}
	if conflict.Paradox != nil {
		paradoxRef = statelock.ParadoxRef(*conflict.Paradox)
	}
	result := skillruntime.Evaluate(skillruntime.Request{
		Operation: skillruntime.OperationPromote,
		Candidate: skillruntime.Candidate{
			Name:           "rr004 fixture candidate",
			Source:         "local fixture",
			Risk:           evidence.RiskSensitive,
			OperationClass: evidence.SensitiveSelfmodPromotion,
			Profile:        cfg.Profile,
			EvidenceRefs:   []trace.Ref{receiptRef, paradoxRef},
			Metadata: map[string]string{
				"rollback_path": "fixture rollback",
				"disable_path":  "fixture disable",
			},
		},
		Config:          cfg,
		HarnessReport:   &harnessReport,
		ReplayResult:    &replayResult,
		StateLockResult: &conflict,
		EvidenceRefs:    []trace.Ref{receiptRef, paradoxRef},
		Metadata: map[string]string{
			"scenario":      "skillruntime_profile_linkage",
			"rollback_path": "fixture rollback",
			"disable_path":  "fixture disable",
		},
	})
	if result.Status != skillruntime.StatusDenied {
		t.Fatalf("skillruntime conflict linkage status=%s warnings=%v, want denied", result.Status, result.Warnings)
	}
	joinedWarnings := strings.Join(result.Warnings, "|")
	if !strings.Contains(joinedWarnings, "state lock conflict") {
		t.Fatalf("skillruntime warnings=%v, want state lock conflict", result.Warnings)
	}
	if len(result.ValidationRefs) == 0 {
		t.Fatalf("skillruntime result missing validation refs")
	}
	if err := result.Receipt.Validate(); err != nil {
		t.Fatalf("skillruntime linkage receipt invalid: %v", err)
	}

	t.Logf("RR004_SCENARIO name=skillruntime_profile_linkage classification=PASS")
	t.Logf("RR004_STORY skillruntime_profile_linkage: operator tries promotion with conflict/paradox evidence attached")
	t.Logf("RR004_EXPECTED skillruntime_profile_linkage: adverse statelock/paradox evidence cannot silently pass; receipt/ref linkage present")
	t.Logf("RR004_ACTUAL skillruntime_profile_linkage: status=%s decision=%s warnings=%s refs=%d receipt=%s", result.Status, result.Decision, joinedWarnings, len(result.ValidationRefs), result.Receipt.ID)
	t.Logf("RR004_EVIDENCE skillruntime_profile_linkage: receipt_ref=%s paradox_ref=%s profile=%s", evidence.ReceiptRef(result.Receipt).Ref, paradoxRef.Ref, cfg.Profile)
	t.Logf("RR004_WEAK_SEAM skillruntime_profile_linkage: result warns about state lock conflict but does not inline all conflict reasons")

	return rr004ScenarioResult{Classification: "PASS", SkillResult: result, Receipt: result.Receipt, ReceiptRef: evidence.ReceiptRef(result.Receipt)}
}

func rr004ConflictReasons(conflicts []statelock.Conflict) string {
	reasons := make([]string, 0, len(conflicts))
	for i := range conflicts {
		if conflicts[i].ReasonCode == "" {
			continue
		}
		reasons = append(reasons, conflicts[i].ReasonCode)
	}
	return strings.Join(reasons, ",")
}
