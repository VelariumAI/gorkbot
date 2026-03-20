package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
)

// SmartReadFileTool - Custom generated tool
type SmartReadFileTool struct {
	BaseTool
}

func NewSmartReadFileTool() *SmartReadFileTool {
	return &SmartReadFileTool{
		BaseTool: BaseTool{
			name:               "smart_read_file",
			description:        "Unified file reader with raw/MCP/hashed modes (pruned for efficiency).",
			category:           CategoryFile,
			requiresPermission: true,
			defaultPermission:  PermissionOnce,
		},
	}
}

func (t *SmartReadFileTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"head": map[string]interface{}{
				"type":        "integer",
				"description": "head parameter",
			},
			"tail": map[string]interface{}{
				"type":        "integer",
				"description": "tail parameter",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "path parameter",
			},
			"mode": map[string]interface{}{
				"type":        "string",
				"description": "mode parameter",
			},
		},
		"required": []string{"head", "tail", "path", "mode"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *SmartReadFileTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	mode := ""
	if p, ok := params["mode"].(string); ok {
		mode = p
	}

	head := ""
	if p, ok := params["head"].(string); ok {
		head = p
	}

	tail := ""
	if p, ok := params["tail"].(string); ok {
		tail = p
	}

	path := ""
	if p, ok := params["path"].(string); ok {
		path = p
	}

	command := "if [ \"" + mode + "\" = \"hashed\" ]; then read_file_hashed \"" + path + "\"; elif [ \"" + mode + "\" = \"mcp\" ]; then mcp_filesystem_read_text_file \"" + path + "\" head=\"" + strconv.Itoa(head) + "\" tail=\"" + strconv.Itoa(tail) + "\"; else read_file \"" + path + "\"; fi"

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
	})
}
