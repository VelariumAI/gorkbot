package statelock

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/velariumai/gorkbot/pkg/replay"
	"github.com/velariumai/gorkbot/pkg/trace"
)

func TestParadoxReportGeneration(t *testing.T) {
	store := NewMemoryStore()
	if err := store.SaveLock(context.Background(), Lock{
		ID:          "l1",
		Scope:       ScopeWorkspace,
		Dimension:   DimensionArtifact,
		Subject:     "subject-a",
		StateHash:   "old",
		Status:      StatusActive,
		PolicyState: PolicyMatched,
		CreatedAt:   time.Now().UTC(),
	}); err != nil {
		t.Fatalf("save lock: %v", err)
	}
	e := &Evaluator{Store: store}
	result := e.Check(context.Background(), ProposedState{
		Scope:       ScopeWorkspace,
		Dimension:   DimensionArtifact,
		Subject:     "subject-a",
		StateHash:   "new",
		PolicyState: PolicyMatched,
		Risk:        RiskMedium,
	})
	if result.Status != CheckStatusConflict {
		t.Fatalf("expected conflict, got %s", result.Status)
	}
	if result.Paradox == nil {
		t.Fatal("expected paradox report")
	}
	if result.Paradox.Status != ParadoxPossible && result.Paradox.Status != ParadoxConfirmed {
		t.Fatalf("unexpected paradox status: %s", result.Paradox.Status)
	}
}

func TestParadoxSummaryLanguageByStatus(t *testing.T) {
	if got := paradoxSummaryForStatus(ParadoxConfirmed); !strings.Contains(got, "no valid path") {
		t.Fatalf("confirmed summary must include no-valid-path language, got %q", got)
	}
	if got := paradoxSummaryForStatus(ParadoxPossible); strings.Contains(got, "no valid path") {
		t.Fatalf("possible summary must not include no-valid-path language, got %q", got)
	}
	if got := paradoxSummaryForStatus(ParadoxInconclusive); strings.Contains(got, "no valid path") {
		t.Fatalf("inconclusive summary must not include no-valid-path language, got %q", got)
	}
}

func TestBuildParadoxReportMetadataIncludesConflictCount(t *testing.T) {
	proposed := ProposedState{
		Scope:       ScopeWorkspace,
		Dimension:   DimensionArtifact,
		Subject:     "subject-a",
		StateHash:   "new",
		PolicyState: PolicyMatched,
		Risk:        RiskMedium,
	}
	conflicts := []Conflict{
		{
			ReasonCode: "validation_mismatch",
			Severity:   SeverityHigh,
			Dimension:  DimensionArtifact,
			Subject:    "subject-a",
		},
		{
			ReasonCode: "policy_gap",
			Severity:   SeverityLow,
			Dimension:  DimensionArtifact,
			Subject:    "subject-a",
		},
	}
	report := buildParadoxReport(proposed, conflicts)
	if got := report.Metadata["conflict_count"]; got != "2" {
		t.Fatalf("expected conflict_count=2, got %q", got)
	}
}

func TestParadoxNoAutoRemediationExecution(t *testing.T) {
	store := NewMemoryStore()
	e := &Evaluator{Store: store}
	result := e.Check(context.Background(), ProposedState{
		Scope:       ScopeWorkspace,
		Dimension:   DimensionDecision,
		Subject:     "subject-a",
		StateHash:   "new",
		PolicyState: PolicyNoMatch,
		Risk:        RiskSensitive,
	})
	if result.Status != CheckStatusConflict {
		t.Fatalf("expected conflict status, got %s", result.Status)
	}
	if len(store.paradoxes) != 0 {
		t.Fatal("evaluator must not auto-save/execute remediation")
	}
}

func TestRemediationBounding(t *testing.T) {
	report := ParadoxReport{
		Status:      ParadoxPossible,
		Summary:     "s",
		PolicyState: PolicyNoMatch,
		Risk:        RiskSensitive,
		StartedAt:   time.Now().UTC(),
		Remediation: []Remediation{
			{Kind: RemediationProvidePolicy, Description: "one"},
			{Kind: RemediationProvidePolicy, Description: "two"},
		},
	}
	n := report.Normalized()
	if len(n.Remediation) != 2 {
		t.Fatalf("expected remediation preserved, got %d", len(n.Remediation))
	}
}

func TestNoRawSensitivePayloadInReportMetadata(t *testing.T) {
	report := ParadoxReport{
		Status:      ParadoxPossible,
		Summary:     "s",
		PolicyState: PolicyNoMatch,
		Risk:        RiskSensitive,
		StartedAt:   time.Now().UTC(),
		Metadata: map[string]string{
			"token": "super-secret",
			"safe":  "ok",
		},
	}
	n := report.Normalized()
	if got := n.Metadata["token"]; got != "[REDACTED]" {
		t.Fatalf("expected token redacted, got %q", got)
	}
	if got := n.Metadata["safe"]; got != "ok" {
		t.Fatalf("expected safe metadata retained, got %q", got)
	}
}

func TestReplayAndTraceHelpers(t *testing.T) {
	rr := replay.Result{
		CaseID:       "case-1",
		TrajectoryID: "traj-1",
		Verdict:      replay.VerdictPass,
		BaselineOutcome: replay.Outcome{
			ValidationRefs: []trace.Ref{trace.NewRef("v", "a", "h", 1)},
		},
		CandidateOutcome: replay.Outcome{
			ValidationRefs: []trace.Ref{trace.NewRef("v", "b", "h2", 1)},
		},
	}
	ps := ProposedStateFromReplayResult(rr, ScopeTrajectory, "subj", PolicyMatched)
	if ps.Dimension != DimensionValidationResult || len(ps.EvidenceRefs) == 0 {
		t.Fatalf("unexpected replay helper output: %#v", ps)
	}

	traj := trace.NewTrajectory("session-1", "objective", "mode")
	traj.EndStateHash = "end-hash"
	ps2 := ProposedStateFromTraceTrajectory(traj, ScopeTrajectory, DimensionDecision, PolicyMatched)
	if ps2.StateHash == "" || ps2.Subject == "" {
		t.Fatalf("unexpected trace helper output: %#v", ps2)
	}
}
