package webui

import (
	"testing"
	"time"
)

func TestRunStore_Create(t *testing.T) {
	store := NewRunStore()
	run := store.Create("run_1", "hello world", "Grok", "xAI")

	if run.ID != "run_1" {
		t.Errorf("Expected ID run_1, got %s", run.ID)
	}
	if run.Prompt != "hello world" {
		t.Errorf("Expected prompt 'hello world', got %s", run.Prompt)
	}
	if run.Status != "running" {
		t.Errorf("Expected status 'running', got %s", run.Status)
	}
	if run.Model != "Grok" {
		t.Errorf("Expected model Grok, got %s", run.Model)
	}
	if run.Provider != "xAI" {
		t.Errorf("Expected provider xAI, got %s", run.Provider)
	}
	if run.StartTime.IsZero() {
		t.Error("Expected non-zero StartTime")
	}
	if run.EndTime != nil {
		t.Error("Expected nil EndTime for running run")
	}
	if len(run.Tools) != 0 {
		t.Errorf("Expected 0 tools, got %d", len(run.Tools))
	}
}

func TestRunStore_Get_NotFound(t *testing.T) {
	store := NewRunStore()
	run, ok := store.Get("nonexistent")

	if ok {
		t.Error("Expected Get to return false for nonexistent run")
	}
	if run != nil {
		t.Error("Expected nil run")
	}
}

func TestRunStore_List_LimitRespected(t *testing.T) {
	store := NewRunStore()

	// Create 5 runs
	for i := 0; i < 5; i++ {
		store.Create("run_"+string(rune('0'+i)), "prompt", "model", "provider")
		time.Sleep(1 * time.Millisecond)
	}

	// Request 3 (most recent)
	recent := store.List(3)

	if len(recent) != 3 {
		t.Errorf("Expected 3 runs, got %d", len(recent))
	}

	// Should be in reverse order (most recent first)
	expected := []string{"run_4", "run_3", "run_2"}
	for i, exp := range expected {
		if recent[i].ID != exp {
			t.Errorf("Expected run %s at position %d, got %s", exp, i, recent[i].ID)
		}
	}
}

func TestRunStore_ToolLifecycle(t *testing.T) {
	store := NewRunStore()
	run := store.Create("run_1", "test", "model", "provider")

	// Tool start
	store.ToolStart("run_1", "bash")
	if len(run.Tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(run.Tools))
	}
	if run.Tools[0].Status != "running" {
		t.Errorf("Expected status 'running', got %s", run.Tools[0].Status)
	}

	time.Sleep(10 * time.Millisecond)

	// Tool done
	store.ToolDone("run_1", "bash")
	if run.Tools[0].Status != "complete" {
		t.Errorf("Expected status 'complete', got %s", run.Tools[0].Status)
	}
	if run.Tools[0].EndTime == nil {
		t.Error("Expected non-nil EndTime")
	}
}

func TestRunStore_Complete(t *testing.T) {
	store := NewRunStore()
	run := store.Create("run_1", "test", "model", "provider")

	time.Sleep(10 * time.Millisecond)

	store.Complete("run_1", 50)

	if run.Status != "complete" {
		t.Errorf("Expected status 'complete', got %s", run.Status)
	}
	if run.EndTime == nil {
		t.Error("Expected non-nil EndTime")
	}
	if run.TokensUsed != 50 {
		t.Errorf("Expected 50 tokens, got %d", run.TokensUsed)
	}
	if run.LatencyMS <= 0 {
		t.Errorf("Expected positive latency, got %d", run.LatencyMS)
	}
}

func TestRunStore_Fail(t *testing.T) {
	store := NewRunStore()
	run := store.Create("run_1", "test", "model", "provider")

	time.Sleep(10 * time.Millisecond)

	store.Fail("run_1", "connection timeout")

	if run.Status != "error" {
		t.Errorf("Expected status 'error', got %s", run.Status)
	}
	if run.ErrorMsg != "connection timeout" {
		t.Errorf("Expected error 'connection timeout', got %s", run.ErrorMsg)
	}
	if run.EndTime == nil {
		t.Error("Expected non-nil EndTime")
	}
}
