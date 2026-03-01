package engine

// introspection.go — Implements tools.IntrospectionReporter on the Orchestrator.

import (
	"fmt"
	"strings"
)

// GetRoutingStats implements tools.IntrospectionReporter.
func (o *Orchestrator) GetRoutingStats() string {
	if o.Intelligence == nil {
		return "ARC Router not initialized."
	}
	stats := o.Intelligence.Router.Stats()
	last := o.Intelligence.Router.LastDecision()

	var sb strings.Builder
	sb.WriteString("## ARC Router Statistics\n\n")
	sb.WriteString(fmt.Sprintf("**Total routed**: %d\n", stats.TotalRouted))
	sb.WriteString(fmt.Sprintf("**Platform**: %s\n\n", o.Intelligence.Router.PlatformName()))
	sb.WriteString("**Per-class breakdown**:\n")
	classNames := []string{"Conversational", "Factual", "Analytical", "Agentic", "Creative", "SecurityCritical"}
	for i, count := range stats.CountByClass {
		if i < len(classNames) {
			sb.WriteString(fmt.Sprintf("  - %s: %d\n", classNames[i], count))
		}
	}
	if last != nil {
		sb.WriteString(fmt.Sprintf("\n**Last decision** (%s):\n", last.Timestamp.Format("15:04:05")))
		sb.WriteString(fmt.Sprintf("  - Workflow: %s\n", last.Classification.String()))
		sb.WriteString(fmt.Sprintf("  - CostTier: %d\n", last.Budget.CostTier))
		sb.WriteString(fmt.Sprintf("  - MaxToolCalls: %d\n", last.Budget.MaxToolCalls))
		sb.WriteString(fmt.Sprintf("  - MaxTokens: %d\n", last.Budget.MaxTokens))
		sb.WriteString(fmt.Sprintf("  - Temperature: %.2f\n", last.Budget.Temperature))
	}
	return sb.String()
}

// GetHeuristics implements tools.IntrospectionReporter.
func (o *Orchestrator) GetHeuristics(query string, k int) string {
	if o.Intelligence == nil || o.Intelligence.Store == nil {
		return "MEL VectorStore not initialized."
	}
	total := o.Intelligence.Store.Len()
	heuristics := o.Intelligence.Store.Query(query, k)
	if len(heuristics) == 0 {
		return fmt.Sprintf("No heuristics found (total stored: %d).", total)
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## MEL Heuristics (query: %q, total stored: %d)\n\n", query, total))
	for i, h := range heuristics {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, h.Text()))
		sb.WriteString(fmt.Sprintf("   *Confidence: %.0f%% | Used: %d times*\n\n", h.Confidence*100, h.UseCount))
	}
	return sb.String()
}

// GetMemoryState implements tools.IntrospectionReporter.
func (o *Orchestrator) GetMemoryState(query string) string {
	var sb strings.Builder
	sb.WriteString("## SENSE Memory State\n\n")

	if o.AgeMem != nil {
		stats := o.AgeMem.UsageStats()
		sb.WriteString("### AgeMem\n")
		for k, v := range stats {
			sb.WriteString(fmt.Sprintf("- %s: %v\n", k, v))
		}
		if query != "" {
			relevant := o.AgeMem.FormatRelevant(query, 1000)
			if relevant != "" {
				sb.WriteString("\n**Relevant memories:**\n")
				sb.WriteString(relevant)
			}
		}
	} else {
		sb.WriteString("AgeMem not initialized.\n")
	}

	sb.WriteString("\n### Engrams\n")
	if o.Engrams != nil && query != "" {
		engCtx := o.Engrams.FormatAsContext(query)
		if engCtx != "" {
			sb.WriteString(engCtx)
		} else {
			sb.WriteString("No relevant engrams for this query.\n")
		}
	} else if o.Engrams == nil {
		sb.WriteString("EngramStore not initialized.\n")
	} else {
		sb.WriteString("(provide a query to surface relevant engrams)\n")
	}

	return sb.String()
}

// GetSystemState implements tools.IntrospectionReporter.
func (o *Orchestrator) GetSystemState() string {
	var sb strings.Builder
	sb.WriteString("## Gorkbot System Diagnostic\n\n")

	// Context window
	if o.ContextMgr != nil {
		used := o.ContextMgr.TokensUsed()
		limit := o.ContextMgr.TokenLimit()
		pct := 0.0
		if limit > 0 {
			pct = float64(used) / float64(limit) * 100
		}
		sb.WriteString(fmt.Sprintf("**Context Window**: %d / %d tokens (%.1f%%)\n", used, limit, pct))
	}

	// ARC Router
	if o.Intelligence != nil {
		stats := o.Intelligence.Router.Stats()
		sb.WriteString(fmt.Sprintf("**ARC Router**: %d routed | Direct: %d | Complex: %d\n",
			stats.TotalRouted, stats.DirectCount, stats.ReasonVerifyCount))
		sb.WriteString(fmt.Sprintf("**MEL Heuristics**: %d stored\n", o.Intelligence.Store.Len()))
	} else {
		sb.WriteString("**Intelligence Layer**: not initialized\n")
	}

	// AgeMem
	if o.AgeMem != nil {
		stats := o.AgeMem.UsageStats()
		if stm, ok := stats["stm_count"]; ok {
			sb.WriteString(fmt.Sprintf("**AgeMem STM**: %v entries\n", stm))
		}
		if ltm, ok := stats["ltm_count"]; ok {
			sb.WriteString(fmt.Sprintf("**AgeMem LTM**: %v entries\n", ltm))
		}
	} else {
		sb.WriteString("**AgeMem**: not initialized\n")
	}

	// Background agents
	if o.BackgroundAgents != nil {
		all := o.BackgroundAgents.List()
		running := o.BackgroundAgents.Running()
		sb.WriteString(fmt.Sprintf("**Background Agents**: %d running / %d total\n", len(running), len(all)))
	}

	// Goal ledger
	if o.GoalLedger != nil {
		open := o.GoalLedger.OpenGoals()
		sb.WriteString(fmt.Sprintf("**Open Goals**: %d\n", len(open)))
	}

	// Tool analytics top 5
	if o.Registry != nil && o.Registry.GetAnalytics() != nil {
		analytics := o.Registry.GetAnalytics()
		top := analytics.GetTopTools(5)
		if len(top) > 0 {
			sb.WriteString("\n**Top Tools (by execution count)**:\n")
			for _, t := range top {
				rate := 0.0
				if t.ExecutionCount > 0 {
					rate = float64(t.SuccessCount) / float64(t.ExecutionCount) * 100
				}
				sb.WriteString(fmt.Sprintf("  - %s: %d calls (%.0f%% success)\n", t.ToolName, t.ExecutionCount, rate))
			}
		}
	}

	// HITL status
	hitlStatus := "DISABLED"
	if o.HITLGuard != nil && o.HITLGuard.Enabled {
		hitlStatus = "ENABLED"
	}
	sb.WriteString(fmt.Sprintf("**HITL Guard**: %s\n", hitlStatus))

	return sb.String()
}
