package tools

// introspection.go — Self-introspection tools for Gorkbot
//
// These tools allow the AI to query its own intelligence systems:
//   - gorkbot_status      : Verified date/time + build/embedder/provider status
//   - query_routing_stats : ARC Router statistics and last routing decision
//   - query_heuristics    : MEL VectorStore learned heuristics
//   - query_memory_state  : SENSE AgeMem + Engrams current state
//   - query_system_state  : Full diagnostic snapshot of all systems

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
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
	// GetRuntimeStatus returns build tags, embedder name, version, and live
	// provider/session info — facts the AI cannot know from training data.
	GetRuntimeStatus() string
	// GetAuditStats returns tool execution history from the persistent audit DB.
	// kind: "summary" (all-time table), "errors" (recent failures), "rate" (24h error rate).
	// filter is an optional tool name to scope the result.
	GetAuditStats(kind, filter string) string
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

// ── gorkbot_status ────────────────────────────────────────────────────────────

// GorkbotStatusTool is the authoritative source of truth for Gorkbot's runtime
// state. It always executes `date` to retrieve the verified system clock so the
// AI is grounded in real-world time, then combines build info, embedder status,
// provider state, and session stats from the live orchestrator.
//
// The AI MUST call this tool before making any claim about:
//   - the current date or time
//   - its own version, build, or capabilities
//   - which AI providers or models are active
//   - whether semantic embedding is running
type GorkbotStatusTool struct{ BaseTool }

func NewGorkbotStatusTool() *GorkbotStatusTool {
	return &GorkbotStatusTool{
		BaseTool: NewBaseTool(
			"gorkbot_status",
			"Get the authoritative, verified status of this Gorkbot instance. Always runs `date` to confirm the real system clock, then reports build variant (standard/llamacpp), semantic embedder state, active providers, MEL heuristic count, session info, and context usage. Call this tool BEFORE making any claim about the current date/time, your own version, capabilities, or which systems are online.",
			CategoryMeta,
			false,
			PermissionAlways,
		),
	}
}

func (t *GorkbotStatusTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"required":[]}`)
}

func (t *GorkbotStatusTool) Execute(ctx context.Context, _ map[string]interface{}) (*ToolResult, error) {
	var sb strings.Builder

	// 1. Always run `date` for a verified, tamper-proof timestamp.
	dateOut, err := exec.CommandContext(ctx, "date").Output()
	if err != nil {
		sb.WriteString(fmt.Sprintf("**System Clock**: unavailable (%v)\n\n", err))
	} else {
		sb.WriteString(fmt.Sprintf("**System Clock (verified)**: %s\n", strings.TrimSpace(string(dateOut))))
	}

	// 2. Runtime status from the orchestrator.
	rep, _ := ctx.Value(IntrospectionKey).(IntrospectionReporter)
	if rep == nil {
		sb.WriteString("\nRuntime status not available (introspection not wired).")
	} else {
		sb.WriteString(rep.GetRuntimeStatus())
	}

	return &ToolResult{Success: true, Output: sb.String(), OutputFormat: FormatText}, nil
}

// ── query_audit_log ───────────────────────────────────────────────────────────

// QueryAuditLogTool lets the AI inspect its own tool execution history from the
// persistent SQLite audit database (audit.db).
type QueryAuditLogTool struct{ BaseTool }

func NewQueryAuditLogTool() *QueryAuditLogTool {
	return &QueryAuditLogTool{
		BaseTool: NewBaseTool(
			"query_audit_log",
			"Query the persistent tool execution audit log (audit.db). "+
				"kind='summary' returns an all-time breakdown by tool (calls, success %, avg ms, top error category). "+
				"kind='errors' returns the most recent tool failures (optionally filtered by tool_name). "+
				"kind='rate' returns the error rate for the last 24 hours. "+
				"Use this to diagnose recurring failures, understand which tools you rely on most, and spot error patterns.",
			CategoryMeta,
			false,
			PermissionAlways,
		),
	}
}

func (t *QueryAuditLogTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"kind": {
				"type": "string",
				"description": "What to query: 'summary' (all-time per-tool stats), 'errors' (recent failures), or 'rate' (24-hour error rate). Defaults to 'summary'.",
				"enum": ["summary", "errors", "rate"]
			},
			"tool_name": {
				"type": "string",
				"description": "Optional: filter results to a specific tool name (only applies to kind='errors')."
			}
		},
		"required": []
	}`)
}

func (t *QueryAuditLogTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	rep, _ := ctx.Value(IntrospectionKey).(IntrospectionReporter)
	if rep == nil {
		return &ToolResult{Success: true, Output: "Audit log not available (introspection not wired)."}, nil
	}
	kind, _ := params["kind"].(string)
	if kind == "" {
		kind = "summary"
	}
	toolFilter, _ := params["tool_name"].(string)
	return &ToolResult{Success: true, Output: rep.GetAuditStats(kind, toolFilter), OutputFormat: FormatText}, nil
}
