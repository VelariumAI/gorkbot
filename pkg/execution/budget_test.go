package execution

import (
	"context"
	"testing"
	"time"
)

func TestDefaultBudgetNonZero(t *testing.T) {
	b := DefaultBudget()
	if b.TurnTimeout <= 0 || b.ToolDecisionTimeout <= 0 || b.ToolExecutionTimeout <= 0 {
		t.Fatalf("expected nonzero defaults, got %#v", b)
	}
	if b.MaxToolCalls <= 0 || b.MaxRepeatedToolCalls <= 0 {
		t.Fatalf("expected positive counters, got %#v", b)
	}
}

func TestBudgetForToolLookup(t *testing.T) {
	b := DefaultBudget()
	tb := b.BudgetForTool("read_file")
	if tb.DecisionTimeout != 50*time.Millisecond {
		t.Fatalf("unexpected read_file decision timeout: %v", tb.DecisionTimeout)
	}
	if tb.ExecutionTimeout != 1*time.Second {
		t.Fatalf("unexpected read_file execution timeout: %v", tb.ExecutionTimeout)
	}
}

func TestBudgetForUnknownToolFallsBack(t *testing.T) {
	b := DefaultBudget()
	tb := b.BudgetForTool("totally_unknown")
	if tb.DecisionTimeout != b.ToolDecisionTimeout {
		t.Fatalf("expected fallback decision timeout %v, got %v", b.ToolDecisionTimeout, tb.DecisionTimeout)
	}
	if tb.ExecutionTimeout != b.ToolExecutionTimeout {
		t.Fatalf("expected fallback execution timeout %v, got %v", b.ToolExecutionTimeout, tb.ExecutionTimeout)
	}
}

func TestTimeoutContextExpires(t *testing.T) {
	b := DefaultBudget()
	b.ToolDecisionTimeout = 20 * time.Millisecond

	ctx, cancel := b.WithToolDecisionTimeout(context.Background(), "unknown")
	defer cancel()

	select {
	case <-ctx.Done():
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected decision timeout to expire")
	}
}
