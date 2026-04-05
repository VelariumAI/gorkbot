//go:build !android

package tools

import (
	"context"
	"encoding/json"
)

// Non-Android fallback so tool packs compile on all platforms.
type TermuxSensorTool struct {
	BaseTool
}

func NewTermuxSensorTool() *TermuxSensorTool {
	return &TermuxSensorTool{
		BaseTool: BaseTool{
			name:               "termux_sensor",
			description:        "Android-only tool (Termux:API required).",
			category:           CategorySystem,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *TermuxSensorTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
	b, _ := json.Marshal(schema)
	return b
}

func (t *TermuxSensorTool) Execute(_ context.Context, _ map[string]interface{}) (*ToolResult, error) {
	return &ToolResult{Success: false, Error: "termux_sensor is only available on Android/Termux"}, nil
}

type TermuxLocationTool struct {
	BaseTool
}

func NewTermuxLocationTool() *TermuxLocationTool {
	return &TermuxLocationTool{
		BaseTool: BaseTool{
			name:               "termux_location",
			description:        "Android-only tool (Termux:API required).",
			category:           CategorySystem,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *TermuxLocationTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
	b, _ := json.Marshal(schema)
	return b
}

func (t *TermuxLocationTool) Execute(_ context.Context, _ map[string]interface{}) (*ToolResult, error) {
	return &ToolResult{Success: false, Error: "termux_location is only available on Android/Termux"}, nil
}
