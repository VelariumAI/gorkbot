// Package tui — analytics_view.go
//
// Analytics Dashboard Tab: a rich real-time dashboard showing session metrics,
// token usage, tool analytics, and model distribution.
//
// Activated by Ctrl+A (new "Analytics" tab in the tab bar).
// All widgets are rendered natively in Lip Gloss using the Sparkline, Gauge,
// and BarChart helpers from sparkline.go — inspired by termui's widget library
// (Sparkline, Gauge, BarChart, PieChart, Plot) but without any termbox dependency.
//
// ┌─────────────────────────────────────────────────────────────────┐
// │  SESSION ANALYTICS                                              │
// ├──────────────────────┬──────────────────────────────────────────┤
// │  Context Window      │  Token Generation Rate                   │
// │  [████████░░░] 72%   │  ⣠⣤⣶⣿⣿⣷⣶⣤⣄⣀                           │
// ├──────────────────────┴──────────────────────────────────────────┤
// │  Top Tools Used                                                 │
// │  bash         ████████████████████ 42                          │
// │  read_file    ██████████           22                          │
// ├─────────────────────────────────────────────────────────────────┤
// │  Session Stats                                                  │
// │  Messages: 14   Tokens: 48,291   Cost: $0.023   Turns: 6       │
// └─────────────────────────────────────────────────────────────────┘
package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// AnalyticsData holds the real-time metrics shown in the analytics dashboard.
// It is populated by the TUI model on every ToolProgressMsg and context sync tick.
type AnalyticsData struct {
	// Context window.
	ContextUsedPct  float64 // 0.0–1.0
	ContextUsedToks int
	ContextMaxToks  int

	// Token rate history (tokens per second, one entry per update tick).
	TokenRateHistory []float64

	// Tool usage counts by name.
	ToolCounts map[string]float64

	// Session stats.
	MessageCount  int
	TotalTokens   int
	SessionCostUS float64
	ToolTurnCount int

	// Model usage breakdown (model name → token count).
	ModelTokens map[string]int

	// Session start time (for elapsed).
	StartTime time.Time

	// Last token arrival time (for rate calculation).
	lastTokenTime  time.Time
	lastTokenCount int
}

// NewAnalyticsData initialises an empty AnalyticsData.
func NewAnalyticsData() *AnalyticsData {
	return &AnalyticsData{
		ToolCounts:  make(map[string]float64),
		ModelTokens: make(map[string]int),
		StartTime:   time.Now(),
	}
}

// RecordTokens updates token counts and calculates the token rate.
func (a *AnalyticsData) RecordTokens(newTotal int, costUSD float64) {
	now := time.Now()
	a.TotalTokens = newTotal
	a.SessionCostUS = costUSD

	if !a.lastTokenTime.IsZero() {
		elapsed := now.Sub(a.lastTokenTime).Seconds()
		if elapsed > 0 {
			delta := float64(newTotal - a.lastTokenCount)
			rate := delta / elapsed
			if rate < 0 {
				rate = 0
			}
			a.TokenRateHistory = append(a.TokenRateHistory, rate)
			// Keep last 60 samples.
			if len(a.TokenRateHistory) > 60 {
				a.TokenRateHistory = a.TokenRateHistory[len(a.TokenRateHistory)-60:]
			}
		}
	}
	a.lastTokenTime = now
	a.lastTokenCount = newTotal
}

// RecordToolUse increments the usage count for a tool.
func (a *AnalyticsData) RecordToolUse(toolName string) {
	if a.ToolCounts == nil {
		a.ToolCounts = make(map[string]float64)
	}
	a.ToolCounts[toolName]++
	a.ToolTurnCount++
}

// RecordMessage increments the message counter.
func (a *AnalyticsData) RecordMessage() {
	a.MessageCount++
}

// ─────────────────────────────────────────────────────────────────────────────
// Analytics view rendering
// ─────────────────────────────────────────────────────────────────────────────

