package engine

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/hooks"
	"github.com/velariumai/gorkbot/pkg/tools"
)

func TestOrchestratorExecuteTool_RuleEngineDeny(t *testing.T) {
	reg := tools.NewRegistry(nil)
	if err := reg.Register(newCtxProbeTool("ctx_probe", func(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
		return &tools.ToolResult{Success: true, Output: "should-not-run"}, nil
	})); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	tmpDir := t.TempDir()
	re := tools.NewRuleEngine(tmpDir)
	if err := re.AddRule(tools.RuleDeny, "ctx_probe", "blocked by test"); err != nil {
		t.Fatalf("add rule: %v", err)
	}

	orch := &Orchestrator{
		Registry:            reg,
		RuleEngine:          re,
		ConversationHistory: ai.NewConversationHistory(),
		Logger:              slog.Default(),
	}
	orch.ConversationHistory.AddAssistantMessage("latest reasoning")

	res, err := orch.ExecuteTool(context.Background(), tools.ToolRequest{
		ToolName:  "ctx_probe",
		RequestID: "rule-1",
	})
	if err != nil {
		t.Fatalf("expected nil error on rule deny, got %v", err)
	}
	if res == nil || res.Success {
		t.Fatalf("expected denied result, got %#v", res)
	}
	if !strings.Contains(res.Error, "Blocked by rule") {
		t.Fatalf("unexpected deny reason: %q", res.Error)
	}
}

func TestOrchestratorExecuteTool_HookBlocksPreExecution(t *testing.T) {
	reg := tools.NewRegistry(nil)
	if err := reg.Register(newCtxProbeTool("ctx_probe", func(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
		return &tools.ToolResult{Success: true, Output: "should-not-run"}, nil
	})); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	tmpDir := t.TempDir()
	hm := hooks.NewManager(tmpDir, slog.Default())
	script := filepath.Join(tmpDir, "hooks", string(hooks.EventPreToolUse)+".sh")
	body := "#!/bin/sh\n" +
		"echo blocked-by-hook >&2\n" +
		"exit 2\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write hook script: %v", err)
	}

	orch := &Orchestrator{
		Registry:            reg,
		Hooks:               hm,
		ConversationHistory: ai.NewConversationHistory(),
		Logger:              slog.Default(),
	}
	orch.ConversationHistory.AddAssistantMessage("latest reasoning")

	res, err := orch.ExecuteTool(context.Background(), tools.ToolRequest{
		ToolName:  "ctx_probe",
		RequestID: "hook-1",
	})
	if err != nil {
		t.Fatalf("expected nil error on hook deny, got %v", err)
	}
	if res == nil || res.Success {
		t.Fatalf("expected blocked result, got %#v", res)
	}
	if !strings.Contains(res.Error, "Blocked by pre_tool_use hook") {
		t.Fatalf("unexpected block reason: %q", res.Error)
	}
}

func TestOrchestratorExecuteTool_HITLBlocksHighStakes(t *testing.T) {
	orch := &Orchestrator{
		Registry:            tools.NewRegistry(nil),
		HITLGuard:           NewHITLGuard(),
		ConversationHistory: ai.NewConversationHistory(),
		Logger:              slog.Default(),
	}
	orch.HITLCallback = func(req HITLRequest) HITLDecision {
		return HITLDecision{Approval: HITLRejected, Notes: "denied for test"}
	}
	orch.ConversationHistory.AddAssistantMessage("latest reasoning")

	res, err := orch.ExecuteTool(context.Background(), tools.ToolRequest{
		ToolName:   "bash",
		Parameters: map[string]interface{}{"command": "rm -rf /tmp/test"},
		RequestID:  "hitl-1",
	})
	if err != nil {
		t.Fatalf("expected nil error on HITL deny, got %v", err)
	}
	if res == nil || res.Success {
		t.Fatalf("expected blocked result, got %#v", res)
	}
	if !strings.Contains(strings.ToLower(res.Error), "blocked by hitl") {
		t.Fatalf("unexpected HITL block reason: %q", res.Error)
	}
}

func TestOrchestratorSISnapshotAndTriggerWithoutDriver(t *testing.T) {
	orch := &Orchestrator{}
	snap := orch.SISnapshot()
	if snap.Enabled {
		t.Fatalf("unexpected snapshot: %#v", snap)
	}
	after := orch.TriggerSICycle(context.Background())
	if after.Enabled {
		t.Fatalf("unexpected trigger snapshot: %#v", after)
	}
}
