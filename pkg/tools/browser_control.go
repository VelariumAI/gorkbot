package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// BrowserControlTool provides comprehensive browser automation via Puppeteer bridge.
type BrowserControlTool struct {
	BaseTool
}

func NewBrowserControlTool() *BrowserControlTool {
	return &BrowserControlTool{
		BaseTool: BaseTool{
			name:              "browser_control",
			description:       "Comprehensive browser automation (navigate, click, type, screenshot, evaluate, wait). Supports React/SPA.",
			category:          CategoryWeb,
			requiresPermission: true,
			defaultPermission: PermissionOnce,
		},
	}
}

func (t *BrowserControlTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"navigate", "click", "type", "screenshot", "content", "evaluate", "wait_for"},
				"description": "Action to perform in the browser",
			},
			"url": map[string]interface{}{
				"type":        "string",
				"description": "URL to navigate to (for 'navigate' action)",
			},
			"selector": map[string]interface{}{
				"type":        "string",
				"description": "CSS selector for click/type/wait_for actions",
			},
			"text": map[string]interface{}{
				"type":        "string",
				"description": "Text to type (for 'type' action)",
			},
			"script": map[string]interface{}{
				"type":        "string",
				"description": "JavaScript to evaluate (for 'evaluate' action)",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to save screenshot (for 'screenshot' action, default: /sdcard/browser_shot.png)",
			},
			"timeout": map[string]interface{}{
				"type":        "number",
				"description": "Timeout in milliseconds (default: 30000)",
			},
			"wait_until": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"load", "domcontentloaded", "networkidle0", "networkidle2"},
				"description": "Condition to wait for after navigation (default: load)",
			},
		},
		"required": []string{"action"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *BrowserControlTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	action, ok := params["action"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "action is required"}, fmt.Errorf("action required")
	}

	// Prepare parameters JSON for the bridge script
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return &ToolResult{Success: false, Error: "failed to marshal params"}, err
	}

	// Find the project root to locate the script
	projectRoot := os.Getenv("HOME") + "/project/gorkbot"
	scriptPath := filepath.Join(projectRoot, "scripts/browser_bridge.js")

	// Command to run the node script
	command := fmt.Sprintf("node %s %s %s", 
		shellescape(scriptPath), 
		shellescape(action), 
		shellescape(string(paramsJSON)))

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
	})
}
