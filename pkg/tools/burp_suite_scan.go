package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// BurpSuiteScanTool - Custom generated tool
type BurpSuiteScanTool struct {
	BaseTool
}

func NewBurpSuiteScanTool() *BurpSuiteScanTool {
	return &BurpSuiteScanTool{
		BaseTool: BaseTool{
			name:               "burp_suite_scan",
			description:        "Run Burp Suite scans on a remote server to test web application security.",
			category:           CategoryCustom,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *BurpSuiteScanTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"remote_host": map[string]interface{}{
				"type":        "string",
				"description": "remote_host parameter",
			},
			"target_url": map[string]interface{}{
				"type":        "string",
				"description": "target_url parameter",
			},
			"scan_type_flag": map[string]interface{}{
				"type":        "string",
				"description": "scan_type_flag parameter",
			},
			"config_file": map[string]interface{}{
				"type":        "string",
				"description": "config_file parameter",
			},
		},
		"required": []string{"remote_host", "target_url", "scan_type_flag", "config_file"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *BurpSuiteScanTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	config_file := ""
	if p, ok := params["config_file"].(string); ok {
		config_file = p
	}

	remote_host := ""
	if p, ok := params["remote_host"].(string); ok {
		remote_host = p
	}

	target_url := ""
	if p, ok := params["target_url"].(string); ok {
		target_url = p
	}

	scan_type_flag := ""
	if p, ok := params["scan_type_flag"].(string); ok {
		scan_type_flag = p
	}

	command := fmt.Sprintf("ssh %s 'java -jar /path/to/burpsuite_community.jar --project-file=/tmp/burp_project --config-file=%s --scan-target=%s %s' 2>&1",
		shellescape(remote_host), shellescape(config_file), shellescape(target_url), shellescape(scan_type_flag))

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
	})
}
