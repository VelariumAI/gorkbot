// Package tui — side_panel.go
//
// Side panel rendered to the right of the chat viewport (permanently, adaptive width).
// Refreshes every 500 ms via sidePanelTick.
//
// Sections (top-to-bottom, only shown when non-empty):
//  1. AGENTS   — background agents currently running or pending
//  2. TOP TOOLS — top-5 tool usage bar chart
//  3. INTENT   — ARC intent badge for the latest user message
//  4. RALPH    — Ralph loop iteration counter (when active)
//  5. ACTIONS  — active hooks summary
package tui

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/velariumai/gorkbot/internal/engine"
	"github.com/velariumai/gorkbot/pkg/adaptive"
	"github.com/velariumai/gorkbot/pkg/selfimprove"
)

// ── Color list for TOP TOOLS bars (3-color rotation across ranks) ────────
var dashBarColors = []string{
	DashBarMagenta, DashBarPurple, DashBarTeal, DashBarTeal, DashBarTeal,
}

// ── renderSectionHeader renders "❖ LABEL ──────" with green styling
func renderSectionHeader(label string, innerW int) string {
	title := "❖ " + label
	titleStyled := lipgloss.NewStyle().
		Foreground(lipgloss.Color(DashMainGreen)).Bold(true).Render(title)
	ruleLen := innerW - lipgloss.Width(titleStyled) - 1
	if ruleLen < 1 {
		ruleLen = 1
	}
	rule := lipgloss.NewStyle().
		Foreground(lipgloss.Color(DashRuleCharcoal)).
		Render(strings.Repeat("─", ruleLen))
	return titleStyled + " " + rule
}

// ── (m *Model) renderDashAgents renders agent status with bullets & elapsed
func (m *Model) renderDashAgents(innerW int) string {
	if m.orchestrator == nil || m.orchestrator.BackgroundAgents == nil {
		return ""
	}
	agents := m.orchestrator.BackgroundAgents.Running()
	if len(agents) == 0 {
		return ""
	}

	lines := []string{renderSectionHeader("AGENTS", innerW)}
	for _, a := range agents {
		var bullet, status string
		if a.Status == engine.AgentRunning {
			// Check if elapsed < 3 seconds (very recent start)
			elapsed := time.Since(a.StartedAt)
			if elapsed < 3*time.Second {
				bullet = lipgloss.NewStyle().
					Foreground(lipgloss.Color(DashBulletYellow)).
					Render("⠧")
				status = " (active)"
			} else {
				bullet = lipgloss.NewStyle().
					Foreground(lipgloss.Color(DashBulletYellow)).
					Render("•")
				status = " (running)"
			}
		} else {
			// Pending/default
			bullet = lipgloss.NewStyle().
				Foreground(lipgloss.Color(DashBulletBlue)).
				Render("•")
			status = " (pending)"
		}

		label := truncate(a.Label, innerW-len(bullet)-len(status)-1)
		line := bullet + " " + label + status
		lines = append(lines, lipgloss.NewStyle().
			Foreground(lipgloss.Color(DashFgMuted)).
			Width(innerW).
			Render(line))
	}

	return strings.Join(lines, "\n")
}

// ── (m *Model) renderDashTopTools renders tool usage bar chart
func (m *Model) renderDashTopTools(innerW int) string {
	if m.analytics == nil || len(m.analytics.ToolCounts) == 0 {
		return ""
	}

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
		return sorted[i].k < sorted[j].k
	})

	top := sorted
	if len(top) > 5 {
		top = top[:5]
	}

	maxVal := top[0].v
	if maxVal == 0 {
		maxVal = 1
	}

	lines := []string{renderSectionHeader("TOP TOOLS", innerW)}
	for idx, item := range top {
		nameW := innerW / 3
		barAvail := innerW - nameW - 2 - 4 // 2 for ║, 4 for count
		if barAvail < 1 {
			barAvail = 1
		}
		filled := int((item.v / maxVal) * float64(barAvail))
		if filled > barAvail {
			filled = barAvail
		}

		barColor := dashBarColors[idx%len(dashBarColors)]
		bar := lipgloss.NewStyle().Background(lipgloss.Color(barColor)).
			Render(strings.Repeat(" ", filled))
		empty := strings.Repeat(" ", barAvail-filled)

		name := truncate(item.k, nameW)
		countStr := fmt.Sprintf("%.0f", item.v)
		line := fmt.Sprintf("%-*s ║ %s%s %s", nameW, name, bar, empty, countStr)
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// ── renderMiniBar renders a compact score bar: "███░░░ 0.57" (width = 6)
func renderMiniBar(score float64) string {
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}

	const width = 6
	filled := int(math.Round(score * float64(width)))
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)

	var barStyle lipgloss.Style
	switch {
	case score >= 0.65:
		barStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff6b6b")) // red
	case score >= 0.35:
		barStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffa500")) // orange
	default:
		barStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(DashMainGreen))
	}
	return barStyle.Render(bar) + " " + fmt.Sprintf("%.2f", score)
}

