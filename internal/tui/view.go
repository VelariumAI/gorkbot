package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// View renders the entire TUI
func (m *Model) View() string {
	if m.quitting {
		return m.styles.Help.Render("Thanks for using Gorkbot! 👋\n")
	}

	// Show animated splash screen until the user presses Enter.
	if !m.splashDone {
		return m.renderSplashScreen()
	}

	if !m.ready {
		return "Initializing..."
	}

	// 1. Tab Bar is now the topmost element (header moved to splash).
	var sections []string

	// 2. Tab Bar
	tabs := m.renderTabs()
	sections = append(sections, tabs)

	// 2b. Notification zone (toast, 1 line, zero height when empty)
	if zone := m.renderNotificationZone(); zone != "" {
		sections = append(sections, zone)
	}

	// 3. Main Content Area (dynamic based on active tab)
	var content string
	switch m.state {
	case modelSelectView: // also handles modelListView (same const)
		content = m.renderModelSelectView()
	case toolsTableView:
		content = m.toolsTable.View()
	case discoveryView:
		content = m.renderDiscoveryView()
	case analyticsView:
		content = m.renderAnalyticsView()
	case diagnosticsView:
		content = m.renderDiagnosticsView()
	case dagView:
		content = m.renderDAGView()
	default:
		content = m.renderChatView()
	}

	// Ensure content fits in remaining height
	// Header + Tabs + StatusBar are fixed
	// We need to calculate remaining height for content area
	// Note: Viewport and Lists handle their own height, but we need to ensure the container fits
	sections = append(sections, content)

	// 4. Status Bar (if not in full-screen list modes that have their own)
	// Actually, let's show status bar everywhere for consistency
	statusBar := m.renderStatusBar()
	sections = append(sections, statusBar)

	// Join all sections
	view := lipgloss.JoinVertical(lipgloss.Left, sections...)

	// OVERLAYS

	// API Key Prompt (model select overlay)
	if m.apiKeyPrompt.active {
		view = m.renderAPIKeyPrompt()
	}

	// Permission Prompt
	if m.awaitingPermission && m.permissionPrompt != nil {
		permissionBox := m.permissionPrompt.Render(m.width - 8)
		view = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, permissionBox,
			lipgloss.WithWhitespaceChars("░"), lipgloss.WithWhitespaceForeground(lipgloss.Color("235")))
	}

	// HITL Approval Prompt — displayed as a modal overlay over the chat view.
	if m.awaitingHITL && m.hitlRequest != nil {
		hitlBox := m.renderHITLPrompt()
		view = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, hitlBox,
			lipgloss.WithWhitespaceChars("░"), lipgloss.WithWhitespaceForeground(lipgloss.Color("235")))
	}

	// Dynamic Auth Wizard Prompt
	if m.awaitingAuth && m.authRequest != nil {
		authBox := m.renderAuthWizard()
		view = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, authBox,
			lipgloss.WithWhitespaceChars("░"), lipgloss.WithWhitespaceForeground(lipgloss.Color("235")))
	}

	// Intervention Prompt
	if m.interventionMode {
		boxStyle := m.styles.ToolBox.Copy().
			BorderForeground(m.styles.Error.GetForeground()).
			Width(60).
			Padding(1, 2)
		titleStyle := m.styles.Error.Copy().Bold(true).Underline(true)
		content := fmt.Sprintf("%s\n\n", titleStyle.Render("⚠️  WATCHDOG ALERT  ⚠️"))
		// ... (abbreviated for brevity, reusing existing logic)
		content += "System detected potential loop.\n[C]ontinue  [S]ession Allow  [K]ill"
		promptBox := boxStyle.Render(content)
		view = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, promptBox,
			lipgloss.WithWhitespaceChars("▒"), lipgloss.WithWhitespaceForeground(lipgloss.Color("52")))
	}

	// Rewind Menu overlay (double-Esc)
	if m.rewindMenuOpen {
		view = m.renderRewindMenu()
	}

	// Bookmark Manager overlay
	if m.bookmarkOverlay {
		view = m.renderBookmarkOverlay()
	}

	// Active Overlay (e.g. Process Manager)
	if m.activeOverlay != nil {
		overlayContent := m.activeOverlay.View()
		view = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlayContent,
			lipgloss.WithWhitespaceChars("░"), lipgloss.WithWhitespaceForeground(lipgloss.Color("235")))
	}

	return view
}

