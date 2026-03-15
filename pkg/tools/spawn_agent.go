// Package tools — spawn_agent.go
//
// SpawnAgentTool allows the primary AI to fire a sub-agent in the background
// and collect its result later. This enables parallel execution of independent
// sub-tasks — inspired by oh-my-opencode's background-task tool.
//
// Workflow:
//  1. AI calls spawn_agent with a label and prompt → gets back an agent_id.
//  2. AI continues working on other parts of the task.
//  3. When ready, AI calls collect_agent with the agent_id to retrieve the result.
//     If the agent is still running, collect_agent will wait (with timeout).
//
// This tool requires that an BackgroundAgentManager is available in the context
// (injected by the orchestrator via engine.BackgroundAgentManagerToContext).
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// backgroundAgentManagerKey is used to retrieve the manager from context.
// We use an interface to avoid import cycles with internal/engine.
type BackgroundSpawner interface {
	SpawnFromTool(ctx context.Context, label, prompt, model string) (string, error)
	CollectFromTool(ctx context.Context, agentID string, timeoutSec int) (string, error)
	ListRunningFromTool() []BackgroundAgentInfo
}

// BackgroundAgentInfo is a minimal snapshot for listing.
type BackgroundAgentInfo struct {
	ID      string
	Label   string
	Status  string
	Elapsed time.Duration
}

type bgSpawnerKey struct{}

// BackgroundSpawnerToContext stores a BackgroundSpawner in the context.
func BackgroundSpawnerToContext(ctx context.Context, spawner BackgroundSpawner) context.Context {
	return context.WithValue(ctx, bgSpawnerKey{}, spawner)
}

