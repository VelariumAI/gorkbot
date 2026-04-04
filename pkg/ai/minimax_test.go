package ai

import (
	"testing"
)

// TestMiniMaxWithModelPreservesThinkingBudget verifies that switching models
// preserves the ThinkingBudget field.
func TestMiniMaxWithModelPreservesThinkingBudget(t *testing.T) {
	mm := NewMiniMaxProvider("test-key", "MiniMax-M2.5")
	mm.inner.ThinkingBudget = 10000

	// Switch model
	newProvider := mm.WithModel("MiniMax-M1")
	newMM, ok := newProvider.(*MiniMaxProvider)
	if !ok {
		t.Fatalf("WithModel returned wrong type: %T", newProvider)
	}

	// Verify ThinkingBudget was preserved
	if newMM.inner.ThinkingBudget != 10000 {
		t.Errorf("ThinkingBudget not preserved: got %d, want 10000", newMM.inner.ThinkingBudget)
	}

	// Verify model was actually switched
	if newMM.inner.Model != "MiniMax-M1" {
		t.Errorf("Model not switched: got %s, want MiniMax-M1", newMM.inner.Model)
	}
}

// TestMiniMaxThinkingBudgetProvider verifies that MiniMax implements
// the ThinkingBudgetProvider interface.
func TestMiniMaxThinkingBudgetProvider(t *testing.T) {
	mm := NewMiniMaxProvider("test-key", "MiniMax-M2.5")

	// Verify interface implementation
	var _ ThinkingBudgetProvider = mm

	// Test SetThinkingBudget
	mm.SetThinkingBudget(5000)
	if mm.GetThinkingBudget() != 5000 {
		t.Errorf("GetThinkingBudget failed: got %d, want 5000", mm.GetThinkingBudget())
	}

	// Test zero budget
	mm.SetThinkingBudget(0)
	if mm.GetThinkingBudget() != 0 {
		t.Errorf("GetThinkingBudget failed: got %d, want 0", mm.GetThinkingBudget())
	}
}

// TestAnthropicThinkingBudgetProvider verifies that Anthropic implements
// the ThinkingBudgetProvider interface.
func TestAnthropicThinkingBudgetProvider(t *testing.T) {
	ap := NewAnthropicProvider("test-key", "claude-opus-4")

	// Verify interface implementation
	var _ ThinkingBudgetProvider = ap

	// Test SetThinkingBudget
	ap.SetThinkingBudget(8000)
	if ap.GetThinkingBudget() != 8000 {
		t.Errorf("GetThinkingBudget failed: got %d, want 8000", ap.GetThinkingBudget())
	}

	// Test via WithModel (should preserve budget)
	newAP := ap.WithModel("claude-sonnet-4-5").(*AnthropicProvider)
	if newAP.ThinkingBudget != 8000 {
		t.Errorf("WithModel did not preserve ThinkingBudget: got %d, want 8000", newAP.ThinkingBudget)
	}
}

// TestAnthropicWithModelPreservesThinkingBudget verifies that Anthropic's
// WithModel preserves all configuration fields.
func TestAnthropicWithModelPreservesThinkingBudget(t *testing.T) {
	ap := NewAnthropicProvider("test-key", "claude-opus-4")
	ap.ThinkingBudget = 15000

	newAP := ap.WithModel("claude-sonnet-4-5")
	newAPPtr, ok := newAP.(*AnthropicProvider)
	if !ok {
		t.Fatalf("WithModel returned wrong type: %T", newAP)
	}

	if newAPPtr.ThinkingBudget != 15000 {
		t.Errorf("ThinkingBudget not preserved: got %d, want 15000", newAPPtr.ThinkingBudget)
	}

	if newAPPtr.APIKey != ap.APIKey {
		t.Errorf("APIKey not preserved")
	}

	if newAPPtr.BaseURL != ap.BaseURL {
		t.Errorf("BaseURL not preserved")
	}

	if newAPPtr.Model != "claude-sonnet-4-5" {
		t.Errorf("Model not switched: got %s, want claude-sonnet-4-5", newAPPtr.Model)
	}
}