// renderHITLPrompt renders the HITL plan-and-execute approval overlay.
func (m *Model) renderHITLPrompt() string {
	req := m.hitlRequest
	if req == nil {
		return ""
	}

	boxWidth := m.width - 4
	if boxWidth > 82 {
		boxWidth = 82
	}

	boxStyle := m.styles.HITL.Copy().Width(boxWidth)

	// Derive title/tool accent from border foreground of HITL style.
	accentColor := m.styles.HITL.GetBorderTopForeground()

	titleStyle := lipgloss.NewStyle().
		Foreground(accentColor).
		Bold(true)

	toolStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(TextWhite)).
		Bold(true)

	hintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(TextGray)).
		Italic(true)

	approveStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#00FF88")).
		Bold(true)

	rejectStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(ErrorRed)).
		Bold(true)

	// Truncate plan if it would overflow the box.
	plan := req.Plan
	lines := strings.Split(plan, "\n")
	const maxPlanLines = 14
	if len(lines) > maxPlanLines {
		lines = append(lines[:maxPlanLines], hintStyle.Render("  … (truncated)"))
		plan = strings.Join(lines, "\n")
	}

	queueNote := ""
	if len(m.hitlQueue) > 1 {
		queueNote = "\n" + hintStyle.Render(fmt.Sprintf("(%d more requests queued)", len(m.hitlQueue)-1))
	}

	content := titleStyle.Render("⚡ SENSE HITL — Action Requires Approval") + "\n\n"
	content += "Tool: " + toolStyle.Render(req.ToolName) + "\n\n"
	content += plan + "\n\n"
	content += "───────────────────────────────────────\n"
	content += approveStyle.Render("[Y] Approve") + "    " + rejectStyle.Render("[N] Reject") + queueNote + "\n"
	content += hintStyle.Render("Press Y / Enter to approve · N / Esc to reject")

	return boxStyle.Render(content)
}

// renderTabs renders the top navigation tabs.
// When m.compactTabs is true, only icon+key shortcut is shown (1 line, no border).
func (m *Model) renderTabs() string {
	var tabs []string

	if m.compactTabs {
		// Compact mode: icon + shortcut only, single line.
		renderCompact := func(icon string, state sessionState) string {
			if m.state == state {
				return m.styles.ActiveTab.Render(icon)
			}
			return m.styles.Tab.Render(icon)
		}
		tabs = append(tabs, renderCompact("💬", chatView))
		tabs = append(tabs, m.styles.TabGap.Render("|"))
		tabs = append(tabs, renderCompact("🤖", modelSelectView))
		tabs = append(tabs, m.styles.TabGap.Render("|"))
		tabs = append(tabs, renderCompact("🔧", toolsTableView))
		tabs = append(tabs, m.styles.TabGap.Render("|"))
		tabs = append(tabs, renderCompact("☁", discoveryView))
		tabs = append(tabs, m.styles.TabGap.Render("|"))
		tabs = append(tabs, renderCompact("📊", analyticsView))
		tabs = append(tabs, m.styles.TabGap.Render("|"))
		tabs = append(tabs, renderCompact("🩺", diagnosticsView))
		row := lipgloss.JoinHorizontal(lipgloss.Bottom, tabs...)
		return lipgloss.NewStyle().Width(m.width).Render(row)
	}

	// Full mode.
	renderTab := func(name string, state sessionState) string {
		if m.state == state {
			return m.styles.ActiveTab.Render(name)
		}
		return m.styles.Tab.Render(name)
	}

	tabs = append(tabs, renderTab("Chat", chatView))
	tabs = append(tabs, m.styles.TabGap.Render("|"))
	tabs = append(tabs, renderTab("Models (Ctrl+T)", modelSelectView))
	tabs = append(tabs, m.styles.TabGap.Render("|"))
	tabs = append(tabs, renderTab("Tools (Ctrl+E)", toolsTableView))
	tabs = append(tabs, m.styles.TabGap.Render("|"))
	tabs = append(tabs, renderTab("Cloud Brains (Ctrl+D)", discoveryView))
	tabs = append(tabs, m.styles.TabGap.Render("|"))
	tabs = append(tabs, renderTab("Analytics (Ctrl+A)", analyticsView))
	tabs = append(tabs, m.styles.TabGap.Render("|"))
	tabs = append(tabs, renderTab("Diagnostics (Ctrl+\\)", diagnosticsView))

	row := lipgloss.JoinHorizontal(lipgloss.Bottom, tabs...)
	return lipgloss.NewStyle().
		Width(m.width).
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(lipgloss.Color("240")).
		Render(row)
}

