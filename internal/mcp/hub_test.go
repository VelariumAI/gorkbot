package mcp

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

type fakeExecutor struct {
	executeFn func(ctx context.Context, server *MCPServer, tool *MCPTool, input map[string]interface{}) (interface{}, error)
	healthFn  func(ctx context.Context, server *MCPServer) error
}

func (f *fakeExecutor) ExecuteTool(ctx context.Context, server *MCPServer, tool *MCPTool, input map[string]interface{}) (interface{}, error) {
	if f.executeFn == nil {
		return map[string]interface{}{"ok": true}, nil
	}
	return f.executeFn(ctx, server, tool, input)
}

func (f *fakeExecutor) HealthCheck(ctx context.Context, server *MCPServer) error {
	if f.healthFn == nil {
		return nil
	}
	return f.healthFn(ctx, server)
}

func TestMCPHubExecuteToolSuccess(t *testing.T) {
	hub := NewMCPHub(slog.Default())
	hub.SetExecutor(&fakeExecutor{
		executeFn: func(ctx context.Context, server *MCPServer, tool *MCPTool, input map[string]interface{}) (interface{}, error) {
			if server.Name != "srv1" || tool.Name != "echo" || input["value"] != "ok" {
				t.Fatalf("unexpected execution input")
			}
			return map[string]interface{}{"result": "done"}, nil
		},
	})
	if err := hub.RegisterServer(&MCPServer{Name: "srv1", Enabled: true, Status: "stopped"}); err != nil {
		t.Fatalf("register server: %v", err)
	}
	hub.RegisterTool(&MCPTool{Name: "echo", ServerName: "srv1"})

	out, err := hub.ExecuteToolContext(context.Background(), "echo", map[string]interface{}{"value": "ok"})
	if err != nil {
		t.Fatalf("ExecuteToolContext failed: %v", err)
	}
	got, ok := out.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T", out)
	}
	if got["result"] != "done" {
		t.Fatalf("unexpected result: %#v", got)
	}

	s := hub.GetServer("srv1")
	if s == nil || s.Status != "running" {
		t.Fatalf("expected server running, got %#v", s)
	}
	if s.LastPing.IsZero() {
		t.Fatalf("expected LastPing to be updated")
	}
}

func TestMCPHubExecuteToolTimeout(t *testing.T) {
	hub := NewMCPHub(slog.Default())
	hub.executionTimeout = 10 * time.Millisecond
	hub.SetExecutor(&fakeExecutor{
		executeFn: func(ctx context.Context, server *MCPServer, tool *MCPTool, input map[string]interface{}) (interface{}, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	})
	if err := hub.RegisterServer(&MCPServer{Name: "srv1", Enabled: true, Status: "stopped"}); err != nil {
		t.Fatalf("register server: %v", err)
	}
	hub.RegisterTool(&MCPTool{Name: "slow", ServerName: "srv1"})

	_, err := hub.ExecuteToolContext(context.Background(), "slow", map[string]interface{}{})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	s := hub.GetServer("srv1")
	if s == nil || s.Status != "error" {
		t.Fatalf("expected server status error, got %#v", s)
	}
}

func TestMCPHubExecuteToolDisabledServer(t *testing.T) {
	hub := NewMCPHub(slog.Default())
	if err := hub.RegisterServer(&MCPServer{Name: "srv1", Enabled: false}); err != nil {
		t.Fatalf("register server: %v", err)
	}
	hub.RegisterTool(&MCPTool{Name: "echo", ServerName: "srv1"})

	_, err := hub.ExecuteTool("echo", map[string]interface{}{})
	if err == nil {
		t.Fatal("expected disabled server error")
	}
}

func TestMCPHubHealthCheckStatusUpdates(t *testing.T) {
	hub := NewMCPHub(slog.Default())
	hub.SetExecutor(&fakeExecutor{
		healthFn: func(ctx context.Context, server *MCPServer) error {
			if server.Name == "bad" {
				return errors.New("probe failed")
			}
			return nil
		},
	})

	if err := hub.RegisterServer(&MCPServer{Name: "ok", Enabled: true, Status: "stopped"}); err != nil {
		t.Fatalf("register ok server: %v", err)
	}
	if err := hub.RegisterServer(&MCPServer{Name: "bad", Enabled: true, Status: "stopped"}); err != nil {
		t.Fatalf("register bad server: %v", err)
	}

	if err := hub.HealthCheck("ok"); err != nil {
		t.Fatalf("health check ok failed: %v", err)
	}
	if err := hub.HealthCheck("bad"); err == nil {
		t.Fatal("expected bad server health check error")
	}

	if s := hub.GetServer("ok"); s == nil || s.Status != "running" {
		t.Fatalf("expected ok server running, got %#v", s)
	}
	if s := hub.GetServer("bad"); s == nil || s.Status != "error" {
		t.Fatalf("expected bad server error, got %#v", s)
	}
}
