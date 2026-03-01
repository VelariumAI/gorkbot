package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/velariumai/gorkbot/pkg/colony"
)

// ColonyDebateTool spawns a multi-agent bee colony debate
type ColonyDebateTool struct {
	BaseTool
	runner func(ctx context.Context, sys, prompt string) (string, error)
}

// NewColonyDebateTool creates a new ColonyDebateTool with the given runner.
func NewColonyDebateTool(runner func(ctx context.Context, sys, prompt string) (string, error)) *ColonyDebateTool {
	return &ColonyDebateTool{
		BaseTool: NewBaseTool(
			"colony_debate",
			"Run a multi-agent bee colony debate where parallel analyst bees examine a question from different perspectives (advocate, critic, pragmatist, contrarian), then a synthesizer merges their insights. Use for complex decisions, architectural choices, or any question benefiting from multiple viewpoints.",
			CategoryAI,
			false,
			PermissionAlways,
		),
		runner: runner,
	}
}

func (t *ColonyDebateTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"question": {
				"type": "string",
				"description": "The question or topic to debate. Be specific and concrete."
			},
			"roles": {
				"type": "array",
				"description": "Optional custom roles. If omitted, uses default 4-bee colony (advocate/critic/pragmatist/contrarian).",
				"items": {
					"type": "object",
					"properties": {
						"name":   {"type": "string"},
						"stance": {"type": "string"},
						"focus":  {"type": "string"}
					},
					"required": ["name", "stance", "focus"]
				}
			}
		},
		"required": ["question"]
	}`)
}

func (t *ColonyDebateTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	question, _ := params["question"].(string)
	if question == "" {
		return &ToolResult{Success: false, Error: "question is required"}, nil
	}

	var roles []colony.Role
	if rawRoles, ok := params["roles"].([]interface{}); ok {
		for _, r := range rawRoles {
			if m, ok := r.(map[string]interface{}); ok {
				roles = append(roles, colony.Role{
					Name:   fmt.Sprintf("%v", m["name"]),
					Stance: fmt.Sprintf("%v", m["stance"]),
					Focus:  fmt.Sprintf("%v", m["focus"]),
				})
			}
		}
	}

	if t.runner == nil {
		return &ToolResult{Success: false, Error: "colony runner not configured"}, nil
	}

	hive := colony.NewHive(t.runner)
	result, err := hive.Debate(ctx, question, roles)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("debate failed: %v", err)}, nil
	}

	// Truncate if very long
	if len(result) > 12000 {
		result = result[:12000] + "\n\n... [truncated]"
	}

	return &ToolResult{
		Success:      true,
		Output:       result,
		OutputFormat: FormatText,
	}, nil
}
