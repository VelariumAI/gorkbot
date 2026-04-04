package engine

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/velariumai/gorkbot/pkg/tools"
)

type fakeTool struct {
	name string
	run  func(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error)
}

func (f *fakeTool) Name() string                             { return f.name }
func (f *fakeTool) Description() string                      { return "fake tool" }
func (f *fakeTool) Category() tools.ToolCategory             { return tools.CategoryMeta }
func (f *fakeTool) Parameters() json.RawMessage              { return json.RawMessage(`{"type":"object"}`) }
func (f *fakeTool) RequiresPermission() bool                 { return false }
func (f *fakeTool) DefaultPermission() tools.PermissionLevel { return tools.PermissionAlways }
func (f *fakeTool) OutputFormat() tools.OutputFormat         { return tools.FormatText }
func (f *fakeTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
	if f.run == nil {
		return &tools.ToolResult{Success: true, Output: "ok"}, nil
	}
	return f.run(ctx, params)
}

func TestToolRegistryAdapter_ErrorPaths(t *testing.T) {
	adapter := &toolRegistryAdapter{}
	if _, err := adapter.ExecuteTool(context.Background(), "x", nil); err == nil {
		t.Fatal("expected registry not available error")
	}

	reg := tools.NewRegistry(nil)
	adapter.reg = reg
	if _, err := adapter.ExecuteTool(context.Background(), "missing", nil); err == nil {
		t.Fatal("expected tool not found error")
	}
}

func TestToolRegistryAdapter_ExecuteErrorsAndSuccess(t *testing.T) {
	reg := tools.NewRegistry(nil)
	adapter := &toolRegistryAdapter{reg: reg}

	_ = reg.Register(&fakeTool{
		name: "tool_exec_err",
		run: func(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
			return nil, errors.New("boom")
		},
	})
	if _, err := adapter.ExecuteTool(context.Background(), "tool_exec_err", nil); err == nil {
		t.Fatal("expected execution error")
	}

	_ = reg.Register(&fakeTool{
		name: "tool_nil_result",
		run: func(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
			return nil, nil
		},
	})
	if _, err := adapter.ExecuteTool(context.Background(), "tool_nil_result", nil); err == nil {
		t.Fatal("expected nil result error")
	}

	_ = reg.Register(&fakeTool{
		name: "tool_failed_result",
		run: func(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
			return &tools.ToolResult{Success: false, Error: "failed"}, nil
		},
	})
	out, err := adapter.ExecuteTool(context.Background(), "tool_failed_result", nil)
	if err == nil {
		t.Fatal("expected failed result error")
	}
	if out != "failed" {
		t.Fatalf("expected returned error output, got %q", out)
	}

	_ = reg.Register(&fakeTool{
		name: "tool_ok",
		run: func(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
			return &tools.ToolResult{Success: true, Output: "done"}, nil
		},
	})
	out, err = adapter.ExecuteTool(context.Background(), "tool_ok", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "done" {
		t.Fatalf("expected done, got %q", out)
	}
}

func TestToolRegistryAdapter_PropagatesContextCancel(t *testing.T) {
	reg := tools.NewRegistry(nil)
	adapter := &toolRegistryAdapter{reg: reg}

	_ = reg.Register(&fakeTool{
		name: "tool_wait_ctx",
		run: func(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := adapter.ExecuteTool(ctx, "tool_wait_ctx", nil)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}

	// Sanity with timeout context as well.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel2()
	_, err = adapter.ExecuteTool(ctx2, "tool_wait_ctx", nil)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}
