package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/velariumai/gorkbot/pkg/vision"
)

// ADBSetupTool checks ADB connection status, auto-discovers the device IP
// and wireless debugging port, and attempts auto-connect. If connection
// cannot be established it prints the exact commands the user needs to run.
type ADBSetupTool struct{ BaseTool }

func NewADBSetupTool() *ADBSetupTool {
	return &ADBSetupTool{BaseTool{
		name: "adb_setup",
		description: "Check wireless ADB connection status and auto-connect if possible. " +
			"Discovers device IP and ADB port automatically from /proc/net/tcp. " +
			"If not yet paired, prints the exact adb pair + adb connect commands to run. " +
			"Required for all vision tools (screen capture).",
		category:           CategoryAndroid,
		requiresPermission: false, // read-only diagnostics + adb connect — safe to run freely
		defaultPermission:  PermissionAlways,
	}}
}

func (t *ADBSetupTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "\"status\" (default) — show connection info; \"connect\" — attempt auto-connect; \"disconnect\" — run adb disconnect",
				"enum":        []string{"status", "connect", "disconnect"},
			},
		},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *ADBSetupTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	action := "connect" // default: always try to connect
	if a, ok := params["action"].(string); ok && a != "" {
		action = a
	}

	switch action {
	case "status":
		return t.status(ctx)
	case "disconnect":
		return t.disconnect(ctx)
	default:
		return t.connect(ctx)
	}
}

func (t *ADBSetupTool) status(ctx context.Context) (*ToolResult, error) {
	status := vision.SetupStatus(ctx)
	return &ToolResult{
		Success: true,
		Output:  "## ADB Status\n\n```\n" + status + "\n```",
		Data:    map[string]interface{}{"status": status},
	}, nil
}

func (t *ADBSetupTool) connect(ctx context.Context) (*ToolResult, error) {
	result, err := vision.AutoConnect(ctx)
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   "Auto-connect error: " + err.Error(),
		}, nil
	}

	if result.Connected {
		addr := result.Address
		if addr == "" {
			addr = "existing connection"
		}
		out := fmt.Sprintf("## ADB Connected\n\n✓ Connected to %s\n\nVision tools are ready.", addr)
		return &ToolResult{
			Success: true,
			Output:  out,
			Data: map[string]interface{}{
				"connected": true,
				"address":   addr,
			},
		}, nil
	}

	// Not connected — result.Message has precise instructions
	var sb strings.Builder
	sb.WriteString("## ADB Setup Required\n\n")
	sb.WriteString(result.Message)
	sb.WriteString("\n\n---\n")

	// Append quick-reference status
	if status := vision.SetupStatus(ctx); status != "" {
		sb.WriteString("\n**Current state:**\n```\n")
		sb.WriteString(status)
		sb.WriteString("\n```")
	}

	return &ToolResult{
		Success: false,
		Output:  sb.String(),
		Data: map[string]interface{}{
			"connected": false,
			"device_ip": result.DeviceIP,
			"ports":     result.Ports,
			"message":   result.Message,
		},
	}, nil
}

func (t *ADBSetupTool) disconnect(ctx context.Context) (*ToolResult, error) {
	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": "/data/data/com.termux/files/usr/bin/adb disconnect",
	})
}
