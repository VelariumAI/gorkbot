package statelock

import (
	"fmt"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/harness"
	"github.com/velariumai/gorkbot/pkg/replay"
	"github.com/velariumai/gorkbot/pkg/trace"
)

func LockRef(lock Lock) trace.Ref {
	n := lock.Normalized()
	return trace.NewRef("state_lock", "lock:"+n.ID, n.StateHash, int64(len(n.ValidationRefs)+len(n.EvidenceRefs)))
}

func ParadoxRef(report ParadoxReport) trace.Ref {
	n := report.Normalized()
	return trace.NewRef("paradox_report", "paradox:"+n.ID, trace.StableHash(n.Summary, string(n.Status)), int64(len(n.Conflicts)))
}

func LockFromHarnessReport(report harness.Report, scope Scope, subject string, policyState PolicyState) (Lock, error) {
	norm := report.Normalized()
	if strings.TrimSpace(norm.HarnessID) == "" || strings.TrimSpace(subject) == "" {
		return Lock{}, fmt.Errorf("%w: harness report missing identity", ErrInvalidLock)
	}
	stateHash := trace.StableHash(string(norm.Status), norm.StableID())
	lock := Lock{
		Scope:          NormalizeScope(string(scope)),
		Dimension:      DimensionValidationResult,
		Subject:        subject,
		StateHash:      stateHash,
		Status:         StatusActive,
		Source:         SourceHarness,
		ValidationRefs: []trace.Ref{norm.ValidationRef()},
		PolicyState:    normalizePolicyState(string(policyState)),
		CreatedAt:      norm.FinishedAt,
		Metadata: normalizeMetadata(map[string]string{
			"harness_id":  norm.HarnessID,
			"artifact_id": norm.ArtifactID,
			"status":      string(norm.Status),
			"validated":   "true",
		}),
	}
	if lock.CreatedAt.IsZero() {
		lock.CreatedAt = time.Now().UTC()
	}
	return lock.Normalized(), lock.Validate()
}

func ProposedStateFromReplayResult(result replay.Result, scope Scope, subject string, policyState PolicyState) ProposedState {
	norm := normalizeReplayResult(result)
	state := ProposedState{
		Scope:       NormalizeScope(string(scope)),
		Dimension:   DimensionValidationResult,
		Subject:     subjectOrFallback(subject, norm.CaseID, norm.TrajectoryID),
		StateHash:   string(norm.Verdict),
		PolicyState: normalizePolicyState(string(policyState)),
		Risk:        RiskMedium,
		EvidenceRefs: combineRefs(
			norm.BaselineOutcome.ValidationRefs,
			norm.CandidateOutcome.ValidationRefs,
		),
		Metadata: normalizeMetadata(map[string]string{
			"case_id":       norm.CaseID,
			"trajectory_id": norm.TrajectoryID,
			"verdict":       string(norm.Verdict),
		}),
	}
	if strings.EqualFold(string(norm.Verdict), string(replay.VerdictFail)) || strings.EqualFold(string(norm.Verdict), string(replay.VerdictRegression)) {
		state.Risk = RiskSensitive
	}
	if state.Risk == RiskSensitive && IsPolicyAbsent(state.PolicyState) {
		state.Metadata["policy_gap"] = "sensitive_without_policy"
	}
	return state.Normalized()
}

func ProposedStateFromTraceTrajectory(traj trace.Trajectory, scope Scope, dimension Dimension, policyState PolicyState) ProposedState {
	norm := traj.Normalized()
	stateHash := strings.TrimSpace(norm.EndStateHash)
	if stateHash == "" {
		stateHash = trace.StableHash(norm.TrajectoryID, norm.Status, norm.Outcome)
	}
	// Defaults to a low-risk replay_compare posture only. Callers handling
	// sensitive trace dimensions must override Risk, Dimension, and PolicyState.
	// This helper is side-effect-free and does not make authority decisions.
	return ProposedState{
		Scope:        NormalizeScope(string(scope)),
		Dimension:    NormalizeDimension(string(dimension)),
		Subject:      subjectOrFallback(norm.TrajectoryID, norm.SessionID, norm.ObjectiveHash),
		StateHash:    stateHash,
		PolicyState:  normalizePolicyState(string(policyState)),
		Risk:         ClassifyOperationRisk("replay_compare"),
		EvidenceRefs: normalizeRefs(norm.ValidationRefs),
		Metadata: normalizeMetadata(map[string]string{
			"trajectory_id": norm.TrajectoryID,
			"status":        norm.Status,
			"outcome":       norm.Outcome,
		}),
	}.Normalized()
}

func subjectOrFallback(primary, fallbackA, fallbackB string) string {
	for _, v := range []string{primary, fallbackA, fallbackB} {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return "unknown_subject"
}

func normalizeReplayResult(in replay.Result) replay.Result {
	out := in
	out.CaseID = trace.RedactString(strings.TrimSpace(out.CaseID), 256)
	out.TrajectoryID = trace.RedactString(strings.TrimSpace(out.TrajectoryID), 256)
	out.Verdict = replay.Verdict(strings.ToLower(strings.TrimSpace(string(out.Verdict))))
	out.BaselineOutcome.ValidationRefs = normalizeRefs(out.BaselineOutcome.ValidationRefs)
	out.CandidateOutcome.ValidationRefs = normalizeRefs(out.CandidateOutcome.ValidationRefs)
	return out
}
