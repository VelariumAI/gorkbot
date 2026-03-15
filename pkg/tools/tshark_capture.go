package tools

import (
	"context"
	"encoding/json"
	"fmt"
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
	iface := ""
	if p, ok := params["interface"].(string); ok {
		iface = p
	}

	filter := ""
	if p, ok := params["filter"].(string); ok {
		filter = p
	}

	packet_count := ""
	if p, ok := params["packet_count"].(string); ok {
		packet_count = p
	}

	remote_host := ""
	if p, ok := params["remote_host"].(string); ok {
		remote_host = p
	}

	command := fmt.Sprintf("ssh %s 'tshark -i %s -f \"%s\" -c %s -w /tmp/capture.pcap 2>&1 && scp %s:/tmp/capture.pcap /data/data/com.termux/files/home/capture.pcap' 2>&1", remote_host, iface, filter, packet_count, remote_host)

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
	})
}
