package engine

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/tools"
)

type ctxProbeTool struct {
	tools.BaseTool
	run func(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error)
}

func newCtxProbeTool(name string, run func(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error)) *ctxProbeTool {
	return &ctxProbeTool{
		BaseTool: tools.NewBaseTool(name, "ctx probe", tools.CategoryMeta, false, tools.PermissionAlways),
		run:      run,
	}
}

func (t *ctxProbeTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}

func (t *ctxProbeTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
	return t.run(ctx, params)
}

func TestOrchestratorExecuteTool_PlanModeBlocksWriteTools(t *testing.T) {
	orch := &Orchestrator{
		ModeManager:         NewModeManager(),
		Registry:            tools.NewRegistry(nil),
		ConversationHistory: ai.NewConversationHistory(),
		Logger:              slog.Default(),
	}
	orch.ModeManager.Set(ModePlan)

	res, err := orch.ExecuteTool(context.Background(), tools.ToolRequest{
		ToolName:   "write_file",
		Parameters: map[string]interface{}{"path": "a.txt", "content": "x"},
	})
	if err != nil {
		t.Fatalf("expected nil error for plan-mode block, got %v", err)
	}
	if res == nil || res.Success {
		t.Fatalf("expected blocked tool result, got %#v", res)
	}
}

func TestOrchestratorExecuteTool_InjectsOrchestratorInContext(t *testing.T) {
	reg := tools.NewRegistry(nil)
	if err := reg.Register(newCtxProbeTool("ctx_probe", func(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
		v := ctx.Value(tools.OrchestratorContextKey())
		if v == nil {
			return &tools.ToolResult{Success: false, Error: "missing orchestrator context"}, nil
		}
		return &tools.ToolResult{Success: true, Output: "ok"}, nil
	})); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	orch := &Orchestrator{
		Registry:            reg,
		ConversationHistory: ai.NewConversationHistory(),
		Logger:              slog.Default(),
	}
	orch.ConversationHistory.AddAssistantMessage("latest reasoning")

	res, err := orch.ExecuteTool(context.Background(), tools.ToolRequest{
		ToolName:   "ctx_probe",
		Parameters: map[string]interface{}{},
		RequestID:  "req-1",
	})
	if err != nil {
		t.Fatalf("ExecuteTool returned error: %v", err)
	}
	if res == nil || !res.Success || res.Output != "ok" {
		t.Fatalf("unexpected result: %#v", res)
	}
}

func TestOrchestratorExecuteTool_PropagatesContextCancellation(t *testing.T) {
	reg := tools.NewRegistry(nil)
	if err := reg.Register(newCtxProbeTool("wait_ctx", func(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	orch := &Orchestrator{
		Registry:            reg,
		ConversationHistory: ai.NewConversationHistory(),
		Logger:              slog.Default(),
	}
	orch.ConversationHistory.AddAssistantMessage("latest reasoning")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, err := orch.ExecuteTool(ctx, tools.ToolRequest{
		ToolName:  "wait_ctx",
		RequestID: "req-2",
	})
	if res != nil {
		t.Fatalf("expected nil result on cancellation, got %#v", res)
	}
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled error, got %v", err)
	}
}
