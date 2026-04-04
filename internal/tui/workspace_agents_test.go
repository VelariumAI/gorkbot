package tui

import (
	"testing"
)

// TestRenderAgentsViewNilOrchestrator ensures renderAgentsView doesn't panic with nil orchestrator.
func TestRenderAgentsViewNilOrchestrator(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("renderAgentsView panicked: %v", r)
		}
	}()

	m := &Model{
		state:        agentsView,
		orchestrator: nil, // Deliberately nil
	}

	result := m.renderAgentsView()
	if result == "" {
		t.Error("Expected renderAgentsView to return non-empty string")
	}
	// Verify the header is present
	if !contains(result, "AGENTS") {
		t.Error("Expected renderAgentsView to include 'AGENTS' header")
	}
}

// TestRenderMemoryViewNilOrchestrator ensures renderMemoryView doesn't panic with nil orchestrator.
func TestRenderMemoryViewNilOrchestrator(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("renderMemoryView panicked: %v", r)
		}
	}()

	m := &Model{
		state:        memoryView,
		orchestrator: nil, // Deliberately nil
	}

	result := m.renderMemoryView()
	if result == "" {
		t.Error("Expected renderMemoryView to return non-empty string")
	}
	// Verify the header is present
	if !contains(result, "MEMORY") {
		t.Error("Expected renderMemoryView to include 'MEMORY' header")
	}
}

// TestRenderAgentsViewNilTokenStyles ensures renderAgentsView handles nil TokenStyles gracefully.
func TestRenderAgentsViewNilTokenStyles(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("renderAgentsView panicked with nil TokenStyles: %v", r)
		}
	}()

	m := &Model{
		state:        agentsView,
		orchestrator: nil,
		styles: &Styles{
			TokenStyles: nil, // Deliberately nil
		},
	}

	_ = m.renderAgentsView()
	// If we get here without panic, test passes
}

// contains is a helper function to check if a string contains a substring.
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