// renderChatView renders the standard chat interface
func (m *Model) renderChatView() string {
	// Viewport
	viewport := m.renderViewport()

	// Separator
	separator := m.styles.Help.Render(strings.Repeat("─", m.viewport.Width))

	// Loading
	var loading string
	if m.generating {
		loading = m.renderLoadingIndicator()
	}

	// Input
	input := m.renderInputArea()

	// Extended thinking panel (rendered when thinking budget is active).
	var thinkingBox string
	if m.thinkingBuf.Len() > 0 {
		thinkingBox = m.renderThinkingBox()
	}

	// Hook tree (live action log rendered below loading indicator)
	hookSection := m.renderHookSection()

	// Combine chat column
	parts := []string{viewport, separator}
	if thinkingBox != "" {
		parts = append(parts, thinkingBox)
	}
	if loading != "" {
		parts = append(parts, loading)
	}
	if hookSection != "" {
		parts = append(parts, hookSection)
	}
	parts = append(parts, input)

	chatContent := lipgloss.JoinVertical(lipgloss.Left, parts...)

	if !m.sidePanelOpen {
		return chatContent
	}
	// V.2: Clip chat column to the non-panel width without reflowing glamour content.
	chatW := m.width - m.sidePanelWidth - 1
	if chatW < 20 {
		chatW = 20
	}
	clippedChat := lipgloss.NewStyle().Width(chatW).MaxWidth(chatW).Render(chatContent)
	side := m.renderSidePanel()
	return lipgloss.JoinHorizontal(lipgloss.Top, clippedChat, side)
}

// renderNotificationZone renders up to 3 stacked toasts.
// Toasts are sorted highest-priority-first in the queue; we display them
// reversed so the most important toast sits at the bottom, closest to content.
// Each toast occupies exactly 1 terminal line.
func (m *Model) renderNotificationZone() string {
	if len(m.toasts) == 0 {
		return ""
	}

	// Clamp to at most 3 visible items (queue may hold up to 5).
	visible := m.toasts
	if len(visible) > 3 {
		visible = visible[:3]
	}

	// Build lines in reverse so highest-priority toast is at the bottom.
	lines := make([]string, len(visible))
	for i := len(visible) - 1; i >= 0; i-- {
		lines[len(visible)-1-i] = m.renderToastLine(visible[i])
	}
	return strings.Join(lines, "\n")
}

// renderToastLine renders a single toast as one full-width terminal line.
func (m *Model) renderToastLine(t toastItem) string {
	// Build text: icon + body + optional progress indicator.
	var sb strings.Builder
	sb.WriteString(t.Icon)
	sb.WriteString("  ")
	sb.WriteString(t.Text)

	if t.Kind == KindProgress {
		pct := t.Progress
		if pct < 0 {
			pct = 0
		}
		if pct > 1 {
			pct = 1
		}
		// Compact 8-block progress bar.
		const barW = 8
		filled := int(pct * barW)
		sb.WriteString(fmt.Sprintf(" %s%s %.0f%%",
			strings.Repeat("█", filled),
			strings.Repeat("░", barW-filled),
			pct*100,
		))
	}

	if t.Kind == KindPersistent {
		sb.WriteString(" ●") // visual indicator: not auto-dismissing
	}

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Color)).
		Background(lipgloss.Color(BgDarkAlt)).
		Width(m.width).
		Padding(0, 2).
		Render(sb.String())
}

// renderViewport renders the chat viewport
func (m *Model) renderViewport() string {
	return m.styles.Viewport.
		Width(m.width).
		Height(m.viewport.Height).
		Render(m.viewport.View())
}

// renderLoadingIndicator renders the loading spinner with a phase-aware label.
// Always uses the single-line DegradedSigilFrame to avoid consuming vertical space.
func (m *Model) renderLoadingIndicator() string {
	// Phase-aware label.
	var phaseLabel string
	switch m.genPhase {
	case phaseThinking:
		phaseLabel = "Reasoning..."
	case phaseTool:
		if m.currentActivity != nil {
			phaseLabel = m.currentActivity.Icon + " " + m.currentActivity.Label
		} else {
			phaseLabel = "⚙  Executing tool..."
		}
	case phaseSynthesizing:
		phaseLabel = "✍  Composing response..."
	default:
		phaseLabel = m.currentPhrase
	}

	spinGlyph := DegradedSigilFrame(m.hookSpinFrame)
	row := lipgloss.JoinHorizontal(lipgloss.Center, spinGlyph, "   ", loadingPhraseStyle.Render(phaseLabel))
	return loadingRowWrapper.Render(row)
}