// ── renderSignalBreakdown renders S1-S5 signal rows with mini bars
func renderSignalBreakdown(sig selfimprove.SignalSnapshot) []string {
	// Compute S1-S5 scores
	s1 := sig.SPARKDriveScore
	s2 := 1.0
	if sig.ToolHealthWorst > 0 {
		s2 = 1.0 - minf(sig.ToolHealthWorst, 1.0)
	}
	s3 := 0.0
	if sig.SPARKActiveDirectives > 0 {
		s3 = minf(float64(sig.SPARKActiveDirectives)/20.0, 1.0)
	}
	s4 := 0.0
	if sig.HarnessTotal > 0 {
		s4 = float64(sig.HarnessFailing) / float64(sig.HarnessTotal)
	}
	s5 := 0.0
	if sig.FreeWillProposalsPending > 0 {
		s5 = minf(float64(sig.FreeWillProposalsPending)/5.0, 1.0)
	}

	signals := []struct {
		name    string
		score   float64
		context string
	}{
		{"SPARK", s1, fmt.Sprintf("dirs:%d", sig.SPARKActiveDirectives)},
		{"Health", s2, fmt.Sprintf("fail:%.0f%%", sig.ToolHealthWorst*100)},
		{"Dirs", s3, fmt.Sprintf("%d active", sig.SPARKActiveDirectives)},
		{"Harness", s4, fmt.Sprintf("%d/%d", sig.HarnessFailing, sig.HarnessTotal)},
		{"FreeWill", s5, fmt.Sprintf("%d props", sig.FreeWillProposalsPending)},
	}

	var lines []string
	for idx, sig := range signals {
		bar := renderMiniBar(sig.score)
		context := truncate(sig.context, 12)
		line := fmt.Sprintf("  S%d %s  %s", idx+1, bar, context)
		lines = append(lines, line)
	}
	return lines
}

// ── (m *Model) renderDashIntent renders the intent pill badge
func (m *Model) renderDashIntent(innerW int) string {
	if m.lastIntentCategory == "" {
		return ""
	}

	label := adaptive.CategoryLabel(adaptive.IntentCategory(m.lastIntentCategory))
	emoji := adaptive.CategoryEmoji(adaptive.IntentCategory(m.lastIntentCategory))

	pill := lipgloss.NewStyle().
		Foreground(lipgloss.Color(DashPillFg)).
		Background(lipgloss.Color(DashPillBg)).
		Bold(true).
		Padding(0, 1).
		Render(emoji + " " + label)

	return renderSectionHeader("INTENT", innerW) + "\n" + pill
}

// ── (m *Model) renderDashRalph renders the RALPH iteration counter
func (m *Model) renderDashRalph(innerW int) string {
	if m.orchestrator == nil || m.orchestrator.RalphLoop == nil {
		return ""
	}
	iter := m.orchestrator.RalphLoop.IterationsUsed()
	if iter == 0 {
		return ""
	}

	lines := []string{
		renderSectionHeader("RALPH", innerW),
		"  ↳ Iteration count: " + fmt.Sprintf("%d", iter) + " (Stabilizing...)",
		"  (SENSE engine refining output...)",
	}
	return strings.Join(lines, "\n")
}

