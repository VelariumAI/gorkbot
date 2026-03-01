package tools

// audit.go — AuditToolCallTool logs tool invocations to the Gorkbot state DB.
// Integrated from dynamic tool history; rewritten with injection-safe SQL.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// AuditToolCallTool logs every tool call to the `calls` table in state.db.
type AuditToolCallTool struct{ BaseTool }

func NewAuditToolCallTool() *AuditToolCallTool {
	return &AuditToolCallTool{BaseTool: BaseTool{
		name:               "audit_tool_call",
		description:        "Log a tool call to the Gorkbot audit log (calls table in state.db). Records tool name, parameters, and result status.",
		category:           CategoryMeta,
		requiresPermission: false,
		defaultPermission:  PermissionAlways,
	}}
}

func (t *AuditToolCallTool) Parameters() json.RawMessage {
	s, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"tool_name": map[string]interface{}{
				"type":        "string",
				"description": "Name of the tool being audited",
			},
			"params_json": map[string]interface{}{
				"type":        "string",
				"description": "JSON-encoded parameters passed to the tool",
			},
			"status": map[string]interface{}{
				"type":        "string",
				"description": "Result status: 'success', 'error', or 'pending'",
				"enum":        []string{"success", "error", "pending"},
			},
		},
		"required": []string{"tool_name"},
	})
	return s
}

func (t *AuditToolCallTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	toolName, _ := params["tool_name"].(string)
	if toolName == "" {
		return &ToolResult{Output: "error: tool_name is required"}, nil
	}
	paramsJSON, _ := params["params_json"].(string)
	status, _ := params["status"].(string)
	if status == "" {
		status = "pending"
	}

	// Ensure DB and schema exist.
	db := gorkStateDB()
	if err := os.MkdirAll(filepath.Dir(db), 0700); err != nil {
		return &ToolResult{Output: fmt.Sprintf("error creating DB dir: %v", err)}, nil
	}

	// Use single-quoted SQL literals with escaping — no shell variable expansion.
	sql := fmt.Sprintf(
		"INSERT INTO calls (tool, params, status) VALUES ('%s', '%s', '%s');",
		sqlEscapeSingleQuote(toolName),
		sqlEscapeSingleQuote(paramsJSON),
		sqlEscapeSingleQuote(status),
	)

	// Init schema then insert.
	initCmd := fmt.Sprintf("sqlite3 %s %s 2>/dev/null || true",
		shellescape(db), shellescape(gorkStateDBInit()))
	insertCmd := fmt.Sprintf("sqlite3 %s %s",
		shellescape(db), shellescape(sql))

	bash := NewBashTool()
	bash.Execute(ctx, map[string]interface{}{"command": initCmd}) //nolint:errcheck

	result, err := bash.Execute(ctx, map[string]interface{}{"command": insertCmd})
	if err != nil {
		return &ToolResult{Output: fmt.Sprintf("audit insert error: %v", err)}, nil
	}
	if result != nil && result.Error != "" {
		return &ToolResult{Output: fmt.Sprintf("audit insert failed: %s", result.Error)}, nil
	}

	return &ToolResult{Output: fmt.Sprintf("Logged: tool=%s status=%s", toolName, status)}, nil
}
