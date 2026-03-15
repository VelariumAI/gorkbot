package tools

// goals.go — Prospective Memory: Goal Ledger tools.
//
// Provides three tools for managing cross-session goals:
//   - add_goal    : add a new open goal
//   - close_goal  : mark a goal as done
//   - list_goals  : list all open goals

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/velariumai/gorkbot/pkg/security"
)

// goalLedgerKeyType is the context key type for the goal ledger.
type goalLedgerKeyType struct{}

// GoalLedgerKey is the context key used to inject a GoalLedger.
var GoalLedgerKey = goalLedgerKeyType{}

// GoalLedgerAccessor is the interface the goal tools use to access the ledger.
// The concrete implementation is *memory.GoalLedger.
type GoalLedgerAccessor interface {
	AddGoal(description string) string
	CloseGoal(id string) bool
	DeferGoal(id string) bool
	FormatBrief() string
}

// ── add_goal ──────────────────────────────────────────────────────────────────

// AddGoalTool adds a new goal to the cross-session ledger.
type AddGoalTool struct{ BaseTool }

func NewAddGoalTool() *AddGoalTool {
	return &AddGoalTool{
		BaseTool: NewBaseTool(
			"add_goal",
			"Add a new goal to the persistent cross-session goal ledger. Goals survive session restarts and are surfaced at the start of each new session. Use this to track work-in-progress tasks that may span multiple conversations.",
			CategoryMeta,
			false,
			PermissionAlways,
		),
	}
}

func (t *AddGoalTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"description":{"type":"string","description":"A clear, actionable description of the goal (e.g. 'Implement TF-IDF similarity for MEL deduplication')."}
		},
		"required":["description"]
	}`)
}

func (t *AddGoalTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	desc, _ := params["description"].(string)
	if desc == "" {
		return &ToolResult{Success: false, Error: "description is required"}, nil
	}
	accessor, _ := ctx.Value(GoalLedgerKey).(GoalLedgerAccessor)
	if accessor == nil {
		return &ToolResult{Success: false, Error: "goal ledger not available (not wired)"}, nil
	}
	id := accessor.AddGoal(desc)
	return &ToolResult{
		Success: true,
		Output:  fmt.Sprintf("Goal added [ID: %s]: %s", id, desc),
	}, nil
}

// ── close_goal ────────────────────────────────────────────────────────────────

// CloseGoalTool marks a goal as completed.
type CloseGoalTool struct{ BaseTool }

func NewCloseGoalTool() *CloseGoalTool {
	return &CloseGoalTool{
		BaseTool: NewBaseTool(
			"close_goal",
			"Mark a goal as completed in the cross-session goal ledger. The goal will no longer appear in session-start summaries.",
			CategoryMeta,
			false,
			PermissionAlways,
		),
	}
}

func (t *CloseGoalTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"id":{"type":"string","description":"The goal ID to mark as completed (get IDs from list_goals)."}
		},
		"required":["id"]
	}`)
}

func (t *CloseGoalTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	id, _ := params["id"].(string)
	if id == "" {
		return &ToolResult{Success: false, Error: "id is required"}, nil
	}
	if err := security.ValidateInput(id); err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("invalid id: %v", err)}, nil
	}
	accessor, _ := ctx.Value(GoalLedgerKey).(GoalLedgerAccessor)
	if accessor == nil {
		return &ToolResult{Success: false, Error: "goal ledger not available (not wired)"}, nil
	}
	if !accessor.CloseGoal(id) {
		return &ToolResult{Success: false, Error: fmt.Sprintf("goal %q not found", id)}, nil
	}
	return &ToolResult{
		Success: true,
		Output:  fmt.Sprintf("Goal %s marked as done.", id),
	}, nil
}

// ── list_goals ────────────────────────────────────────────────────────────────

// ListGoalsTool lists all open goals.
type ListGoalsTool struct{ BaseTool }

func NewListGoalsTool() *ListGoalsTool {
	return &ListGoalsTool{
		BaseTool: NewBaseTool(
			"list_goals",
			"List all open goals in the cross-session goal ledger. Returns goal IDs, descriptions, and how long ago they were created.",
			CategoryMeta,
			false,
			PermissionAlways,
		),
	}
}

func (t *ListGoalsTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"required":[]}`)
}

func (t *ListGoalsTool) Execute(ctx context.Context, _ map[string]interface{}) (*ToolResult, error) {
	accessor, _ := ctx.Value(GoalLedgerKey).(GoalLedgerAccessor)
	if accessor == nil {
		return &ToolResult{Success: false, Error: "goal ledger not available (not wired)"}, nil
	}
	brief := accessor.FormatBrief()
	if brief == "" {
		brief = "No open goals."
	}
	return &ToolResult{Success: true, Output: brief, OutputFormat: FormatText}, nil
}