// ── (m *Model) renderDashEvolve renders comprehensive autonomous evolution state
func (m *Model) renderDashEvolve(innerW int) string {
	snap := m.evolveSnapshot
	if !snap.Enabled {
		return ""
	}

	header := renderSectionHeader("EVOLVE", innerW)
	var lines []string
	lines = append(lines, header)

	// ── ACTIVE CYCLE MODE (IsRunning == true) ────────────────────────────────
	if snap.IsRunning {
		// Cycle ID with pulsing indicator
		cycleIDLine := fmt.Sprintf("  ◉ CYCLE #%d RUNNING", snap.CycleCount)
		cycleIDStyled := lipgloss.NewStyle().
			Foreground(lipgloss.Color(DashBulletYellow)).
			Render(cycleIDLine)
		lines = append(lines, cycleIDStyled)

		// Source → Target line (what we're working on)
		if snap.LastCandidate != nil {
			sourceName := snap.LastCandidate.Source.String()
			targetName := truncate(snap.LastCandidate.Target, innerW-len(sourceName)-8)
			sourceLine := fmt.Sprintf("  📡 %s → %s", sourceName, targetName)
			sourceStyled := lipgloss.NewStyle().Foreground(lipgloss.Color(DashFgMuted)).Render(sourceLine)
			lines = append(lines, sourceStyled)
		}

		// Phase line with detailed action
		phaseLine := "  ⚙  "
		phaseDetail := ""
		switch {
		case strings.HasPrefix(snap.ActivePhase, "selecting"):
			phaseLine += "⟳ Analyzing candidates"
			phaseDetail = "(evaluating best target)"
		case strings.HasPrefix(snap.ActivePhase, "executing:"):
			target := strings.TrimPrefix(snap.ActivePhase, "executing:")
			phaseLine += "▶ Executing: " + truncate(target, innerW-20)
			phaseDetail = "(in progress...)"
		case snap.ActivePhase == "verifying":
			phaseLine += "✦ Verifying outcome"
			phaseDetail = "(checking results)"
		default:
			phaseLine += "? Unknown phase"
		}
		phaseStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("#87ceeb")).Render(phaseLine)
		lines = append(lines, phaseStyled)
		if phaseDetail != "" {
			detailStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render("  " + phaseDetail)
			lines = append(lines, detailStyled)
		}

		// Divider
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color(DashRuleCharcoal)).Render(strings.Repeat("─", innerW)))

		// Detailed signal breakdown (S1-S5 with context)
		sigLines := renderSignalBreakdown(snap.Signals)
		for _, sigLine := range sigLines {
			styled := lipgloss.NewStyle().Foreground(lipgloss.Color(DashFgMuted)).Render(sigLine)
			lines = append(lines, styled)
		}

		// Extended signal details (tool health, harness, proposals)
		detailLines := renderDetailedSignals(snap.Signals)
		for _, detailLine := range detailLines {
			styled := lipgloss.NewStyle().Foreground(lipgloss.Color(DashFgMuted)).Render(detailLine)
			lines = append(lines, styled)
		}

	} else {
		// ── IDLE MODE (IsRunning == false) ────────────────────────────────
		// Cycle count & pending signal badges
		badgeLine := fmt.Sprintf("  #%d cycles completed  ⚡ %d signals", snap.CycleCount, snap.PendingSignals)
		badgeStyled := lipgloss.NewStyle().Foreground(lipgloss.Color(DashFgMuted)).Render(badgeLine)
		lines = append(lines, badgeStyled)

		// Divider
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color(DashRuleCharcoal)).Render(strings.Repeat("─", innerW)))

		// Detailed signal breakdown (S1-S5 with context)
		sigLines := renderSignalBreakdown(snap.Signals)
		for _, sigLine := range sigLines {
			styled := lipgloss.NewStyle().Foreground(lipgloss.Color(DashFgMuted)).Render(sigLine)
			lines = append(lines, styled)
		}

		// Extended signal details (tool health, harness, proposals)
		detailLines := renderDetailedSignals(snap.Signals)
		for _, detailLine := range detailLines {
			styled := lipgloss.NewStyle().Foreground(lipgloss.Color(DashFgMuted)).Render(detailLine)
			lines = append(lines, styled)
		}
	}

	// Divider
	lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color(DashRuleCharcoal)).Render(strings.Repeat("─", innerW)))

	// Drive score bar (shared between modes)
	scoreBar := renderDriveBar(snap.DriveScore, innerW-4)
	lines = append(lines, "  "+scoreBar)

	// Mode badge + reasoning + heartbeat countdown
	modeBadge := renderModeBadge(snap.Mode)
	modeReason := renderModeReasoning(snap.Mode, snap.RawScore)
	var hbStr string
	if !snap.NextHeartbeat.IsZero() {
		countdown := time.Until(snap.NextHeartbeat)
		if countdown < 0 {
			countdown = 0
		}
		hbStr = "↻ " + formatDuration(countdown)
	} else {
		hbStr = "↻ –"
	}
	modeHBLine := modeBadge + "  " + truncate(modeReason, 15)
	lines = append(lines, modeHBLine)

	hbLine := lipgloss.NewStyle().Foreground(lipgloss.Color(DashFgMuted)).Render("  next: " + hbStr)
	lines = append(lines, hbLine)

	// History divider + section
	lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color(DashRuleCharcoal)).Render(strings.Repeat("─", innerW)))
	histHeader := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#666666")).
		Render("  Last cycles:")
	lines = append(lines, histHeader)

	// Last 3 cycles (or fewer if not many)
	historyCount := len(snap.CycleHistory)
	if historyCount > 3 {
		historyCount = 3
	}

	if historyCount > 0 {
		// Show in reverse order (most recent first)
		for i := len(snap.CycleHistory) - 1; i >= 0 && len(snap.CycleHistory)-1-i < 3; i-- {
			cycle := snap.CycleHistory[i]
			var icon, color string
			switch cycle.Outcome {
			case "success":
				icon = "✓"
				color = DashMainGreen
			case "failed":
				icon = "✗"
				color = "#ff6b6b"
			default: // skipped, etc
				icon = "–"
				color = DashFgMuted
			}

			targetName := truncate(cycle.Target, innerW-15)
			durationStr := formatDuration(cycle.Duration)
			histLine := fmt.Sprintf("    %s %s (%s)", icon, targetName, durationStr)
			histStyled := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(histLine)
			lines = append(lines, histStyled)
		}
	} else {
		noHistLine := lipgloss.NewStyle().Foreground(lipgloss.Color(DashFgMuted)).Render("    (none yet)")
		lines = append(lines, noHistLine)
	}

	return strings.Join(lines, "\n")
}