// BackgroundSpawnerFromContext retrieves the spawner from context.
func BackgroundSpawnerFromContext(ctx context.Context) BackgroundSpawner {
	if v := ctx.Value(bgSpawnerKey{}); v != nil {
		if s, ok := v.(BackgroundSpawner); ok {
			return s
		}
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// SpawnAgentTool
// ─────────────────────────────────────────────────────────────────────────────

// SpawnAgentTool fires a background AI sub-agent asynchronously.
type SpawnAgentTool struct {
	BaseTool
}

func NewSpawnAgentTool() *SpawnAgentTool {
	return &SpawnAgentTool{
		BaseTool: BaseTool{
			name:               "spawn_agent",
			description:        "Launch a background AI sub-agent to handle a parallel task. Returns an agent_id immediately — the agent runs asynchronously. Call collect_agent with that ID when you need the result. Use this to parallelise independent work: e.g. research docs while implementing code.",
			category:           CategoryAI,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *SpawnAgentTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"label": map[string]interface{}{
				"type":        "string",
				"description": "Short human-readable label for the task, shown in the TUI (e.g. 'search-auth-docs')",
			},
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "Full prompt for the sub-agent. Be specific and self-contained — the agent has no conversation history.",
			},
			"model": map[string]interface{}{
				"type":        "string",
				"description": "Optional model ID to use (e.g. 'gemini-2.0-flash'). Defaults to the current secondary model.",
			},
		},
		"required": []string{"label", "prompt"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *SpawnAgentTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	label, _ := params["label"].(string)
	prompt, _ := params["prompt"].(string)
	model, _ := params["model"].(string)

	if prompt == "" {
		return &ToolResult{Success: false, Output: "prompt is required", OutputFormat: FormatError}, nil
	}
	if label == "" {
		label = "sub-agent"
	}

	spawner := BackgroundSpawnerFromContext(ctx)
	if spawner == nil {
		return &ToolResult{
			Success:      false,
			Output:       "background agent manager not available in this context",
			OutputFormat: FormatError,
		}, nil
	}

	id, err := spawner.SpawnFromTool(ctx, label, prompt, model)
	if err != nil {
		return &ToolResult{
			Success:      false,
			Output:       fmt.Sprintf("failed to spawn agent: %v", err),
			OutputFormat: FormatError,
		}, nil
	}

	return &ToolResult{
		Success:      true,
		Output:       fmt.Sprintf("Background agent spawned.\nagent_id: %s\nlabel: %s\n\nUse collect_agent with agent_id=%q to retrieve the result when ready.", id, label, id),
		OutputFormat: FormatText,
	}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// CollectAgentTool
// ─────────────────────────────────────────────────────────────────────────────

// CollectAgentTool retrieves the result of a previously spawned background agent.
type CollectAgentTool struct {
	BaseTool
}

func NewCollectAgentTool() *CollectAgentTool {
	return &CollectAgentTool{
		BaseTool: BaseTool{
			name:               "collect_agent",
			description:        "Collect the result of a background agent previously started with spawn_agent. Blocks until the agent completes or the timeout expires. Returns the agent's full output.",
			category:           CategoryAI,
			requiresPermission: false,
			defaultPermission:  PermissionAlways,
		},
	}
}

func (t *CollectAgentTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"agent_id": map[string]interface{}{
				"type":        "string",
				"description": "The agent_id returned by spawn_agent",
			},
			"timeout_seconds": map[string]interface{}{
				"type":        "integer",
				"description": "Max seconds to wait for completion (default: 120)",
			},
		},
		"required": []string{"agent_id"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *CollectAgentTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	agentID, _ := params["agent_id"].(string)
	if agentID == "" {
		return &ToolResult{Success: false, Output: "agent_id is required", OutputFormat: FormatError}, nil
	}

	timeoutSec := 120
	if v, ok := params["timeout_seconds"].(float64); ok && v > 0 {
		timeoutSec = int(v)
	}

	spawner := BackgroundSpawnerFromContext(ctx)
	if spawner == nil {
		return &ToolResult{
			Success:      false,
			Output:       "background agent manager not available in this context",
			OutputFormat: FormatError,
		}, nil
	}

	result, err := spawner.CollectFromTool(ctx, agentID, timeoutSec)
	if err != nil {
		return &ToolResult{
			Success:      false,
			Output:       fmt.Sprintf("agent %s failed or timed out: %v", agentID, err),
			OutputFormat: FormatError,
		}, nil
	}

	return &ToolResult{
		Success:      true,
		Output:       fmt.Sprintf("=== Result from background agent %s ===\n\n%s", agentID, result),
		OutputFormat: FormatText,
	}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// ListAgentsTool
// ─────────────────────────────────────────────────────────────────────────────

// ListAgentsTool shows the status of all running/recently-completed background agents.
type ListAgentsTool struct {
	BaseTool
}

func NewListAgentsTool() *ListAgentsTool {
	return &ListAgentsTool{
		BaseTool: BaseTool{
			name:               "list_agents",
			description:        "List all background agents spawned in this session and their current status (pending/running/done/failed).",
			category:           CategoryAI,
			requiresPermission: false,
			defaultPermission:  PermissionAlways,
		},
	}
}

func (t *ListAgentsTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *ListAgentsTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	spawner := BackgroundSpawnerFromContext(ctx)
	if spawner == nil {
		return &ToolResult{
			Success:      true,
			Output:       "No background agent manager available (no agents spawned yet).",
			OutputFormat: FormatText,
		}, nil
	}

	agents := spawner.ListRunningFromTool()
	if len(agents) == 0 {
		return &ToolResult{
			Success:      true,
			Output:       "No background agents in this session.",
			OutputFormat: FormatText,
		}, nil
	}

	var sb fmt.Stringer
	_ = sb
	lines := fmt.Sprintf("%-12s %-20s %-12s %s\n", "ID", "Label", "Status", "Elapsed")
	lines += fmt.Sprintf("%s\n", "─────────────────────────────────────────────────────")
	for _, a := range agents {
		elapsed := a.Elapsed.Round(time.Second).String()
		lines += fmt.Sprintf("%-12s %-20s %-12s %s\n", a.ID, a.Label, a.Status, elapsed)
	}

	return &ToolResult{
		Success:      true,
		Output:       lines,
		OutputFormat: FormatText,
	}, nil
}
