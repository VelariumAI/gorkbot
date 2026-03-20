package spark

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestNewSPARKNilSubsystems(t *testing.T) {
	dir, _ := os.MkdirTemp("", "spark-daemon-test-*")
	defer os.RemoveAll(dir)

	cfg := DefaultConfig(dir)
	// All optional args nil — must not panic.
	s := New(cfg, nil, nil, nil, nil, nil)
	if s == nil {
		t.Fatal("New returned nil")
	}
}

func TestTriggerCycleNonBlocking(t *testing.T) {
	dir, _ := os.MkdirTemp("", "spark-trigger-*")
	defer os.RemoveAll(dir)

	cfg := DefaultConfig(dir)
	s := New(cfg, nil, nil, nil, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)

	// Two rapid triggers — must not block.
	done := make(chan struct{})
	go func() {
		s.TriggerCycle()
		s.TriggerCycle()
		close(done)
	}()
	select {
	case <-done:
		// pass
	case <-time.After(500 * time.Millisecond):
		t.Fatal("TriggerCycle blocked")
	}
}

func TestAppendToolEventUpdatesIDL(t *testing.T) {
	dir, _ := os.MkdirTemp("", "spark-idl-update-*")
	defer os.RemoveAll(dir)

	cfg := DefaultConfig(dir)
	s := New(cfg, nil, nil, nil, nil, nil)

	// Record 5 failures for a tool to push it into IDL.
	for i := 0; i < 5; i++ {
		s.AppendToolEvent("bad_tool", false, 50, "connection refused")
	}
	// IDL should have at least one entry now.
	if s.idl.Len() == 0 {
		t.Error("expected IDL to have entries after 5 failures")
	}
}

func TestPrepareContextEmptyBeforeCycle(t *testing.T) {
	dir, _ := os.MkdirTemp("", "spark-ctx-*")
	defer os.RemoveAll(dir)

	cfg := DefaultConfig(dir)
	s := New(cfg, nil, nil, nil, nil, nil)

	// Before first cycle, lastState is nil → PrepareContext returns "".
	ctx := s.PrepareContext()
	if ctx != "" {
		t.Errorf("expected empty context before first cycle, got %q", ctx)
	}
}

func TestGetStatus(t *testing.T) {
	dir, _ := os.MkdirTemp("", "spark-status-*")
	defer os.RemoveAll(dir)

	cfg := DefaultConfig(dir)
	s := New(cfg, nil, nil, nil, nil, nil)
	status := s.GetStatus()
	if status == "" {
		t.Error("GetStatus returned empty string")
	}
}