// ── renderDriveBar renders a 12-block Unicode bar for drive score (0-1)
func renderDriveBar(score float64, maxWidth int) string {
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}

	blockCount := 12
	filledBlocks := int(score * float64(blockCount))
	bar := strings.Repeat("█", filledBlocks) + strings.Repeat("░", blockCount-filledBlocks)

	scoreColor := DashMainGreen
	if score > 0.65 {
		scoreColor = "#ff6b6b" // urgent red
	} else if score > 0.35 {
		scoreColor = "#ffa500" // orange/curious
	}

	barStyled := lipgloss.NewStyle().Foreground(lipgloss.Color(scoreColor)).Render(bar)
	return barStyled + " " + fmt.Sprintf("%.2f", score)
}

// ── renderModeBadge renders colored mode indicator
func renderModeBadge(mode selfimprove.EmotionalMode) string {
	modeStr := mode.String()
	var color string
	switch mode {
	case selfimprove.ModeCalm:
		color = "#808080" // gray
	case selfimprove.ModeCurious:
		color = "#87ceeb" // sky blue
	case selfimprove.ModeFocused:
		color = "#32cd32" // lime green
	case selfimprove.ModeUrgent:
		color = "#ff6b6b" // red
	case selfimprove.ModeRestrained:
		color = "#9370db" // purple
	default:
		color = DashFgMuted
	}
	badgeStyled := lipgloss.NewStyle().
		Foreground(lipgloss.Color(color)).
		Bold(true).
		Render("[" + modeStr + "]")
	return badgeStyled
}

