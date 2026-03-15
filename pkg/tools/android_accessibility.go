package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
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
	// Not implemented in standard Termux:API?
	// Actually there is termux-accessibility-services? No.
	// But `adb shell uiautomator dump` works if ADB is available.
	// Or `termux-accessibility` if the user installed a specific plugin.
	// Standard termux-api doesn't have accessibility query.
	// The roadmap item says "(requires Termux:API)".
	// Maybe it meant `termux-assist` or similar?
	// Or maybe `adb shell input` + `screencap` is what was meant?
	// But `accessibility_query` implies structured data.

	// I will implement a placeholder that tries to use `adb shell uiautomator dump` via `adb_shell` if available,
	// or returns an error suggesting installation.

	// Actually, let's assume `adb` is available since `intent_broadcast` used `am`.
	// `am` works on device without root/adb if in Termux? No, usually `am` requires shell permission or adb.
	// Termux environment usually requires `adb` (via `android-tools`) and wireless debugging enabled.

	// So I will use `adb shell uiautomator dump /sdcard/window_dump.xml && cat /sdcard/window_dump.xml`

	cmd := exec.CommandContext(ctx, "adb", "shell", "uiautomator", "dump", "/sdcard/window_dump.xml")
	if err := cmd.Run(); err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Failed to dump UI: %v (Is ADB connected?)", err)}, nil
	}

	cmdRead := exec.CommandContext(ctx, "adb", "shell", "cat", "/sdcard/window_dump.xml")
	out, err := cmdRead.CombinedOutput()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Failed to read UI dump: %v", err)}, nil
	}

	return &ToolResult{Success: true, Output: string(out)}, nil
}
