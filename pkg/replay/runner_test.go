package replay

import (
	"context"
	"errors"
	"testing"

	"github.com/velariumai/gorkbot/pkg/trace"
)

func TestRunnerWithNoopEvaluator(t *testing.T) {
	base := testTrajectory("success", "success", 100, 1000)
	c, err := NewCaseFromTrajectory("runner", base, CandidateSpec{ID: "runner-c1", Kind: CandidateKindPolicy}, Expectations{})
	if err != nil {
		t.Fatalf("unexpected case error: %v", err)
	}

	r := Runner{Evaluator: NoopEvaluator{}}
	res, err := r.Run(context.Background(), c)
	if err != nil {
		t.Fatalf("unexpected run error: %v", err)
	}
	if res.Verdict != VerdictPass {
		t.Fatalf("expected pass verdict, got %s", res.Verdict)
	}
}

func TestRunnerWithStaticEvaluatorRegression(t *testing.T) {
	base := testTrajectory("success", "success", 100, 1000)
	cand := testTrajectory("failed", "failed", 100, 1000)
	cand.TrajectoryID = "traj-runner-c2"

	c, err := NewCaseFromTrajectory("runner-reg", base, CandidateSpec{ID: "runner-c2", Kind: CandidateKindPolicy}, Expectations{})
	if err != nil {
		t.Fatalf("unexpected case error: %v", err)
	}

	r := Runner{Evaluator: StaticEvaluator{Outcome: CandidateOutcome{Trajectory: cand}}}
	res, err := r.Run(context.Background(), c)
	if err != nil {
		t.Fatalf("unexpected run error: %v", err)
	}
	if res.Verdict != VerdictRegression {
		t.Fatalf("expected regression verdict, got %s", res.Verdict)
	}
}

func TestRunnerInconclusiveOnEvaluatorError(t *testing.T) {
	base := testTrajectory("success", "success", 100, 1000)
	c, err := NewCaseFromTrajectory("runner-inc", base, CandidateSpec{ID: "runner-c3", Kind: CandidateKindPolicy}, Expectations{})
	if err != nil {
		t.Fatalf("unexpected case error: %v", err)
	}

	r := Runner{Evaluator: StaticEvaluator{Err: ErrInconclusive}}
	res, err := r.Run(context.Background(), c)
	if !errors.Is(err, ErrInconclusive) {
		t.Fatalf("expected ErrInconclusive, got %v", err)
	}
	if res.Verdict != VerdictInconclusive {
		t.Fatalf("expected inconclusive verdict, got %s", res.Verdict)
	}
}

func TestBaselineEvaluatorNoSideEffects(t *testing.T) {
	base := testTrajectory("success", "success", 100, 1000)
	e := BaselineEvaluator{}
	out, err := e.Evaluate(context.Background(), Case{Baseline: base, BaselineEvents: []trace.Event{{EventID: "evt", Timestamp: base.StartedAt, Component: "x", Operator: trace.OperatorExecute, EventKind: "ok"}}})
	if err != nil {
		t.Fatalf("unexpected evaluate error: %v", err)
	}
	if out.Trajectory.TrajectoryID != base.TrajectoryID {
		t.Fatalf("expected baseline trajectory")
	}
}
