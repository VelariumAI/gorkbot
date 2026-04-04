package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// AccessibilityQueryTool queries the Android accessibility tree via Termux:API.
type AccessibilityQueryTool struct {
	BaseTool
}

func NewAccessibilityQueryTool() *AccessibilityQueryTool {
	return &AccessibilityQueryTool{
		BaseTool: BaseTool{
			name:               "accessibility_query",
			description:        "Query the Android accessibility tree to inspect UI elements.",
			category:           CategorySystem,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *AccessibilityQueryTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"package": map[string]interface{}{
				"type":        "string",
				"description": "Filter by package name (optional).",
			},
		},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *AccessibilityQueryTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	if _, err := exec.LookPath("adb"); err != nil {
		return &ToolResult{
			Success: false,
			Error:   "adb is not installed or not in PATH; install android-tools to use accessibility_query",
		}, nil
	}

	packageFilter, _ := args["package"].(string)
	packageFilter = strings.TrimSpace(packageFilter)

	cmd := exec.CommandContext(ctx, "adb", "shell", "uiautomator", "dump", "/sdcard/window_dump.xml")
	if err := cmd.Run(); err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("failed to dump UI with adb: %v (is a device connected and authorized?)", err),
		}, nil
	}

	cmdRead := exec.CommandContext(ctx, "adb", "shell", "cat", "/sdcard/window_dump.xml")
	out, err := cmdRead.CombinedOutput()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("failed to read UI dump: %v", err)}, nil
	}

	xml := string(out)
	if packageFilter != "" {
		needle := fmt.Sprintf(`package="%s"`, packageFilter)
		if !strings.Contains(xml, needle) {
			return &ToolResult{
				Success: true,
				Output:  fmt.Sprintf("No accessibility nodes found for package %q.", packageFilter),
				Data: map[string]interface{}{
					"package_filter": packageFilter,
					"matched":        false,
				},
			}, nil
		}
	}

	return &ToolResult{
		Success: true,
		Output:  xml,
		Data: map[string]interface{}{
			"package_filter": packageFilter,
			"matched":        true,
		},
	}, nil
}
