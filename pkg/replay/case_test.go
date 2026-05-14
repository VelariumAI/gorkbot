package replay

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/velariumai/gorkbot/pkg/trace"
)

func testTrajectory(status, outcome string, durationMS, costMicros int64) trace.Trajectory {
	start := time.Unix(1700000000, 0).UTC()
	finish := start.Add(time.Duration(durationMS) * time.Millisecond)
	return trace.Trajectory{
		TrajectoryID:   "traj-test-1",
		StartedAt:      start,
		FinishedAt:     finish,
		Duration:       durationMS,
		Status:         status,
		Outcome:        outcome,
		OperatorPath:   []trace.Operator{trace.OperatorPlan, trace.OperatorExecute},
		ValidationRefs: []trace.Ref{trace.NewRef("validation", "val-1", "", 0)},
		ReceiptRefs:    []trace.Ref{trace.NewRef("receipt", "rcpt-1", "", 0)},
		CostSummary: trace.CostSummary{
			CostEstimate: trace.CostEstimate{TotalMicros: costMicros},
		},
	}
}

func TestCaseFromTrajectoryAndValidation(t *testing.T) {
	traj := testTrajectory("success", "success", 100, 1000)
	cand := CandidateSpec{ID: "candidate-1", Kind: CandidateKindPolicy}

	c, err := NewCaseFromTrajectory("case-name", traj, cand, Expectations{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.ID == "" {
		t.Fatalf("expected generated case id")
	}
	if c.TrajectoryID != traj.TrajectoryID {
		t.Fatalf("expected trajectory id %q, got %q", traj.TrajectoryID, c.TrajectoryID)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("expected valid case, got %v", err)
	}
}

func TestCaseValidationMissingTrajectory(t *testing.T) {
	traj := testTrajectory("success", "success", 100, 1000)
	traj.TrajectoryID = ""
	cand := CandidateSpec{ID: "candidate-1", Kind: CandidateKindPolicy}

	_, err := NewCaseFromTrajectory("case-name", traj, cand, Expectations{})
	if err == nil {
		t.Fatalf("expected missing trajectory error")
	}
	if !strings.Contains(err.Error(), ErrMissingTrajectory.Error()) {
		t.Fatalf("expected ErrMissingTrajectory, got %v", err)
	}
}

func TestCaseMetadataBoundedAndRedacted(t *testing.T) {
	traj := testTrajectory("success", "success", 100, 1000)
	cand := CandidateSpec{
		ID:   "candidate-1",
		Kind: CandidateKindPolicy,
		Metadata: map[string]string{
			"api_key": "secret",
		},
	}
	meta := map[string]string{
		"session_token": "abcd",
	}

	c, err := CaseFromTrajectory("", "case-name", traj, cand, Expectations{}, meta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := c.Candidate.Metadata["api_key"]; got != "[REDACTED]" {
		t.Fatalf("expected redacted candidate metadata, got %q", got)
	}
	if got := c.Metadata["session_token"]; got != "[REDACTED]" {
		t.Fatalf("expected redacted case metadata, got %q", got)
	}
}

func TestRunnerInvalidCaseHandling(t *testing.T) {
	r := Runner{}
	_, err := r.Run(context.Background(), Case{})
	if err == nil {
		t.Fatalf("expected invalid case error")
	}
	if !strings.Contains(err.Error(), ErrInvalidCase.Error()) {
		t.Fatalf("expected ErrInvalidCase, got %v", err)
	}
}