// maxHookSectionLines is the maximum number of tree lines (excluding the header)
// the hook section may occupy.  This constant is the single source of truth used
// by both renderHookSection (for display truncation) and recalcViewportHeight
// (for height reservation), keeping layout arithmetic consistent.
const maxHookSectionLines = 8

// renderHookSection renders the live hook tree below the loading indicator.
// Returns "" when there are no hooks to show.
// Output is capped at maxHookSectionLines tree lines to prevent viewport overflow.
func (m *Model) renderHookSection() string {
	if len(m.activeHooks) == 0 || (!m.generating && !hasActiveHooks(m.activeHooks)) {
		return ""
	}
	// Available width is m.width minus side-panel if open.
	availW := m.width
	if m.sidePanelOpen {
		availW -= m.sidePanelWidth
	}
	if availW < 20 {
		return ""
	}
	tree := RenderHookTree(m.activeHooks, availW-4, m.hookSpinFrame, m.styles.Hook)
	if tree == "" {
		return ""
	}

	// Enforce line cap: truncate and append ellipsis if the tree is too tall.
	treeLines := strings.Split(tree, "\n")
	if len(treeLines) > maxHookSectionLines {
		treeLines = treeLines[:maxHookSectionLines]
		treeLines = append(treeLines, m.styles.Hook.Meta.Render("  … more"))
	}
	tree = strings.Join(treeLines, "\n")

	header := RenderHookHeader(availW-4, m.styles.Hook)
	return hookSectionWrapper.Render(header + "\n" + tree)
}

// renderThinkingBox renders the extended-thinking panel shown when Anthropic
// extended thinking is active.  The box is capped at 6 lines to avoid
// crowding the viewport; the most-recent lines are shown.
func (m *Model) renderThinkingBox() string {
	content := m.thinkingBuf.String()
	if content == "" {
		return ""
	}

	// Trim to the last 6 lines for a compact live view.
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) > 6 {
		lines = lines[len(lines)-6:]
	}
	preview := strings.Join(lines, "\n")

	label := "💭 Extended Thinking"
	if m.thinkingActive {
		label += "…"
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Foreground(lipgloss.Color("245")).
		Italic(true).
		Padding(0, 1).
		Width(m.width - 4)

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("243")).
		Bold(true)

	body := labelStyle.Render(label) + "\n" + preview
	return boxStyle.Render(body)
}

