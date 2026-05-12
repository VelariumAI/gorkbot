package tools

import (
	"context"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/velariumai/gorkbot/pkg/governance"
)

func TestCreateToolRejectsTraversalName(t *testing.T) {
	dir := t.TempDir()
	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	tool := NewCreateToolTool()
	ctx := context.WithValue(context.Background(), governanceDecisionContextKey, governance.GovernanceDecision{Mode: governance.GOVERNANCE_ENFORCE})
	manifest := map[string]any{
		"name":             "ok",
		"artifact_kind":    "dynamic_tool",
		"risk_class":       "moderate",
		"capabilities":     []any{"dynamic.skill.stage"},
		"target_paths":     []any{".gorkbot/staging/tools/ok.go"},
		"expected_effects": []any{"stage only"},
		"rollback_plan":    "delete staged file",
	}
	params := map[string]interface{}{
		"name":        "../../../pkg/governance/policy",
		"description": "evil",
		"command":     "echo hi",
		"manifest":    manifest,
	}
	res, err := tool.Execute(ctx, params)
	if err == nil && res.Success {
		t.Fatalf("expected traversal name to be rejected, got success: %+v", res)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "pkg", "governance", "policy.go")); statErr == nil {
		t.Fatalf("policy.go was written outside staging — bypass not closed")
	}
	if res != nil && !strings.Contains(res.Error, "invalid tool name") && !strings.Contains(res.Error, "stage path rejected") {
		t.Fatalf("unexpected error message: %q", res.Error)
	}
}

func TestDefineCommandRejectsTraversalName(t *testing.T) {
	dir := t.TempDir()
	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	tool := NewDefineCommandTool()
	ctx := context.WithValue(context.Background(), governanceDecisionContextKey, governance.GovernanceDecision{Mode: governance.GOVERNANCE_ENFORCE})
	params := map[string]interface{}{
		"name":        "../../../pkg/governance/owned",
		"description": "evil",
		"prompt":      "do bad",
		"manifest": map[string]any{
			"name":             "ok",
			"artifact_kind":    "dynamic_tool",
			"risk_class":       "moderate",
			"capabilities":     []any{"dynamic.skill.stage"},
			"target_paths":     []any{".gorkbot/staging/commands/ok.json"},
			"expected_effects": []any{"stage only"},
			"rollback_plan":    "delete staged file",
		},
	}
	res, _ := tool.Execute(ctx, params)
	if res != nil && res.Success {
		t.Fatalf("expected traversal command name to be rejected")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "pkg", "governance", "owned.json")); statErr == nil {
		t.Fatalf("staged file escaped staging — bypass not closed")
	}
}

func TestModifyToolRejectsTraversalName(t *testing.T) {
	dir := t.TempDir()
	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	tool := NewModifyToolTool()
	ctx := context.WithValue(context.Background(), governanceDecisionContextKey, governance.GovernanceDecision{Mode: governance.GOVERNANCE_ENFORCE})
	res, err := tool.Execute(ctx, map[string]interface{}{
		"name":    "../../../pkg/governance/policy",
		"command": "echo hi",
	})
	if err == nil && res != nil && res.Success {
		t.Fatalf("expected traversal name to be rejected")
	}
	if res == nil || !strings.Contains(res.Error, "invalid tool name") {
		t.Fatalf("expected invalid tool name error, got res=%+v err=%v", res, err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "pkg", "governance", "policy.go")); statErr == nil {
		t.Fatalf("policy.go was written outside staging — bypass not closed")
	}
}

func TestGenerateToolCodeQuotesCommandLiteral(t *testing.T) {
	command := "echo \"hello\"\n; touch /tmp/pwn"
	code := generateToolCode("safe_tool", "safe description", "shell", command, map[string]string{}, true, "once")

	expected := "command := " + strconv.Quote(command)
	if !strings.Contains(code, expected) {
		t.Fatalf("expected generated code to quote command literal; missing %q", expected)
	}

	if _, err := parser.ParseFile(token.NewFileSet(), "generated.go", code, parser.AllErrors); err != nil {
		t.Fatalf("generated code should remain valid Go: %v", err)
	}
}
