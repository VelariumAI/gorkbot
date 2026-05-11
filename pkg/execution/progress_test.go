package execution

import "testing"

func TestHashParamsStable(t *testing.T) {
	p1 := map[string]interface{}{"a": float64(1), "b": "x"}
	p2 := map[string]interface{}{"b": "x", "a": float64(1)}
	if HashParams(p1) != HashParams(p2) {
		t.Fatal("expected stable hash for same params")
	}
}

func TestHashParamsDifferent(t *testing.T) {
	p1 := map[string]interface{}{"a": float64(1)}
	p2 := map[string]interface{}{"a": float64(2)}
	if HashParams(p1) == HashParams(p2) {
		t.Fatal("expected different hashes")
	}
}

func TestRecordAttemptRepeatsCounted(t *testing.T) {
	pt := NewProgressTracker()
	params := map[string]interface{}{"path": "x"}
	if got := pt.RecordAttempt("read_file", params, "s1"); got != 1 {
		t.Fatalf("expected count=1, got %d", got)
	}
	if got := pt.RecordAttempt("read_file", params, "s1"); got != 2 {
		t.Fatalf("expected count=2, got %d", got)
	}
}

func TestLoopDetectedAfterThreshold(t *testing.T) {
	pt := NewProgressTracker()
	params := map[string]interface{}{"path": "x"}
	pt.RecordAttempt("read_file", params, "s1")
	pt.RecordAttempt("read_file", params, "s1")
	pt.RecordAttempt("read_file", params, "s1")
	if !pt.IsLooping("read_file", params, "s1", 2) {
		t.Fatal("expected loop detection")
	}
}

func TestRecordSuccessResetsFailures(t *testing.T) {
	pt := NewProgressTracker()
	pt.RecordFailure()
	pt.RecordFailure()
	pt.RecordSuccess("new")
	if pt.ConsecutiveFailures() != 0 {
		t.Fatalf("expected reset failures, got %d", pt.ConsecutiveFailures())
	}
}
