package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// IntentBroadcastTool sends raw Android Intents using the 'am' command.
type IntentBroadcastTool struct {
	BaseTool
}

func NewIntentBroadcastTool() *IntentBroadcastTool {
	return &IntentBroadcastTool{
		BaseTool: BaseTool{
			name:               "intent_broadcast",
			description:        "Send a raw Android Intent using 'am broadcast' or 'am start'. Use to trigger deep links, specific activities, or system broadcasts.",
			category:           CategorySystem,
			requiresPermission: true,
			defaultPermission:  PermissionOnce,
		},
	}
}

func (t *IntentBroadcastTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]string{
				"type":        "string",
				"description": "The intent action (e.g., android.intent.action.VIEW, com.example.ACTION_UPDATE).",
			},
			"data": map[string]string{
				"type":        "string",
				"description": "The data URI (e.g., https://example.com, content://contacts). Optional.",
			},
			"package": map[string]string{
				"type":        "string",
				"description": "Target package name (e.g., com.android.chrome). Optional.",
			},
			"component": map[string]string{
				"type":        "string",
				"description": "Target component (e.g., com.example/.MainActivity). Optional.",
			},
			"extras": map[string]string{
				"type":        "string",
				"description": "Key-value pairs for extras (e.g., --es key value --ei int_key 123). Format as command line flags.",
			},
			"type": map[string]string{
				"type":        "string",
				"description": "MIME type (e.g., image/png). Optional.",
			},
			"mode": map[string]interface{}{
				"type":        "string",
				"description": "Execution mode: 'broadcast' (default), 'start' (activity), 'service' (start service).",
				"enum":        []string{"broadcast", "start", "service"},
			},
		},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *IntentBroadcastTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	action, _ := args["action"].(string)
	data, _ := args["data"].(string)
	pkg, _ := args["package"].(string)
	comp, _ := args["component"].(string)
	extras, _ := args["extras"].(string)
	mimeType, _ := args["type"].(string)
	mode, _ := args["mode"].(string)

	if mode == "" {
		mode = "broadcast"
	}

	cmdArgs := []string{mode}
	if action != "" {
		cmdArgs = append(cmdArgs, "-a", action)
	}
	if data != "" {
		cmdArgs = append(cmdArgs, "-d", data)
	}
	if pkg != "" {
		cmdArgs = append(cmdArgs, "-p", pkg)
	}
	if comp != "" {
		cmdArgs = append(cmdArgs, "-n", comp)
	}
	if mimeType != "" {
		cmdArgs = append(cmdArgs, "-t", mimeType)
	}
	if extras != "" {
		parts := strings.Fields(extras)
		cmdArgs = append(cmdArgs, parts...)
	}

	cmd := exec.CommandContext(ctx, "am", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Intent failed: %v\nOutput: %s", err, string(out)),
		}, nil
	}

	return &ToolResult{
		Success: true,
		Output:  fmt.Sprintf("Intent sent successfully.\nCommand: am %s\nOutput: %s", strings.Join(cmdArgs, " "), string(out)),
	}, nil
}
