package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// TotpGenerateTool generates 2FA codes for automated logins.
type TotpGenerateTool struct {
	BaseTool
}

func NewTotpGenerateTool() *TotpGenerateTool {
	return &TotpGenerateTool{
		BaseTool: BaseTool{
			name:               "totp_generate",
			description:        "Generate a TOTP 2FA code from a base32 secret.",
			category:           CategorySecurity,
			requiresPermission: true,
			defaultPermission:  PermissionOnce,
		},
	}
}

func (t *TotpGenerateTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"secret": map[string]interface{}{
				"type":        "string",
				"description": "Base32 secret (e.g. JBSWY3DPEHPK3PXP)",
			},
		},
		"required": []string{"secret"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *TotpGenerateTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	secret, ok := params["secret"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "secret is required"}, fmt.Errorf("secret required")
	}

	projectRoot := os.Getenv("HOME") + "/project/gorkbot"
	scriptPath := filepath.Join(projectRoot, "scripts/totp_bridge.js")

	command := fmt.Sprintf("node %s %s",
		shellescape(scriptPath),
		shellescape(secret))

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
	})
}
