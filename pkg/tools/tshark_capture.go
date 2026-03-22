package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// TsharkCaptureTool - Custom generated tool
type TsharkCaptureTool struct {
	BaseTool
}

func NewTsharkCaptureTool() *TsharkCaptureTool {
	return &TsharkCaptureTool{
		BaseTool: BaseTool{
			name:               "tshark_capture",
			description:        "Capture and analyze network packets using tshark on a remote server.",
			category:           CategoryCustom,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *TsharkCaptureTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"remote_host": map[string]interface{}{
				"type":        "string",
				"description": "remote_host parameter",
			},
			"interface": map[string]interface{}{
				"type":        "string",
				"description": "interface parameter",
			},
			"filter": map[string]interface{}{
				"type":        "string",
				"description": "filter parameter",
			},
			"packet_count": map[string]interface{}{
				"type":        "integer",
				"description": "packet_count parameter",
			},
		},
		"required": []string{"remote_host", "interface", "filter", "packet_count"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *TsharkCaptureTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	// Parse parameters
	remoteHost := getStringParam(params, "remote_host", "")
	iface := getStringParam(params, "interface", "")
	filter := getStringParam(params, "filter", "")
	packetCount := "100" // default

	// packet_count may come as float64 (JSON number) or string
	switch v := params["packet_count"].(type) {
	case float64:
		packetCount = fmt.Sprintf("%.0f", v)
	case int:
		packetCount = fmt.Sprintf("%d", v)
	case string:
		if v != "" {
			packetCount = v
		}
	}

	// Shell-escape all parameters to prevent command injection
	// Use single quotes with escaped inner quotes
	remoteHost = escapeForShell(remoteHost)
	iface = escapeForShell(iface)
	filter = escapeForShell(filter)
	packetCount = escapeForShell(packetCount)

	command := fmt.Sprintf("ssh %s 'tshark -i %s -f %s -c %s -w /tmp/capture.pcap 2>&1 && scp %s:/tmp/capture.pcap /data/data/com.termux/files/home/capture.pcap' 2>&1", remoteHost, iface, filter, packetCount, remoteHost)

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
	})
}

// escapeForShell escapes a string for safe use in shell commands
func escapeForShell(s string) string {
	// Wrap in single quotes and escape single quotes within the string
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
