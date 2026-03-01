package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/pipeline"
)

// ─────────────────────────────────────────────────────────────────────────────
// RunPipelineTool
// ─────────────────────────────────────────────────────────────────────────────

// RunPipelineTool executes a multi-step agentic pipeline, passing outputs from
// each step as template variables to subsequent steps.
type RunPipelineTool struct {
	BaseTool
}

func NewRunPipelineTool() *RunPipelineTool {
	return &RunPipelineTool{
		BaseTool: BaseTool{
			name: "run_pipeline",
			description: `Execute a sequential multi-step agentic pipeline. Each step runs a specialized sub-agent and can reference outputs from prior steps via {{outputs.step_name}} placeholders. Steps with no DependsOn run first; steps declare DependsOn to sequence after others.

Example steps JSON:
[
  {"name":"recon","agent_type":"redteam-recon","task":"Map the attack surface of {{target}}"},
  {"name":"inject","agent_type":"redteam-injection","task":"Test injection vectors found in recon:\n{{outputs.recon}}","depends_on":["recon"]},
  {"name":"report","agent_type":"redteam-reporter","task":"Write a consolidated report:\nRecon: {{outputs.recon}}\nInjection: {{outputs.inject}}","depends_on":["recon","inject"]}
]`,
			category:           CategoryAI,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *RunPipelineTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {
				"type": "string",
				"description": "A short descriptive name for this pipeline (e.g. 'security-assessment')"
			},
			"steps": {
				"type": "array",
				"description": "JSON array of pipeline steps. Each step has: name, agent_type, task, depends_on (optional), timeout_seconds (optional).",
				"items": {
					"type": "object",
					"properties": {
						"name":            {"type": "string"},
						"agent_type":      {"type": "string"},
						"task":            {"type": "string"},
						"depends_on":      {"type": "array", "items": {"type": "string"}},
						"timeout_seconds": {"type": "number"}
					},
					"required": ["name", "agent_type", "task"]
				}
			}
		},
		"required": ["steps"]
	}`)
}

func (t *RunPipelineTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	pipelineName, _ := params["name"].(string)
	if pipelineName == "" {
		pipelineName = "pipeline"
	}

	// Parse steps — accept either a JSON string or a pre-decoded []interface{}.
	var steps []pipeline.Step
	switch v := params["steps"].(type) {
	case string:
		if err := json.Unmarshal([]byte(v), &steps); err != nil {
			return &ToolResult{Success: false, Error: fmt.Sprintf("invalid steps JSON: %v", err)}, nil
		}
	case []interface{}:
		raw, _ := json.Marshal(v)
		if err := json.Unmarshal(raw, &steps); err != nil {
			return &ToolResult{Success: false, Error: fmt.Sprintf("failed to decode steps: %v", err)}, nil
		}
	case nil:
		return &ToolResult{Success: false, Error: "steps parameter is required"}, nil
	default:
		return &ToolResult{Success: false, Error: fmt.Sprintf("steps must be a JSON array, got %T", params["steps"])}, nil
	}

	if len(steps) == 0 {
		return &ToolResult{Success: false, Error: "steps array is empty"}, nil
	}

	// Convert timeout_seconds → time.Duration (json numbers come as float64).
	for i := range steps {
		if steps[i].Timeout == 0 {
			steps[i].Timeout = 5 * time.Minute // default per-step timeout
		}
	}

	// Get pipeline runner from registry stored in context.
	reg, _ := ctx.Value(registryContextKey).(*Registry)
	var runner pipeline.AgentRunner
	if reg != nil {
		runner = reg.GetPipelineRunner()
	}
	if runner == nil {
		return &ToolResult{
			Success: false,
			Error:   "pipeline runner not available — orchestrator not wired",
		}, nil
	}

	eng := pipeline.NewEngine(runner)
	p := pipeline.Pipeline{Name: pipelineName, Steps: steps}

	outputs, err := eng.Execute(ctx, p)

	// Format result regardless of error (partial results are useful).
	summary := pipeline.FormatResults(pipelineName, outputs, steps)

	if err != nil {
		return &ToolResult{
			Success: false,
			Output:  summary,
			Error:   fmt.Sprintf("pipeline failed: %v", err),
		}, nil
	}

	// Build a compact final summary.
	var sb strings.Builder
	sb.WriteString(summary)
	sb.WriteString(fmt.Sprintf("---\nPipeline %q completed. %d steps executed.\n", pipelineName, len(steps)))

	return &ToolResult{
		Success:      true,
		Output:       sb.String(),
		OutputFormat: FormatText,
	}, nil
}
