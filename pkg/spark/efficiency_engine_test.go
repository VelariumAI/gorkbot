package spark

import (
	"os"
	"testing"

	"github.com/velariumai/gorkbot/pkg/sense"
)

func TestTIIRecordSuccess(t *testing.T) {
	dir, _ := os.MkdirTemp("", "tii-test-*")
	defer os.RemoveAll(dir)

	e := NewEfficiencyEngine(0.1, dir)
	for i := 0; i < 20; i++ {
		e.RecordSuccess("bash", 10)
	}
	entry := e.GetEntry("bash")
	if entry == nil {
		t.Fatal("entry is nil")
	}
	// After 20 successes (optimistic start=1.0), should stay near 1.0.
	if entry.SuccessRate < 0.95 {
		t.Errorf("expected sr near 1.0 after 20 successes, got %.4f", entry.SuccessRate)
	}
}

func TestTIIRecordFailure(t *testing.T) {
	dir, _ := os.MkdirTemp("", "tii-fail-*")
	defer os.RemoveAll(dir)

	e := NewEfficiencyEngine(0.1, dir)
	// Override the optimistic start by recording 50 failures.
	for i := 0; i < 50; i++ {
		e.RecordFailure("read_file", 20, "not found")
	}
	entry := e.GetEntry("read_file")
	if entry == nil {
		t.Fatal("entry is nil")
	}
	// EWMA converges toward 0 — should be quite low.
	if entry.SuccessRate > 0.1 {
		t.Errorf("expected sr near 0.0 after 50 failures, got %.4f", entry.SuccessRate)
	}
}

func TestTIIDeduplication(t *testing.T) {
	dir, _ := os.MkdirTemp("", "tii-dedup-*")
	defer os.RemoveAll(dir)

	e := NewEfficiencyEngine(0.1, dir)
	e.RecordSuccess("git_status", 5)
	e.RecordSuccess("git_status", 5)
	entry := e.GetEntry("git_status")
	if entry.Invocations != 2 {
		t.Errorf("expected 2 invocations, got %d", entry.Invocations)
	}
}

func TestTIIPersistReload(t *testing.T) {
	dir, _ := os.MkdirTemp("", "tii-persist-*")
	defer os.RemoveAll(dir)

	e := NewEfficiencyEngine(0.1, dir)
	e.RecordSuccess("write_file", 15)
	e.RecordFailure("write_file", 15, "permission denied")

	if err := e.Persist(); err != nil {
		t.Fatal("persist error:", err)
	}

	e2 := NewEfficiencyEngine(0.1, dir)
	entry := e2.GetEntry("write_file")
	if entry == nil {
		t.Fatal("entry not found after reload")
	}
	if entry.Invocations != 2 {
		t.Errorf("expected 2 invocations after reload, got %d", entry.Invocations)
	}
}

func TestIDLPushEvict(t *testing.T) {
	dir, _ := os.MkdirTemp("", "idl-evict-*")
	defer os.RemoveAll(dir)

	maxSize := 5
	l := NewImprovementDebtLedger(maxSize, dir)
	for i := 0; i < maxSize+1; i++ {
		l.Push(IDLEntry{
			ID:       idlTestID(i),
			ToolName: "tool",
			Category: sense.CatToolFailure,
			Severity: float64(i) / 10.0,
		})
	}
	if l.Len() != maxSize {
		t.Errorf("expected len=%d after overfill, got %d", maxSize, l.Len())
	}
}

func TestIDLDedup(t *testing.T) {
	dir, _ := os.MkdirTemp("", "idl-dedup-*")
	defer os.RemoveAll(dir)

	l := NewImprovementDebtLedger(50, dir)
	l.Push(IDLEntry{ID: "same:1", ToolName: "bash", Category: sense.CatToolFailure, Severity: 0.5, Occurrences: 1})
	l.Push(IDLEntry{ID: "same:1", ToolName: "bash", Category: sense.CatToolFailure, Severity: 0.5, Occurrences: 1})
	if l.Len() != 1 {
		t.Errorf("expected dedup to 1 entry, got %d", l.Len())
	}
	top := l.Top(1)
	if len(top) > 0 && top[0].Occurrences != 2 {
		t.Errorf("expected Occurrences=2 after dedup push, got %d", top[0].Occurrences)
	}
}

func TestIDLTop(t *testing.T) {
	dir, _ := os.MkdirTemp("", "idl-top-*")
	defer os.RemoveAll(dir)

	l := NewImprovementDebtLedger(50, dir)
	severities := []float64{0.1, 0.9, 0.5, 0.7, 0.3}
	for i, sev := range severities {
		l.Push(IDLEntry{ID: idlTestID(i), ToolName: "t", Category: sense.CatToolFailure, Severity: sev})
	}
	top := l.Top(3)
	if len(top) != 3 {
		t.Fatalf("expected 3 top items, got %d", len(top))
	}
	// Should be sorted descending.
	if top[0].Severity < top[1].Severity || top[1].Severity < top[2].Severity {
		t.Errorf("top items not sorted descending: %v %v %v", top[0].Severity, top[1].Severity, top[2].Severity)
	}
}

func TestIDLPersistReload(t *testing.T) {
	dir, _ := os.MkdirTemp("", "idl-persist-*")
	defer os.RemoveAll(dir)

	l := NewImprovementDebtLedger(50, dir)
	l.Push(IDLEntry{ID: "persist:1", ToolName: "bash", Category: sense.CatToolFailure, Severity: 0.8})
	if err := l.Persist(); err != nil {
		t.Fatal("persist error:", err)
	}
	l2 := NewImprovementDebtLedger(50, dir)
	if l2.Len() != 1 {
		t.Errorf("expected 1 entry after reload, got %d", l2.Len())
	}
}

// idlTestID generates a unique string ID for tests.
func idlTestID(i int) string {
	return "test:" + string(rune('a'+i))
}
