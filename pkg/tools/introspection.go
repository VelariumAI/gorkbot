package tools

// introspection.go — Self-introspection tools for Gorkbot
//
// These tools allow the AI to query its own intelligence systems:
//   - query_routing_stats : ARC Router statistics and last routing decision
//   - query_heuristics    : MEL VectorStore learned heuristics
//   - query_memory_state  : SENSE AgeMem + Engrams current state
//   - query_system_state  : Full diagnostic snapshot of all systems

import (
	"context"
	"encoding/json"
)

// introspectionKey is the unexported context key for IntrospectionReporter.
type introspectionKeyType struct{}

// IntrospectionKey is the context key used to inject an IntrospectionReporter.
var IntrospectionKey = introspectionKeyType{}

// IntrospectionReporter is implemented by the Orchestrator to surface
// its internal intelligence state to the tool layer.
type IntrospectionReporter interface {
	// GetRoutingStats returns a formatted report of ARC Router statistics.
	GetRoutingStats() string
	// GetHeuristics returns relevant MEL heuristics for the given query.
	// Pass "" or "general" to list all top heuristics.
	GetHeuristics(query string, k int) string
	// GetMemoryState returns a combined AgeMem + Engram summary for the query.
	GetMemoryState(query string) string
	// GetSystemState returns a full diagnostic snapshot of all subsystems.
	GetSystemState() string
}

// ── query_routing_stats ───────────────────────────────────────────────────────

// QueryRoutingStatsTool returns ARC Router statistics to the AI.
type QueryRoutingStatsTool struct{ BaseTool }

func NewQueryRoutingStatsTool() *QueryRoutingStatsTool {
	return &QueryRoutingStatsTool{
		BaseTool: NewBaseTool(
			"query_routing_stats",
			"Query the ARC Router's statistics: total requests routed, per-workflow-class breakdown, and the last routing decision with its resource budget. Use this to understand how prompts are being classified and to check for routing loops.",
			CategoryMeta,
			false,
			PermissionAlways,
		),
	}
}

func (t *QueryRoutingStatsTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"required":[]}`)
}

func (t *QueryRoutingStatsTool) Execute(ctx context.Context, _ map[string]interface{}) (*ToolResult, error) {
	rep, _ := ctx.Value(IntrospectionKey).(IntrospectionReporter)
	if rep == nil {
		return &ToolResult{Success: true, Output: "ARC Router stats not available (introspection not wired)."}, nil
	}
	return &ToolResult{Success: true, Output: rep.GetRoutingStats(), OutputFormat: FormatText}, nil
}

// ── query_heuristics ──────────────────────────────────────────────────────────

// QueryHeuristicsTool returns relevant MEL heuristics to the AI.
type QueryHeuristicsTool struct{ BaseTool }

func NewQueryHeuristicsTool() *QueryHeuristicsTool {
	return &QueryHeuristicsTool{
		BaseTool: NewBaseTool(
			"query_heuristics",
			"Query the MEL (Meta-Experience Learning) VectorStore for learned heuristics. Returns past failure lessons ranked by relevance to your query. Pass an empty query to list the top 20 global heuristics by confidence.",
			CategoryMeta,
			false,
			PermissionAlways,
		),
	}
}

func (t *QueryHeuristicsTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"query":{"type":"string","description":"Topic or keyword to search for relevant heuristics. Leave empty for top global heuristics."}
		},
		"required":[]
	}`)
}

func (t *QueryHeuristicsTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	rep, _ := ctx.Value(IntrospectionKey).(IntrospectionReporter)
	if rep == nil {
		return &ToolResult{Success: true, Output: "MEL heuristics not available (introspection not wired)."}, nil
	}
	query, _ := params["query"].(string)
	if query == "" {
		query = "general"
	}
	return &ToolResult{Success: true, Output: rep.GetHeuristics(query, 20), OutputFormat: FormatText}, nil
}

// ── query_memory_state ────────────────────────────────────────────────────────

// QueryMemoryStateTool returns SENSE AgeMem + Engram state to the AI.
type QueryMemoryStateTool struct{ BaseTool }

func NewQueryMemoryStateTool() *QueryMemoryStateTool {
	return &QueryMemoryStateTool{
		BaseTool: NewBaseTool(
			"query_memory_state",
			"Query the SENSE memory systems: AgeMem (short-term + long-term episodic memory) and EngramStore (persistent tool preferences). Returns a combined summary relevant to the given query topic.",
			CategoryMeta,
			false,
			PermissionAlways,
		),
	}
}

func (t *QueryMemoryStateTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"query":{"type":"string","description":"Topic or keyword to retrieve relevant memories. Leave empty for a general memory overview."}
		},
		"required":[]
	}`)
}

func (t *QueryMemoryStateTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	rep, _ := ctx.Value(IntrospectionKey).(IntrospectionReporter)
	if rep == nil {
		return &ToolResult{Success: true, Output: "Memory state not available (introspection not wired)."}, nil
	}
	query, _ := params["query"].(string)
	return &ToolResult{Success: true, Output: rep.GetMemoryState(query), OutputFormat: FormatText}, nil
}

// ── query_system_state ────────────────────────────────────────────────────────

// QuerySystemStateTool returns a full diagnostic snapshot.
type QuerySystemStateTool struct{ BaseTool }

func NewQuerySystemStateTool() *QuerySystemStateTool {
	return &QuerySystemStateTool{
		BaseTool: NewBaseTool(
			"query_system_state",
			"Get a full diagnostic snapshot of all Gorkbot subsystems: context window %, session cost, ARC Router stats, MEL heuristic count, SENSE AgeMem usage, active background agents, and top tool analytics. Use this for self-diagnosis or when something seems off.",
			CategoryMeta,
			false,
			PermissionAlways,
		),
	}
}

func (t *QuerySystemStateTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"required":[]}`)
}

func (t *QuerySystemStateTool) Execute(ctx context.Context, _ map[string]interface{}) (*ToolResult, error) {
	rep, _ := ctx.Value(IntrospectionKey).(IntrospectionReporter)
	if rep == nil {
		return &ToolResult{Success: true, Output: "System state not available (introspection not wired)."}, nil
	}
	return &ToolResult{Success: true, Output: rep.GetSystemState(), OutputFormat: FormatText}, nil
}
