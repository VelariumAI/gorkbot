package engine_test

import (
	"context"
	"testing"

	"github.com/velariumai/gorkbot/internal/engine"
)

func TestChainExecuteSimple(t *testing.T) {
	c := engine.NewChain()

	req := engine.ToolRequest{
		Name:   "test_tool",
		Params: map[string]interface{}{},
	}

	finalCalled := false
	final := func(ctx context.Context, r engine.ToolRequest) engine.ToolResult {
		finalCalled = true
		return engine.ToolResult{Output: "final"}
	}

	result := c.Execute(context.Background(), req, final)

	if !finalCalled {
		t.Fatal("expected final handler to be called")
	}
	if result.Output != "final" {
		t.Errorf("expected output='final', got %q", result.Output)
	}
}

func TestChainExecutionOrder(t *testing.T) {
	callOrder := []string{}

	// Create middleware that records call order
	m1 := func(ctx context.Context, req engine.ToolRequest, next func() engine.ToolResult) engine.ToolResult {
		callOrder = append(callOrder, "m1_before")
		result := next()
		callOrder = append(callOrder, "m1_after")
		return result
	}

	m2 := func(ctx context.Context, req engine.ToolRequest, next func() engine.ToolResult) engine.ToolResult {
		callOrder = append(callOrder, "m2_before")
		result := next()
		callOrder = append(callOrder, "m2_after")
		return result
	}

	c := engine.NewChain(m1, m2)

	req := engine.ToolRequest{Name: "test"}
	final := func(ctx context.Context, r engine.ToolRequest) engine.ToolResult {
		callOrder = append(callOrder, "final")
		return engine.ToolResult{Output: "result"}
	}

	_ = c.Execute(context.Background(), req, final)

	expectedOrder := []string{"m1_before", "m2_before", "final", "m2_after", "m1_after"}
	if len(callOrder) != len(expectedOrder) {
		t.Errorf("expected %d calls, got %d", len(expectedOrder), len(callOrder))
	}

	for i, expected := range expectedOrder {
		if i >= len(callOrder) {
			t.Fatalf("missing call at index %d", i)
		}
		if callOrder[i] != expected {
			t.Errorf("expected call[%d]=%q, got %q", i, expected, callOrder[i])
		}
	}
}

func TestMiddlewareCanBlock(t *testing.T) {
	blockMW := func(ctx context.Context, req engine.ToolRequest, next func() engine.ToolResult) engine.ToolResult {
		return engine.ToolResult{
			Blocked:  true,
			BlockMsg: "blocked by middleware",
		}
	}

	finalCalled := false
	final := func(ctx context.Context, r engine.ToolRequest) engine.ToolResult {
		finalCalled = true
		return engine.ToolResult{Output: "result"}
	}

	c := engine.NewChain(blockMW)
	req := engine.ToolRequest{Name: "test"}

	result := c.Execute(context.Background(), req, final)

	if !result.Blocked {
		t.Error("expected result to be blocked")
	}
	if finalCalled {
		t.Error("expected final handler NOT to be called when middleware blocks")
	}
}

func TestPlanModeMiddleware(t *testing.T) {
	mw := engine.PlanModeMiddleware(func() bool { return true })

	req := engine.ToolRequest{Name: "test"}
	result := mw(context.Background(), req, func() engine.ToolResult {
		return engine.ToolResult{Output: "should not reach"}
	})

	if !result.Blocked {
		t.Error("expected middleware to block in plan mode")
	}
	if result.Output != "" {
		t.Errorf("expected no output when blocked, got %q", result.Output)
	}
}

func TestPlanModeMiddlewareAlowsNonPlanning(t *testing.T) {
	mw := engine.PlanModeMiddleware(func() bool { return false })

	req := engine.ToolRequest{Name: "test"}
	result := mw(context.Background(), req, func() engine.ToolResult {
		return engine.ToolResult{Output: "allowed"}
	})

	if result.Blocked {
		t.Error("expected middleware to allow when not in plan mode")
	}
	if result.Output != "allowed" {
		t.Errorf("expected output='allowed', got %q", result.Output)
	}
}

func TestNilProvidersAreSafe(t *testing.T) {
	// Ensure all nil-safe middlewares don't panic
	c := engine.NewChain(
		engine.RuleEngineMiddleware(nil),
		engine.ToolCacheMiddleware(nil),
		engine.PreHookMiddleware(nil, "session"),
		engine.CheckpointMiddleware(nil, map[string]bool{}),
		engine.HITLMiddleware(nil),
		engine.SanitizerMiddleware(nil),
		engine.GuardrailsMiddleware(nil),
		engine.PostHookMiddleware(nil, "session"),
		engine.AgeMemMiddleware(nil),
		engine.TracingMiddleware(nil),
	)

	req := engine.ToolRequest{Name: "test"}
	final := func(ctx context.Context, r engine.ToolRequest) engine.ToolResult {
		return engine.ToolResult{Output: "ok"}
	}

	result := c.Execute(context.Background(), req, final)

	if result.Blocked {
		t.Error("expected execution to succeed with nil providers")
	}
	if result.Output != "ok" {
		t.Errorf("expected output='ok', got %q", result.Output)
	}
}

func TestUseMethodAddsMiddleware(t *testing.T) {
	c := engine.NewChain()

	callCount := 0
	mw := func(ctx context.Context, req engine.ToolRequest, next func() engine.ToolResult) engine.ToolResult {
		callCount++
		return next()
	}

	c.Use(mw)

	req := engine.ToolRequest{Name: "test"}
	final := func(ctx context.Context, r engine.ToolRequest) engine.ToolResult {
		return engine.ToolResult{Output: "ok"}
	}

	c.Execute(context.Background(), req, final)

	if callCount != 1 {
		t.Errorf("expected middleware to be called once, called %d times", callCount)
	}
}