// renderAnalyticsView renders the full analytics dashboard for the given
// terminal width and height.  Called from Model.View() when state == analyticsView.
func (m *Model) renderAnalyticsView() string {
	if m.analytics == nil {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			Padding(2, 4).
			Render("No analytics data yet. Start a conversation to see metrics.")
	}

	width := m.width
	if width < 40 {
		width = 40
	}

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(BorderGray)).
		Padding(0, 1)

	sectionTitle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(GrokBlue))

	halfWidth := (width - 6) / 2

	// ── Row 1: Context gauge + Token rate sparkline ────────────────────────

	// Context window gauge.
	ctxPct := m.analytics.ContextUsedPct
	gaugeColor := "#00D9FF"
	if ctxPct > 0.95 {
		gaugeColor = "#FF5555"
	} else if ctxPct > 0.80 {
		gaugeColor = "#FFB86C"
	}
	gaugeWidget := lipgloss.JoinVertical(lipgloss.Left,
		sectionTitle.Render("Context Window"),
		"",
		RenderGauge(ctxPct, halfWidth-2, gaugeColor, "#444444"),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).
			Render(fmt.Sprintf("%s / %s tokens",
				formatInt(m.analytics.ContextUsedToks),
				formatInt(m.analytics.ContextMaxToks))),
	)
	gaugeBox := borderStyle.Width(halfWidth).Render(gaugeWidget)

	// Token rate sparkline.
	sl := NewSparkline(halfWidth-4, 3)
	if len(m.analytics.TokenRateHistory) > 0 {
		sl.SetData(m.analytics.TokenRateHistory)
	}
	lastRate := sl.Last()
	rateStr := fmt.Sprintf("idle  |  %d total tokens", m.analytics.TotalTokens)
	if lastRate > 0 {
		rateStr = fmt.Sprintf("%.1f tok/s  |  %d total tokens", lastRate, m.analytics.TotalTokens)
	}
	sparkWidget := lipgloss.JoinVertical(lipgloss.Left,
		sectionTitle.Render("Token Generation Rate"),
		RenderSparklineWithAxes(sl, GrokBlue),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render(rateStr),
	)
	sparkBox := borderStyle.Width(halfWidth).Render(sparkWidget)

	row1 := lipgloss.JoinHorizontal(lipgloss.Top, gaugeBox, "  ", sparkBox)

	// ── Row 2: Top tools bar chart ─────────────────────────────────────────

	var toolEntries []BarChartEntry
	toolColors := []string{GrokBlue, GeminiPurple, "#50FA7B", "#FFB86C", "#FF79C6", "#8BE9FD", "#F1FA8C"}

	type kv struct {
		k string
		v float64
	}
	var sorted []kv
	for k, v := range m.analytics.ToolCounts {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].v != sorted[j].v {
			return sorted[i].v > sorted[j].v
		}
		return sorted[i].k < sorted[j].k // alphabetical tiebreak = stable color assignment
	})

	maxTools := 8
	for i, item := range sorted {
		if i >= maxTools {
			break
		}
		toolEntries = append(toolEntries, BarChartEntry{
			Label: item.k,
			Value: item.v,
			Color: toolColors[i%len(toolColors)],
		})
	}

	var toolContent string
	if len(toolEntries) > 0 {
		toolContent = RenderBarChart(toolEntries, width-8)
	} else {
		toolContent = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).
			Render("No tools used yet.")
	}

	toolWidget := lipgloss.JoinVertical(lipgloss.Left,
		sectionTitle.Render("Top Tools Used"),
		"",
		toolContent,
	)
	toolBox := borderStyle.Width(width - 4).Render(toolWidget)

	// ── Row 3: Session stats table ─────────────────────────────────────────

	elapsed := time.Since(m.analytics.StartTime).Round(time.Second)

	statStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#CCCCCC"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))

	stat := func(label, value string) string {
		return labelStyle.Render(label+": ") + statStyle.Render(value)
	}

	statsRow1 := lipgloss.JoinHorizontal(lipgloss.Top,
		stat("Messages", fmt.Sprintf("%d", m.analytics.MessageCount)),
		"   ",
		stat("Tokens", formatInt(m.analytics.TotalTokens)),
		"   ",
		stat("Cost", fmt.Sprintf("$%.4f", m.analytics.SessionCostUS)),
		"   ",
		stat("Tool Turns", fmt.Sprintf("%d", m.analytics.ToolTurnCount)),
		"   ",
		stat("Elapsed", elapsed.String()),
	)

	// Context warning row.
	var warningRow string
	if ctxPct >= 0.95 {
		warningRow = "\n" + lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5555")).
			Bold(true).
			Render("⚠  Context at 95%+ — compaction recommended (/compress)")
	} else if ctxPct >= 0.80 {
		warningRow = "\n" + lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFB86C")).
			Render("⚡ Context at 80%+ — approaching limit")
	}

	statsWidget := lipgloss.JoinVertical(lipgloss.Left,
		sectionTitle.Render("Session Statistics"),
		"",
		statsRow1,
		warningRow,
	)
	statsBox := borderStyle.Width(width - 4).Render(statsWidget)

	// ── Assemble ───────────────────────────────────────────────────────────

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(GrokBlue)).
		Width(width).
		Align(lipgloss.Center).
		Render("SESSION ANALYTICS")

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#555555")).
		Width(width).
		Align(lipgloss.Center).
		Render("Ctrl+H → Chat  |  Ctrl+T → Models  |  Ctrl+E → Tools  |  Ctrl+D → Discovery")

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		row1,
		"",
		toolBox,
		"",
		statsBox,
		"",
		help,
	)
}

// formatInt formats an integer with thousands separators.
func formatInt(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var result strings.Builder
	rem := len(s) % 3
	if rem > 0 {
		result.WriteString(s[:rem])
	}
	for i := rem; i < len(s); i += 3 {
		if i > 0 || rem > 0 {
			result.WriteByte(',')
		}
		result.WriteString(s[i : i+3])
	}
	return result.String()
}