// renderInputArea renders the text input area (or search bar when in search mode).
func (m *Model) renderInputArea() string {
	if m.histSearchMode {
		return m.renderHistSearchBar()
	}

	if m.searchMode {
		return m.renderSearchBar()
	}

	input := m.textarea.View()
	help := m.renderInputHelp()
	parts := []string{input, help}

	// @ file autocomplete popup rendered above the textarea.
	if m.atCompleteActive && len(m.atCompleteItems) > 0 {
		parts = []string{m.renderAtCompletePopup(), input, help}
	}

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// renderSearchBar renders the inline search UI shown when Ctrl+K is active.
func (m *Model) renderSearchBar() string {
	// Count matches for display.
	total := len(m.searchMatches)
	var counter string
	if m.searchQuery == "" {
		counter = "type to search"
	} else if total == 0 {
		counter = "no matches"
	} else {
		counter = fmt.Sprintf("%d / %d", m.searchMatchIdx+1, total)
	}

	// Cursor blink — simple block cursor appended to query.
	queryDisplay := m.searchQuery + "█"

	barStyle := searchBarBase.Width(m.width - 2)

	left := searchIconStyle.Render("🔍 ") + searchQueryStyle.Render(queryDisplay)
	right := searchCounterStyle.Render(counter)

	// Pad left side to fill width, right-align counter.
	available := m.width - 4 - lipgloss.Width(left) - lipgloss.Width(right)
	if available < 1 {
		available = 1
	}
	bar := barStyle.Render(left + strings.Repeat(" ", available) + right)

	hint := m.styles.Help.Render("  ↑↓ navigate  Enter next  Esc close")
	return lipgloss.JoinVertical(lipgloss.Left, bar, hint)
}

// renderInputHelp renders help text for the input area
func (m *Model) renderInputHelp() string {
	if m.generating {
		return generatingBadge
	}
	return m.help.View(m.keymap)
}

// renderHistSearchBar renders the input history search bar (Ctrl+R).
func (m *Model) renderHistSearchBar() string {
	total := len(m.histSearchMatches)
	var counter string
	if m.histSearchQuery == "" {
		counter = "type to search history"
	} else if total == 0 {
		counter = "no matches"
	} else {
		counter = fmt.Sprintf("%d / %d", m.histSearchIdx+1, total)
	}

	queryDisplay := m.histSearchQuery + "█"

	barStyle := searchBarBase.Width(m.width - 2)

	left := searchIconStyle.Render("⏮ ") + searchQueryStyle.Render(queryDisplay)
	right := searchCounterStyle.Render(counter)

	available := m.width - 4 - lipgloss.Width(left) - lipgloss.Width(right)
	if available < 1 {
		available = 1
	}
	bar := barStyle.Render(left + strings.Repeat(" ", available) + right)
	hint := m.styles.Help.Render("  ↑↓ navigate  Enter select  Esc close")
	return lipgloss.JoinVertical(lipgloss.Left, bar, hint)
}

// renderStatusBar renders the bottom status bar
func (m *Model) renderStatusBar() string {
	return m.statusBar.View()
}

// renderAtCompletePopup renders a small @ file autocomplete dropdown above the input.
func (m *Model) renderAtCompletePopup() string {
	const maxVisible = 8
	items := m.atCompleteItems
	if len(items) > maxVisible {
		items = items[:maxVisible]
	}

	popStyle := atCompleteBase.Width(m.width - 4)

	var lines []string
	for i, item := range items {
		cursor := "  "
		style := atCompleteItemNormal
		if i == m.atCompleteIdx {
			cursor = "▶ "
			style = atCompleteItemSelected
		}
		lines = append(lines, cursor+style.Render(item))
	}
	content := strings.Join(lines, "\n")
	return atCompleteWrapper.Render(popStyle.Render(content))
}

// renderRewindMenu renders the double-Esc checkpoint rewind menu.
func (m *Model) renderRewindMenu() string {
	if len(m.rewindItems) == 0 {
		return ""
	}

	const maxVisible = 8
	boxWidth := m.width - 4
	if boxWidth > 80 {
		boxWidth = 80
	}

	boxStyle := rewindBoxBase.Width(boxWidth)

	items := m.rewindItems
	if len(items) > maxVisible {
		items = items[:maxVisible]
	}

	var lines []string
	lines = append(lines, rewindTitleStyle.Render("⏮  Rewind to checkpoint"))
	lines = append(lines, "")
	for i, cp := range items {
		cursor := "  "
		style := rewindDimStyle
		if i == m.rewindCursor {
			cursor = "▶ "
			style = rewindActiveStyle
		}
		ts := cp.Timestamp.Format("15:04:05")
		desc := truncate(cp.Description, boxWidth-30)
		line := fmt.Sprintf("%s%s  %s  (%d msgs)", cursor, ts, desc, cp.MessageCount)
		lines = append(lines, style.Render(line))
	}
	lines = append(lines, "")
	lines = append(lines, rewindHintStyle.Render("↑↓ navigate · Enter rewind · Esc cancel"))

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
		boxStyle.Render(strings.Join(lines, "\n")),
		lipgloss.WithWhitespaceChars("░"),
		lipgloss.WithWhitespaceForeground(lipgloss.Color("235")))
}

// Helper functions for styling specific content types...
// (Keep existing RenderUserMessage etc if needed)

func (m *Model) renderAuthWizard() string {
	if m.authRequest == nil {
		return ""
	}

	boxStyle := m.styles.ToolBox.Copy().
		BorderForeground(m.styles.StatusBarValue.GetForeground()).
		Width(60).
		Padding(1, 2)

	titleStyle := lipgloss.NewStyle().
		Foreground(m.styles.StatusBarValue.GetForeground()).
		Bold(true).
		MarginBottom(1)

	descStyle := lipgloss.NewStyle().
		Foreground(m.styles.Help.GetForeground()).
		MarginBottom(1)

	var sb strings.Builder
	sb.WriteString(titleStyle.Render("🔐 DYNAMIC SETUP WIZARD"))
	sb.WriteString("\n\n")
	sb.WriteString(fmt.Sprintf("Tool **%s** requires configuration.\n\n", m.authRequest.ToolName))
	sb.WriteString(descStyle.Render(m.authRequest.Description))
	sb.WriteString("\n\n")
	sb.WriteString(m.authInput.View())
	sb.WriteString("\n\n")
	sb.WriteString(m.styles.Help.Render("Press Enter to Save | Esc to Cancel"))

	return boxStyle.Render(sb.String())
}
