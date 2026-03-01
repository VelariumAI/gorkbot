package process

import (
	"testing"
	"time"
)

// TestManagerStartStop tests that we can start and stop a process
func TestManagerStartStop(t *testing.T) {
	m := NewManager()

	// Start a simple process
	proc, err := m.Start("test-1", "echo", []string{"hello"}, false)
	if err != nil {
		t.Fatalf("Failed to start process: %v", err)
	}

	if proc == nil {
		t.Fatal("Process should not be nil")
	}

	if proc.State != StateRunning {
		t.Errorf("Expected state running, got %s", proc.State)
	}

	// Wait a bit for process to complete
	time.Sleep(100 * time.Millisecond)

	// Stop the process
	err = m.Stop("test-1")
	if err != nil {
		t.Errorf("Failed to stop process: %v", err)
	}
}

// TestManagerGetProcess tests retrieving a process
func TestManagerGetProcess(t *testing.T) {
	m := NewManager()

	// Start a process
	_, err := m.Start("test-get", "echo", []string{"test"}, false)
	if err != nil {
		t.Fatalf("Failed to start process: %v", err)
	}

	// Get the process
	proc, ok := m.GetProcess("test-get")
	if !ok {
		t.Fatal("Process should exist")
	}

	if proc.ID != "test-get" {
		t.Errorf("Expected ID test-get, got %s", proc.ID)
	}
}

// TestManagerListProcesses tests listing all processes
func TestManagerListProcesses(t *testing.T) {
	m := NewManager()

	// Start multiple processes
	m.Start("test-1", "echo", []string{"1"}, false)
	m.Start("test-2", "echo", []string{"2"}, false)

	// List processes
	list := m.ListProcesses()
	if len(list) != 2 {
		t.Errorf("Expected 2 processes, got %d", len(list))
	}
}

// TestProcessOutputBuffering tests that process output is captured
func TestProcessOutputBuffering(t *testing.T) {
	m := NewManager()

	// Start a process with output
	proc, err := m.Start("test-output", "echo", []string{"hello world"}, false)
	if err != nil {
		t.Fatalf("Failed to start process: %v", err)
	}

	// Wait for process to complete
	time.Sleep(200 * time.Millisecond)

	// Check output is captured
	output := proc.GetOutput()
	if output == "" {
		t.Error("Expected output to be captured")
	}
}

// TestOnCompleteCallback tests the completion callback
func TestOnCompleteCallback(t *testing.T) {
	m := NewManager()

	var callbackCalled bool
	var exitCode int

	// Start a process with callback
	proc, err := m.Start("test-callback", "echo", []string{"test"}, false)
	if err != nil {
		t.Fatalf("Failed to start process: %v", err)
	}

	proc.OnComplete = func(code int, isError bool) {
		callbackCalled = true
		exitCode = code
	}

	// Wait for completion
	time.Sleep(200 * time.Millisecond)

	if !callbackCalled {
		t.Error("Callback should have been called")
	}

	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}
}
