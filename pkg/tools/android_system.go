package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// LogcatDumpTool captures a snippet of the Android system log.
type LogcatDumpTool struct {
	BaseTool
}

func NewLogcatDumpTool() *LogcatDumpTool {
	return &LogcatDumpTool{
		BaseTool: BaseTool{
			name:               "logcat_dump",
			description:        "Dump recent Android system logs with optional filtering. Useful for debugging app crashes or system events.",
			category:           CategorySystem,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *LogcatDumpTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"filter": map[string]interface{}{
				"type":        "string",
				"description": "Tag filter (e.g., 'ActivityManager:I *:S'). Defaults to '*:V'.",
			},
			"lines": map[string]interface{}{
				"type":        "integer",
				"description": "Number of lines to return (default: 100).",
			},
			"grep": map[string]interface{}{
				"type":        "string",
				"description": "Regex pattern to filter output lines.",
			},
		},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *LogcatDumpTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	filter, _ := args["filter"].(string)
	lines, _ := args["lines"].(int)
	if f, ok := args["lines"].(float64); ok {
		lines = int(f)
	}
	grepPattern, _ := args["grep"].(string)

	if lines <= 0 {
		lines = 100
	}
	if filter == "" {
		filter = "*:V"
	}

	cmdArgs := []string{"-d", "-t", fmt.Sprintf("%d", lines), filter}
	cmd := exec.CommandContext(ctx, "logcat", cmdArgs...)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Logcat failed: %v", err)}, nil
	}

	result := string(out)
	if grepPattern != "" {
		lines := strings.Split(result, "\n")
		var filtered []string
		for _, line := range lines {
			if strings.Contains(line, grepPattern) {
				filtered = append(filtered, line)
			}
		}
		result = strings.Join(filtered, "\n")
	}

	return &ToolResult{Success: true, Output: result}, nil
}

// ClipboardManagerTool reads or writes to the system clipboard via Termux:API.
type ClipboardManagerTool struct {
	BaseTool
}

func NewClipboardManagerTool() *ClipboardManagerTool {
	return &ClipboardManagerTool{
		BaseTool: BaseTool{
			name:               "clipboard_manager",
			description:        "Read or write to the Android system clipboard using Termux:API.",
			category:           CategorySystem,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *ClipboardManagerTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"read", "write"},
				"description": "Action to perform: 'read' or 'write'.",
			},
			"text": map[string]interface{}{
				"type":        "string",
				"description": "Text to write to clipboard (required for 'write').",
			},
		},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *ClipboardManagerTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	action, _ := args["action"].(string)
	text, _ := args["text"].(string)

	if action == "write" {
		cmd := exec.CommandContext(ctx, "termux-clipboard-set", text)
		if err := cmd.Run(); err != nil {
			return &ToolResult{Success: false, Error: fmt.Sprintf("Clipboard write failed (is Termux:API installed?): %v", err)}, nil
		}
		return &ToolResult{Success: true, Output: "Text copied to clipboard."}, nil
	}

	// Read
	cmd := exec.CommandContext(ctx, "termux-clipboard-get")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Clipboard read failed (is Termux:API installed?): %v", err)}, nil
	}
	return &ToolResult{Success: true, Output: string(out)}, nil
}

// NotificationListenerTool reads active notifications via Termux:API.
type NotificationListenerTool struct {
	BaseTool
}

func NewNotificationListenerTool() *NotificationListenerTool {
	return &NotificationListenerTool{
		BaseTool: BaseTool{
			name:               "notification_listener",
			description:        "List active Android notifications using Termux:API.",
			category:           CategorySystem,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *NotificationListenerTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"filter": map[string]interface{}{
				"type":        "string",
				"description": "Filter by package name or title substring.",
			},
		},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *NotificationListenerTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	filter, _ := args["filter"].(string)

	cmd := exec.CommandContext(ctx, "termux-notification-list")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Failed to list notifications (needs Termux:API): %v", err)}, nil
	}

	result := string(out)
	if filter != "" {
		if !strings.Contains(result, filter) {
			return &ToolResult{Success: true, Output: "[] (No matches found)"}, nil
		}
	}

	return &ToolResult{Success: true, Output: result}, nil
}
