package trace

import (
	"testing"
	"time"
)

func TestFinalizeTrajectory(t *testing.T) {
	start := time.Now().UTC().Add(-2 * time.Second)
	tr := NewTrajectory("session-1", "ship feature", "audit")
	tr.StartedAt = start
	tr.OperatorPath = AppendOperatorPath(tr.OperatorPath, OperatorPlan)
	tr.OperatorPath = AppendOperatorPath(tr.OperatorPath, OperatorExecute)
	tr.EventRefs = []string{"evt1", "evt2"}

	out := FinalizeTrajectory(tr, StableHash("end"), "success", "done", time.Now().UTC(), CostSummary{})
	if out.Duration <= 0 {
		t.Fatalf("expected positive duration, got %d", out.Duration)
	}
	if out.EndStateHash == "" {
		t.Fatalf("expected end state hash")
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("expected valid trajectory: %v", err)
	}
}

func TestAppendOperatorPathBounded(t *testing.T) {
	path := make([]Operator, 0, maxOperatorPath+5)
	for i := 0; i < maxOperatorPath+10; i++ {
		path = AppendOperatorPath(path, OperatorExecute)
	}
	if len(path) != maxOperatorPath {
		t.Fatalf("expected bounded path length %d, got %d", maxOperatorPath, len(path))
	}
}
