package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// contextStatsKeyType is an unexported context key type for the stats reporter.
type contextStatsKeyType struct{}

// ContextStatsKey is the context key for injecting a ContextStatsReporter.
var ContextStatsKey = contextStatsKeyType{}

// ContextStatsReporter is implemented by the orchestrator's ContextManager.
type ContextStatsReporter interface {
	StatusBar() string
	ContextBreakdown(systemT, convT, toolT, extraT int) string
	TokensUsed() int
	TokenLimit() int
	CostReport(primaryModel, consultantModel string) string
}

// ContextStatsTool lets the AI query its own context window usage.
type ContextStatsTool struct {
	BaseTool
}

func NewContextStatsTool() *ContextStatsTool {
	return &ContextStatsTool{
		BaseTool: NewBaseTool(
			"context_stats",
			"Get current context window usage: tokens consumed, percentage full, estimated cost, and a breakdown by category (system/conversation/tools). Use this to gauge how close you are to the context limit.",
			CategoryMeta,
			false,
			PermissionAlways,
		),
	}
}

func (t *ContextStatsTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"required":[]}`)
}

func (t *ContextStatsTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	reporter, ok := ctx.Value(ContextStatsKey).(ContextStatsReporter)
	if !ok || reporter == nil {
		return &ToolResult{
			Success: true,
			Output:  "Context stats not available (orchestrator not wired).",
		}, nil
	}

	used := reporter.TokensUsed()
	limit := reporter.TokenLimit()
	pct := 0.0
	if limit > 0 {
		pct = float64(used) / float64(limit) * 100
	}

	output := fmt.Sprintf("## Context Window Status\n\n"+
		"- **Tokens used**: %d / %d (%.1f%%)\n"+
		"- **Tokens remaining**: %d\n\n"+
		"%s",
		used, limit, pct,
		limit-used,
		reporter.StatusBar(),
	)

	return &ToolResult{
		Success:      true,
		Output:       output,
		OutputFormat: FormatText,
	}, nil
}
