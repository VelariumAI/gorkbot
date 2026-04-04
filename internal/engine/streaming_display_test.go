package engine

import (
	"testing"
	"time"
)

func TestToolExecutionDisplay_BasicFlow(t *testing.T) {
	ted := NewToolExecutionDisplay()

	ted.BeginToolExecution("bash")
	if ted.ToolName != "bash" {
		t.Errorf("expected bash, got %s", ted.ToolName)
	}
	if ted.Status != "pending" {
		t.Errorf("expected pending, got %s", ted.Status)
	}
}

func TestToolExecutionDisplay_Execution(t *testing.T) {
	ted := NewToolExecutionDisplay()
	ted.BeginToolExecution("read_file")

	time.Sleep(10 * time.Millisecond)
	ted.UpdateToolOutput("file contents here", 50)
	if ted.Status != "running" {
		t.Error("expected running status")
	}

	ted.CompleteToolExecution(true, "file contents here", 100)
	if !ted.Success {
		t.Error("expected success")
	}
	if ted.TokensUsed != 100 {
		t.Errorf("expected 100 tokens, got %d", ted.TokensUsed)
	}
}

func TestToolExecutionDisplay_Failure(t *testing.T) {
	ted := NewToolExecutionDisplay()
	ted.BeginToolExecution("curl")

	ted.FailToolExecution("connection timeout")
	if ted.Status != "failed" {
		t.Errorf("expected failed, got %s", ted.Status)
	}
	if ted.ErrorMsg != "connection timeout" {
		t.Errorf("expected timeout error, got %s", ted.ErrorMsg)
	}
}

func TestToolExecutionDisplay_RenderDisplay(t *testing.T) {
	ted := NewToolExecutionDisplay()
	ted.BeginToolExecution("test")

	display := ted.RenderDisplay()
	if !contains(display, "test") {
		t.Errorf("display missing tool name: %s", display)
	}
}