// ── renderModeReasoning returns explanation for why SI is in current mode
func renderModeReasoning(mode selfimprove.EmotionalMode, rawScore float64) string {
	switch mode {
	case selfimprove.ModeCalm:
		return "low activity, idle"
	case selfimprove.ModeCurious:
		return "light exploration"
	case selfimprove.ModeFocused:
		return "normal operations"
	case selfimprove.ModeUrgent:
		return "high priority work"
	case selfimprove.ModeRestrained:
		return "paused by user"
	default:
		return "unknown state"
	}
}

// ── renderDetailedSignals renders extended signal info (tool health, harness status)
func renderDetailedSignals(sig selfimprove.SignalSnapshot) []string {
	var lines []string

	// Tool health section
	if sig.ToolHealthWorst > 0 {
		healthPct := int(sig.ToolHealthWorst * 100)
		healthLine := fmt.Sprintf("  🔧 Tool health: %d%% failure", healthPct)
		lines = append(lines, healthLine)
	}

	// Harness feature status
	if sig.HarnessTotal > 0 {
		failPct := int(float64(sig.HarnessFailing) / float64(sig.HarnessTotal) * 100)
		harnessLine := fmt.Sprintf("  📦 Features: %d/%d passing", sig.HarnessTotal-sig.HarnessFailing, sig.HarnessTotal)
		if failPct > 0 {
			harnessLine = fmt.Sprintf("  📦 Features: %d failing (%d%%)", sig.HarnessFailing, failPct)
		}
		lines = append(lines, harnessLine)
	}

	// SPARK directives
	if sig.SPARKActiveDirectives > 0 {
		dirLine := fmt.Sprintf("  ✦ SPARK: %d active directives", sig.SPARKActiveDirectives)
		lines = append(lines, dirLine)
	}

	// FreeWill proposals
	if sig.FreeWillProposalsPending > 0 {
		fwLine := fmt.Sprintf("  💭 FreeWill: %d proposals pending", sig.FreeWillProposalsPending)
		lines = append(lines, fwLine)
	}

	// Research documents
	if sig.ResearchBufferedDocs > 0 {
		resLine := fmt.Sprintf("  📚 Research: %d docs buffered", sig.ResearchBufferedDocs)
		lines = append(lines, resLine)
	}

	return lines
}

// ── (m *Model) renderDashActions renders active hook summary
func (m *Model) renderDashActions(innerW int) string {
	if len(m.activeHooks) == 0 {
		return ""
	}

	header := renderSectionHeader("ACTIONS", innerW)
	summary := RenderHookSummary(m.activeHooks, innerW, m.hookSpinFrame, 5, m.styles.Hook)
	if summary == "" {
		return ""
	}
	return header + "\n" + summary
}

// ── (m *Model) renderSidePanel assembles all sections into the sidebar
func (m *Model) renderSidePanel() string {
	w := m.sidePanelWidth
	h := m.viewport.Height
	if w < 4 {
		w = 4
	}
	if h < 4 {
		h = 4
	}
	innerW := w - 2
	if innerW < 4 {
		innerW = 4
	}

	var sections []string
	for _, s := range []string{
		m.renderDashAgents(innerW),
		m.renderDashTopTools(innerW),
		m.renderDashIntent(innerW),
		m.renderDashRalph(innerW),
		m.renderDashEvolve(innerW),
		m.renderDashActions(innerW),
	} {
		if s != "" {
			sections = append(sections, s)
		}
	}

	var body string
	if len(sections) == 0 {
		body = lipgloss.NewStyle().
			Foreground(lipgloss.Color(DashFgMuted)).Width(innerW).Render("(idle)")
	} else {
		body = strings.Join(sections, "\n\n")
	}

	// Use custom border to get ║ divider instead of │
	customBorder := lipgloss.Border{Left: "║"}
	return lipgloss.NewStyle().
		Border(customBorder, false, false, false, true).
		BorderForeground(lipgloss.Color(DashRuleCharcoal)).
		Padding(0, 1).
		Width(w).
		Height(h).
		Render(body)
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

// minf returns the minimum of two floats.
func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
