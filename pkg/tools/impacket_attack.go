package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// ImpacketAttackTool - Custom generated tool
type ImpacketAttackTool struct {
	BaseTool
}

func NewImpacketAttackTool() *ImpacketAttackTool {
	return &ImpacketAttackTool{
		BaseTool: BaseTool{
			name:               "impacket_attack",
			description:        "Execute Impacket scripts for Windows network protocol attacks, such as SMB relay or Kerberos exploitation.",
			category:           CategoryCustom,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *ImpacketAttackTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"script": map[string]interface{}{
				"type":        "string",
				"description": "script parameter",
			},
			"target": map[string]interface{}{
				"type":        "string",
				"description": "target parameter",
			},
			"options": map[string]interface{}{
				"type":        "string",
				"description": "options parameter",
			},
		},
		"required": []string{"script", "target", "options"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *ImpacketAttackTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	target := ""
	if p, ok := params["target"].(string); ok {
		target = p
	}

	options := ""
	if p, ok := params["options"].(string); ok {
		options = p
	}

	script := ""
	if p, ok := params["script"].(string); ok {
		script = p
	}

	command := fmt.Sprintf("python3 -m impacket.%s %s %s 2>&1",
		shellescape(script), shellescape(target), shellescape(options))

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
	})
}
