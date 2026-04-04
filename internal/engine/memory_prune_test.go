package engine

import (
	"strings"
	"testing"
)

func TestPruneMemoryParts_DeduplicatesIdentical(t *testing.T) {
	parts := []string{"same string", "same string"}
	result := pruneMemoryParts(parts, 1000)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0] != "same string" {
		t.Errorf("expected 'same string', got '%s'", result[0])
	}
}

func TestPruneMemoryParts_BudgetCap(t *testing.T) {
	// Generate a 200-token string (800 chars)
	longStr := strings.Repeat("A", 800)
	var parts []string
	for i := 0; i < 10; i++ {
		// Make each slightly different so they don't dedup
		parts = append(parts, longStr + string(rune('a'+i)))
	}
	
	// Budget of 500 tokens
	result := pruneMemoryParts(parts, 500)
	
	// Each part is ~200 tokens. 500 budget -> max 2 parts.
	if len(result) > 3 {
		t.Errorf("expected <= 3 parts, got %d", len(result))
	}
}

func TestPruneMemoryParts_EmptyInput(t *testing.T) {
	parts := []string{"", "   ", "\n\t"}
	result := pruneMemoryParts(parts, 1000)
	if len(result) != 0 {
		t.Errorf("expected empty result, got len %d", len(result))
	}
}

func TestPruneMemoryParts_PreservesOrder(t *testing.T) {
	parts := []string{"first", "second", "third"}
	result := pruneMemoryParts(parts, 1000)
	if len(result) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(result))
	}
	if result[0] != "first" || result[1] != "second" || result[2] != "third" {
		t.Errorf("order not preserved: %v", result)
	}
}
