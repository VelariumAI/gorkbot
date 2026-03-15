package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

type TermuxSensorTool struct {
	BaseTool
}

func NewTermuxSensorTool() *TermuxSensorTool {
	return &TermuxSensorTool{
		BaseTool: BaseTool{
			name:               "termux_sensor",
			description:        "Reads hardware sensors on an Android device via Termux:API.",
			category:           CategorySystem,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *TermuxSensorTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"sensor": map[string]interface{}{"type": "string", "description": "Sensor to read (e.g. all, light, gravity)"},
		},
		"required": []string{"sensor"},
	}
	b, _ := json.Marshal(schema)
	return b
}

func (t *TermuxSensorTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	sensor, _ := params["sensor"].(string)
	cmd := fmt.Sprintf("termux-sensor -s %s -n 1", sensor)
	bash := NewBashTool()
	return bash.Execute(ctx, map[string]interface{}{"command": cmd})
}

type TermuxLocationTool struct {
	BaseTool
}

func NewTermuxLocationTool() *TermuxLocationTool {
	return &TermuxLocationTool{
		BaseTool: BaseTool{
			name:               "termux_location",
			description:        "Gets the device location via Termux:API.",
			category:           CategorySystem,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *TermuxLocationTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"provider": map[string]interface{}{"type": "string", "description": "Provider to use (gps, network, passive)"},
		},
	}
	b, _ := json.Marshal(schema)
	return b
}

func (t *TermuxLocationTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	cmd := "termux-location"
	if p, ok := params["provider"].(string); ok && p != "" {
		cmd += " -p " + p
	}
	bash := NewBashTool()
	return bash.Execute(ctx, map[string]interface{}{"command": cmd})
}
