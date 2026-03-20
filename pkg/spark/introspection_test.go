package spark

import (
	"context"
	"os"
	"testing"
)

// mockAgeMem implements ageMemReader for testing.
type mockAgeMem struct {
	shouldPrune bool
	usageStats  map[string]interface{}
}

func (m *mockAgeMem) ShouldPrune() bool                  { return m.shouldPrune }
func (m *mockAgeMem) UsageStats() map[string]interface{} { return m.usageStats }

func TestSnapshotNilAgeMem(t *testing.T) {
	dir, _ := os.MkdirTemp("", "intro-test-*")
	defer os.RemoveAll(dir)

	tii := NewEfficiencyEngine(0.1, dir)
	idl := NewImprovementDebtLedger(50, dir)
	intro := NewIntrospector(tii, idl, nil, nil, nil)

	state := intro.Snapshot(context.Background())
	if state.MemoryPressure != 0.0 {
		t.Errorf("expected MemoryPressure=0 with nil ageMem, got %f", state.MemoryPressure)
	}
	if state.SubsystemHealth == nil {
		t.Error("SubsystemHealth should not be nil")
	}
}

func TestCheckHealthTIIWarn(t *testing.T) {
	dir, _ := os.MkdirTemp("", "health-tii-*")
	defer os.RemoveAll(dir)

	tii := NewEfficiencyEngine(0.1, dir)
	// Record 11 failures to trigger warn (>10 calls, <50% success rate).
	for i := 0; i < 11; i++ {
		tii.RecordFailure("bad_tool", 10, "fail")
	}
	idl := NewImprovementDebtLedger(50, dir)
	intro := NewIntrospector(tii, idl, nil, nil, nil)
	state := intro.Snapshot(context.Background())

	if state.SubsystemHealth["tii"] != "warn" {
		t.Errorf("expected tii health=warn, got %q", state.SubsystemHealth["tii"])
	}
}

func TestCheckHealthIDLError(t *testing.T) {
	dir, _ := os.MkdirTemp("", "health-idl-*")
	defer os.RemoveAll(dir)

	tii := NewEfficiencyEngine(0.1, dir)
	maxSize := 5
	idl := NewImprovementDebtLedger(maxSize, dir)
	for i := 0; i < maxSize; i++ {
		idl.Push(IDLEntry{ID: idlTestID(i), ToolName: "t", Severity: 0.5})
	}
	intro := NewIntrospector(tii, idl, nil, nil, nil)
	state := intro.Snapshot(context.Background())

	if state.SubsystemHealth["idl"] != "error" {
		t.Errorf("expected idl health=error when full, got %q", state.SubsystemHealth["idl"])
	}
}
