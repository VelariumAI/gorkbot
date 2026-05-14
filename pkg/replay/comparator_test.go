package replay

import (
	"testing"

	"github.com/velariumai/gorkbot/pkg/trace"
)

func TestComparatorBaselineSuccessCandidateSuccess(t *testing.T) {
	base := testTrajectory("success", "success", 100, 1000)
	cand := testTrajectory("success", "success", 100, 1000)
	cand.TrajectoryID = "traj-candidate-1"

	c, err := NewCaseFromTrajectory("ok", base, CandidateSpec{ID: "c1", Kind: CandidateKindPolicy}, Expectations{})
	if err != nil {
		t.Fatalf("unexpected case error: %v", err)
	}

	cmp := DeterministicComparator{}.Compare(c, CandidateOutcome{Trajectory: cand})
	if cmp.Verdict != VerdictPass {
		t.Fatalf("expected pass, got %s", cmp.Verdict)
	}
}

func TestComparatorBaselineSuccessCandidateFailure(t *testing.T) {
	base := testTrajectory("success", "success", 100, 1000)
	cand := testTrajectory("failed", "failed", 100, 1000)
	cand.TrajectoryID = "traj-candidate-2"
	c, err := NewCaseFromTrajectory("fail", base, CandidateSpec{ID: "c2", Kind: CandidateKindPolicy}, Expectations{})
	if err != nil {
		t.Fatalf("unexpected case error: %v", err)
	}

	cmp := DeterministicComparator{}.Compare(c, CandidateOutcome{Trajectory: cand})
	if cmp.Verdict != VerdictRegression {
		t.Fatalf("expected regression, got %s", cmp.Verdict)
	}
}

func TestComparatorCostRegression(t *testing.T) {
	base := testTrajectory("success", "success", 100, 1000)
	cand := testTrajectory("success", "success", 100, 1300)
	cand.TrajectoryID = "traj-candidate-3"

	expect := Expectations{MaxCostIncreaseMicros: 100}
	c, err := NewCaseFromTrajectory("cost", base, CandidateSpec{ID: "c3", Kind: CandidateKindPolicy}, expect)
	if err != nil {
		t.Fatalf("unexpected case error: %v", err)
	}
	cmp := DeterministicComparator{}.Compare(c, CandidateOutcome{Trajectory: cand})
	if cmp.Verdict != VerdictRegression {
		t.Fatalf("expected cost regression verdict, got %s", cmp.Verdict)
	}
}

func TestComparatorDurationRegression(t *testing.T) {
	base := testTrajectory("success", "success", 100, 1000)
	cand := testTrajectory("success", "success", 250, 1000)
	cand.TrajectoryID = "traj-candidate-4"

	expect := Expectations{MaxDurationIncreaseMS: 50}
	c, err := NewCaseFromTrajectory("duration", base, CandidateSpec{ID: "c4", Kind: CandidateKindPolicy}, expect)
	if err != nil {
		t.Fatalf("unexpected case error: %v", err)
	}
	cmp := DeterministicComparator{}.Compare(c, CandidateOutcome{Trajectory: cand})
	if cmp.Verdict != VerdictRegression {
		t.Fatalf("expected duration regression verdict, got %s", cmp.Verdict)
	}
}

func TestComparatorImprovement(t *testing.T) {
	base := testTrajectory("success", "success", 200, 2000)
	cand := testTrajectory("success", "success", 100, 1000)
	cand.TrajectoryID = "traj-candidate-5"

	c, err := NewCaseFromTrajectory("improve", base, CandidateSpec{ID: "c5", Kind: CandidateKindPolicy}, Expectations{})
	if err != nil {
		t.Fatalf("unexpected case error: %v", err)
	}
	cmp := DeterministicComparator{}.Compare(c, CandidateOutcome{Trajectory: cand})
	if cmp.Verdict != VerdictImprovement {
		t.Fatalf("expected improvement verdict, got %s", cmp.Verdict)
	}
}

func TestComparatorForbiddenAndRequiredDetection(t *testing.T) {
	base := testTrajectory("success", "success", 100, 1000)
	cand := testTrajectory("success", "success", 100, 1000)
	cand.TrajectoryID = "traj-candidate-6"
	cand.OperatorPath = []trace.Operator{trace.OperatorPlan}

	expect := Expectations{
		RequiredOperators:   []trace.Operator{trace.OperatorExecute},
		ForbiddenOperators:  []trace.Operator{trace.OperatorPlan},
		RequiredEventKinds:  []string{"must_exist"},
		ForbiddenEventKinds: []string{"forbidden_kind"},
	}
	c, err := NewCaseFromTrajectory("forbid", base, CandidateSpec{ID: "c6", Kind: CandidateKindPolicy}, expect)
	if err != nil {
		t.Fatalf("unexpected case error: %v", err)
	}
	cmp := DeterministicComparator{}.Compare(c, CandidateOutcome{
		Trajectory: cand,
		Events: []trace.Event{
			{EventID: "evt-1", Timestamp: base.StartedAt, Component: "test", Operator: trace.OperatorPlan, EventKind: "forbidden_kind"},
		},
	})
	if cmp.Verdict != VerdictRegression {
		t.Fatalf("expected regression verdict, got %s", cmp.Verdict)
	}
}

func TestComparatorMalformedInputDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("comparator panicked: %v", r)
		}
	}()

	base := trace.Trajectory{}
	c := Case{
		ID:           "case-1",
		TrajectoryID: "",
		Baseline:     base,
		Candidate:    CandidateSpec{ID: "c7", Kind: CandidateKindUnknown},
	}
	_ = DeterministicComparator{}.Compare(c, CandidateOutcome{})
}
