package tools

// engram_tool.go — SENSE Engram Recording Tool
//
// Exposes the EngramStore's Record() method to the AI agent as a standard tool.
// The agent calls this tool whenever it discovers a preference or learns a
// reliable pattern, ensuring the preference persists across sessions via AgeMem.
//
// Usage:
//   { "tool": "record_engram",
//     "parameters": {
//       "preference": "Always use sqlite3 -csv for exporting query results",
//       "condition": "When the user asks to export database data to a file",
//       "tool_name": "db_query",
//       "confidence": 0.9
//     }
//   }

import (
	"context"
	"encoding/json"
	"fmt"
)

// EngramRecorderTool writes a learned preference into the persistent EngramStore.
// The EngramStore is injected via the context (set by main.go when SENSE memory
// is initialised); if it's absent, the tool records gracefully to a no-op store.
type EngramRecorderTool struct {
	BaseTool
}

// NewRecordEngramTool constructs the EngramRecorderTool.
func NewRecordEngramTool() *EngramRecorderTool {
	return &EngramRecorderTool{
		BaseTool: BaseTool{
			name: "record_engram",
			description: "Persist a learned preference or reliable pattern to long-term memory " +
				"(SENSE Engram Store). Call this whenever you discover a user preference, " +
				"a tool that works well, or an important fact that should survive across sessions.",
			category:           CategoryMeta,
			requiresPermission: false,
			defaultPermission:  PermissionAlways,
		},
	}
}

func (t *EngramRecorderTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"preference": {
				"type": "string",
				"description": "The learned preference or pattern to remember (e.g. 'User prefers JSON output over CSV')."
			},
			"condition": {
				"type": "string",
				"description": "When does this preference apply? (e.g. 'When the user asks for data exports')."
			},
			"tool_name": {
				"type": "string",
				"description": "Optional: the specific tool this preference relates to."
			},
			"confidence": {
				"type": "number",
				"description": "How confident are you in this preference? 0.0–1.0 (default: 0.7)."
			}
		},
		"required": ["preference", "condition"]
	}`)
}

func (t *EngramRecorderTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	preference, _ := params["preference"].(string)
	condition, _ := params["condition"].(string)
	toolName, _ := params["tool_name"].(string)
	confidence := 0.7
	if v, ok := params["confidence"].(float64); ok {
		// Clamp to [0.0, 1.0] — the agent may return out-of-range values.
		if v < 0 {
			v = 0
		} else if v > 1 {
			v = 1
		}
		confidence = v
	}

	if preference == "" {
		return &ToolResult{Success: false, Error: "preference is required"}, nil
	}
	if condition == "" {
		return &ToolResult{Success: false, Error: "condition is required"}, nil
	}

	summary := fmt.Sprintf("Engram recorded:\n  Preference: %s\n  Condition: %s", preference, condition)
	if toolName != "" {
		summary += fmt.Sprintf("\n  Tool: %s", toolName)
	}
	summary += fmt.Sprintf("\n  Confidence: %.0f%%", confidence*100)

	// Persistence is handled by the Orchestrator post-processing hook that fires
	// after this tool returns (orchestrator.go ExecuteTool, "record_engram" branch).
	// This tool's job is to validate parameters and surface the intent; no direct
	// store access is needed here.

	return &ToolResult{
		Success: true,
		Output:  summary,
	}, nil
}
