package engine_test

import (
	"strings"
	"testing"

	"github.com/velariumai/gorkbot/internal/engine"
	"github.com/velariumai/gorkbot/pkg/ai"
)

func TestExecutePlanMode_TokenAccrual_WithPanic(t *testing.T) {
	// Initialize Orchestrator and ContextManager
	orch := &engine.Orchestrator{}

	// Create a dummy ContextManager. onNearFull callback is nil.
	orch.ContextMgr = engine.NewContextManager(1000, nil)

	// Initialize ConversationHistory
	orch.ConversationHistory = ai.NewConversationHistory()

	// Setup planning buffer
	var planningBuf strings.Builder
	simulatedToolOutput := "intermediate thinking: the database is offline. searching for more details..."
	planningBuf.WriteString(simulatedToolOutput)

	expectedTokens := len(simulatedToolOutput) / 4

	// Verify starting state
	if orch.ContextMgr.InputTokens() != 0 {
		t.Fatalf("Expected 0 starting tokens, got %d", orch.ContextMgr.InputTokens())
	}

	// Simulated tool that panics mid-session
	panickingTool := func() error {
		panic("simulated asynchronous tool crash")
	}

	// Execute the plan mode
	err := engine.ExecutePlanMode(orch, &planningBuf, panickingTool)

	// 1. Verify Error State
	if err == nil {
		t.Fatal("Expected an error due to panic, got nil")
	}
	if !strings.Contains(err.Error(), "planning mode panicked") {
		t.Errorf("Expected panic error message, got: %v", err)
	}

	// 2. Verify State-Safe Execution (Buffer wiped)
	if planningBuf.Len() != 0 {
		t.Errorf("Expected planningBuf to be wiped, but length is %d", planningBuf.Len())
	}

	// 3. Verify Explicit Token Tracking
	actualTokens := orch.ContextMgr.InputTokens()
	if actualTokens != expectedTokens {
		t.Errorf("Expected %d tokens used, got %d", expectedTokens, actualTokens)
	}

	// 4. Verify Summarized History Commit & No Context Contamination
	msgs := orch.ConversationHistory.GetMessages()
	if len(msgs) != 1 {
		t.Fatalf("Expected exactly 1 summarized message in history, got %d", len(msgs))
	}

	sysMsg := msgs[0]
	if sysMsg.Role != "system" {
		t.Errorf("Expected message role 'system', got '%s'", sysMsg.Role)
	}

	if !strings.Contains(sysMsg.Content, "Planning phase completed") {
		t.Errorf("Message does not contain expected summary: %s", sysMsg.Content)
	}

	if strings.Contains(sysMsg.Content, "intermediate thinking") {
		t.Errorf("Context contamination detected: Raw buffer text leaked into history!")
	}
}
