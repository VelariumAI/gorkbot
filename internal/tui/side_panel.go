// Package tui — side_panel.go
//
// Side panel rendered to the right of the chat viewport when m.sidePanelOpen.
// Toggle with Ctrl+|. Refreshes every 500 ms via sidePanelTick.
//
// Sections (top-to-bottom, only shown when non-empty):
//  1. AGENTS   — background agents currently running or pending
//  2. TOP TOOLS — top-5 tool usage bar chart
//  3. INTENT   — ARC intent badge for the latest user message
//  4. RALPH    — Ralph loop iteration counter (when active)
package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/velariumai/gorkbot/internal/engine"
	"github.com/velariumai/gorkbot/pkg/adaptive"
)

// renderSidePanel renders the right-side info panel.
func (m *Model) renderSidePanel() string {
	w := m.sidePanelWidth
	h := m.viewport.Height
	if w < 4 {
		w = 4
	}
	if h < 4 {
		h = 4
	}

	innerW := w - 2 // subtract left border + 1 padding
	if innerW < 2 {
		innerW = 2
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(GrokBlue)).
		Width(innerW)

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(TextGray))

	var sections []string

	// ── AGENTS ──────────────────────────────────────────────────────────────
	if m.orchestrator != nil && m.orchestrator.BackgroundAgents != nil {
		agents := m.orchestrator.BackgroundAgents.Running()
		if len(agents) > 0 {
			lines := []string{titleStyle.Render("AGENTS")}
			for _, a := range agents {
				statusColor := GrokBlue
				if a.Status == engine.AgentRunning {
					statusColor = SuccessGreen
				}
				line := lipgloss.NewStyle().
					Foreground(lipgloss.Color(statusColor)).
					Width(innerW).
					Render(fmt.Sprintf("● %s", truncate(a.Label, innerW-2)))
				lines = append(lines, line)
			}
			sections = append(sections, strings.Join(lines, "\n"))
		}
	}

	// ── TOP TOOLS ────────────────────────────────────────────────────────────
	if m.analytics != nil && len(m.analytics.ToolCounts) > 0 {
		type kv struct {
			k string
			v float64
		}
		var sorted []kv
		for k, v := range m.analytics.ToolCounts {
			sorted = append(sorted, kv{k, v})
		}
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].v > sorted[j].v })

		top := sorted
		if len(top) > 5 {
			top = top[:5]
		}

		maxVal := top[0].v
		if maxVal == 0 {
			maxVal = 1
		}

		lines := []string{titleStyle.Render("TOP TOOLS")}
		for _, item := range top {
			labelW := innerW / 2
			barWidth := innerW - labelW - 5
			if barWidth < 1 {
				barWidth = 1
			}
			filled := int((item.v / maxVal) * float64(barWidth))
			bar := lipgloss.NewStyle().Foreground(lipgloss.Color(GrokBlue)).
				Render(strings.Repeat("█", filled))
			line := fmt.Sprintf("%-*s %s %.0f",
				labelW, truncate(item.k, labelW),
				bar, item.v)
			lines = append(lines, dimStyle.Render(line))
		}
		sections = append(sections, strings.Join(lines, "\n"))
	}

	// ── INTENT ───────────────────────────────────────────────────────────────
	if m.lastIntentCategory != "" {
		label := adaptive.CategoryLabel(adaptive.IntentCategory(m.lastIntentCategory))
		emoji := adaptive.CategoryEmoji(adaptive.IntentCategory(m.lastIntentCategory))
		badge := lipgloss.NewStyle().
			Foreground(lipgloss.Color(TextGray)).
			Background(lipgloss.Color(BgDarkAlt)).
			Padding(0, 1).
			Width(innerW).
			Render(emoji + " " + label)
		sections = append(sections, titleStyle.Render("INTENT")+"\n"+badge)
	}

	// ── HOOKS ────────────────────────────────────────────────────────────────
	if len(m.activeHooks) > 0 {
		hookSummary := RenderHookSummary(m.activeHooks, innerW, m.hookSpinFrame, 5, m.styles.Hook)
		if hookSummary != "" {
			sections = append(sections, titleStyle.Render("ACTIONS")+"\n"+hookSummary)
		}
	}

	// ── RALPH ────────────────────────────────────────────────────────────────
	if m.orchestrator != nil && m.orchestrator.RalphLoop != nil {
		if iter := m.orchestrator.RalphLoop.IterationsUsed(); iter > 0 {
			ralphLine := lipgloss.NewStyle().
				Foreground(lipgloss.Color(WarningYellow)).
				Width(innerW).
				Render(fmt.Sprintf("↩ Ralph iter: %d", iter))
			sections = append(sections, titleStyle.Render("RALPH")+"\n"+ralphLine)
		}
	}

	// Assemble body
	var body string
	if len(sections) == 0 {
		body = dimStyle.Width(innerW).Render("(no data)")
	} else {
		body = strings.Join(sections, "\n\n")
	}

	// Left border only
	panelStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(lipgloss.Color(BorderGray)).
		Padding(0, 1).
		Width(w).
		Height(h)

	return panelStyle.Render(body)
}

// truncate shortens s to max runes, appending "…" if needed.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return "…"
	}
	return string(runes[:maxLen-1]) + "…"
}
