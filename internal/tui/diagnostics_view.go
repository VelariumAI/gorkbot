package tui

// diagnostics_view.go — System Diagnostics Tab
//
// Activated by Ctrl+\ — shows a 4-panel read-only diagnostic dashboard:
//
// ┌─────────────────────────────────────────────────────────────┐
// │  SYSTEM DIAGNOSTICS                                         │
// ├──────────────────────────┬──────────────────────────────────┤
// │  ARC Router Stats        │  MEL Heuristics                  │
// │  Total: 42               │  Stored: 17                      │
// │  Conversational: 8       │  Top: "using bash tool..."       │
// │  Factual: 12             │                                  │
// │  Analytical: 9           │                                  │
// │  Agentic: 8              │                                  │
// │  Creative: 3             │                                  │
// │  SecurityCritical: 2     │                                  │
// ├──────────────────────────┼──────────────────────────────────┤
// │  SENSE Memory            │  Session Health                  │
// │  AgeMem STM: 23 entries  │  Context: 34% (44k/131k)        │
// │  AgeMem LTM: 8 entries   │  HITL: ENABLED                  │
// │  Engrams: 5 stored       │  Goals: 2 open                  │
// └──────────────────────────┴──────────────────────────────────┘

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderDiagnosticsView renders the full system diagnostics tab.
func (m *Model) renderDiagnosticsView() string {
	if m.orchestrator == nil {
		return lipgloss.NewStyle().Padding(2).Render("Diagnostics not available (orchestrator not wired).")
	}

	w := m.width
	if w < 40 {
		w = 40
	}
	halfW := w/2 - 4

	// ── Panel styles ─────────────────────────────────────────────────────────
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("213")).
		Padding(0, 1)

	panelStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(halfW)

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("243"))

	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Bold(true)

	warnStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("208")).
		Bold(true)

	goodStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("82")).
		Bold(true)

	// Helper: render a key-value row
	kv := func(label, value string) string {
		return labelStyle.Render(fmt.Sprintf("%-22s", label)) + valueStyle.Render(value)
	}

	// ── Panel 1: ARC Router Stats ─────────────────────────────────────────────
	var arcLines []string
	orch := m.orchestrator
	if orch.Intelligence != nil {
		stats := orch.Intelligence.Router.Stats()
		arcLines = append(arcLines,
			kv("Total routed:", fmt.Sprintf("%d", stats.TotalRouted)),
		)
		classNames := []string{"Conversational", "Factual", "Analytical", "Agentic", "Creative", "Security"}
		for i, count := range stats.CountByClass {
			if i < len(classNames) {
				arcLines = append(arcLines, kv(classNames[i]+":", fmt.Sprintf("%d", count)))
			}
		}
		if last := orch.Intelligence.Router.LastDecision(); last != nil {
			arcLines = append(arcLines, "")
			arcLines = append(arcLines, labelStyle.Render("Last: ")+valueStyle.Render(last.Classification.String()))
			arcLines = append(arcLines, kv("  MaxTools:", fmt.Sprintf("%d", last.Budget.MaxToolCalls)))
		}
	} else {
		arcLines = append(arcLines, warnStyle.Render("Not initialized"))
	}
	panel1 := panelStyle.Render(
		headerStyle.Render("ARC Router") + "\n" +
			strings.Join(arcLines, "\n"),
	)

	// ── Panel 2: MEL Heuristics ───────────────────────────────────────────────
	var melLines []string
	if orch.Intelligence != nil && orch.Intelligence.Store != nil {
		total := orch.Intelligence.Store.Len()
		melLines = append(melLines, kv("Stored:", fmt.Sprintf("%d / 500", total)))
		top := orch.Intelligence.Store.Query("general", 3)
		if len(top) > 0 {
			melLines = append(melLines, "")
			melLines = append(melLines, labelStyle.Render("Top heuristics:"))
			for i, h := range top {
				text := h.Text()
				if len(text) > halfW-5 {
					text = text[:halfW-8] + "..."
				}
				melLines = append(melLines, fmt.Sprintf("  %d. %s", i+1, text))
			}
		} else {
			melLines = append(melLines, labelStyle.Render("No heuristics yet"))
		}
	} else {
		melLines = append(melLines, warnStyle.Render("Not initialized"))
	}
	panel2 := panelStyle.Render(
		headerStyle.Render("MEL Heuristics") + "\n" +
			strings.Join(melLines, "\n"),
	)

	// ── Panel 3: SENSE Memory ─────────────────────────────────────────────────
	var senseLines []string
	if orch.AgeMem != nil {
		stats := orch.AgeMem.UsageStats()
		if v, ok := stats["stm_count"]; ok {
			senseLines = append(senseLines, kv("AgeMem STM:", fmt.Sprintf("%v entries", v)))
		}
		if v, ok := stats["ltm_count"]; ok {
			senseLines = append(senseLines, kv("AgeMem LTM:", fmt.Sprintf("%v entries", v)))
		}
	} else {
		senseLines = append(senseLines, warnStyle.Render("AgeMem not initialized"))
	}
	if orch.GoalLedger != nil {
		open := orch.GoalLedger.OpenGoals()
		senseLines = append(senseLines, kv("Open Goals:", fmt.Sprintf("%d", len(open))))
	}
	if orch.UnifiedMem != nil {
		senseLines = append(senseLines, goodStyle.Render("UnifiedMem: active"))
	}
	panel3 := panelStyle.Render(
		headerStyle.Render("SENSE Memory") + "\n" +
			strings.Join(senseLines, "\n"),
	)

	// ── Panel 4: Session Health ────────────────────────────────────────────────
	var healthLines []string

	// Context window
	if orch.ContextMgr != nil {
		used := orch.ContextMgr.TokensUsed()
		limit := orch.ContextMgr.TokenLimit()
		pct := 0.0
		if limit > 0 {
			pct = float64(used) / float64(limit) * 100
		}
		pctStr := fmt.Sprintf("%.1f%% (%d/%d tokens)", pct, used, limit)
		style := goodStyle
		if pct > 80 {
			style = warnStyle
		}
		healthLines = append(healthLines, kv("Context:", style.Render(pctStr)))
	}

	// HITL guard
	hitlStr := warnStyle.Render("DISABLED")
	if orch.HITLGuard != nil && orch.HITLGuard.Enabled {
		hitlStr = goodStyle.Render("ENABLED")
	}
	healthLines = append(healthLines, kv("HITL Guard:", hitlStr))

	// Background agents
	if orch.BackgroundAgents != nil {
		all := orch.BackgroundAgents.List()
		running := orch.BackgroundAgents.Running()
		healthLines = append(healthLines, kv("Agents:", fmt.Sprintf("%d running / %d total", len(running), len(all))))
	}

	// Top tool
	if orch.Registry != nil && orch.Registry.GetAnalytics() != nil {
		top := orch.Registry.GetAnalytics().GetTopTools(1)
		if len(top) > 0 {
			rate := 0.0
			if top[0].ExecutionCount > 0 {
				rate = float64(top[0].SuccessCount) / float64(top[0].ExecutionCount) * 100
			}
			healthLines = append(healthLines, kv("Top tool:",
				fmt.Sprintf("%s (%d calls, %.0f%%)", top[0].ToolName, top[0].ExecutionCount, rate)))
		}
	}

	panel4 := panelStyle.Render(
		headerStyle.Render("Session Health") + "\n" +
			strings.Join(healthLines, "\n"),
	)

	// ── Layout: 2×2 grid ──────────────────────────────────────────────────────
	row1 := lipgloss.JoinHorizontal(lipgloss.Top, panel1, "  ", panel2)
	row2 := lipgloss.JoinHorizontal(lipgloss.Top, panel3, "  ", panel4)

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("213")).
		Padding(0, 0, 1, 1).
		Render("⚙ SYSTEM DIAGNOSTICS  [Ctrl+\\ to close]")

	return lipgloss.NewStyle().Padding(1).Render(
		title + "\n" + row1 + "\n\n" + row2,
	)
}
