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

// renderTabs renders the top navigation tabs
func (m *Model) renderTabs() string {
	var tabs []string
	
	// Define tabs
	// Chat (Ctrl+H or default)
	// Models (Ctrl+T)
	// Tools (Ctrl+E)
	
	// Helper to render a tab
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
		BorderForeground(lipgloss.Color("240")). // Subtle separator
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

	// Combine chat column
	parts := []string{viewport, separator}
	if loading != "" {
		parts = append(parts, loading)
	}
	parts = append(parts, input)

	chatContent := lipgloss.JoinVertical(lipgloss.Left, parts...)

	if !m.sidePanelOpen {
		return chatContent
	}
	// Join with side panel
	side := m.renderSidePanel()
	return lipgloss.JoinHorizontal(lipgloss.Top, chatContent, side)
}

// renderNotificationZone renders the active toast (1 line, empty string when no toasts).
func (m *Model) renderNotificationZone() string {
	if len(m.toasts) == 0 {
		return ""
	}
	t := m.toasts[len(m.toasts)-1]
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.Color)).
		Background(lipgloss.Color(BgDarkAlt)).
		Width(m.width).
		Padding(0, 2).
		Render(t.Icon + "  " + t.Text)
}

// renderViewport renders the chat viewport
func (m *Model) renderViewport() string {
	return m.styles.Viewport.
		Width(m.width).
		Height(m.viewport.Height).
		Render(m.viewport.View())
}

// renderLoadingIndicator renders the loading spinner, phrase, and live tool panel.
func (m *Model) renderLoadingIndicator() string {
	spinnerView := m.styles.Spinner.Render(m.spinner.View())
	phraseStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(GrokBlue)).Italic(false)
	phraseView := phraseStyle.Render(m.currentPhrase)
	indicatorRow := lipgloss.JoinHorizontal(lipgloss.Center, spinnerView, "   ", phraseView)

	lines := []string{indicatorRow}

	// Live tool execution panel — show tools currently running.
	if len(m.activeTools) > 0 {
		toolStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Italic(true)
		for _, toolName := range m.activeTools {
			lines = append(lines, toolStyle.Render("    ⚙ "+toolName+"..."))
		}
	}

	combined := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return lipgloss.NewStyle().Padding(1, 2).Render(combined)
}

// renderInputArea renders the text input area
func (m *Model) renderInputArea() string {
	if m.generating {
		return m.styles.Help.Width(m.width).Render("⏳ Generating... (Esc to cancel)")
	}
	input := m.textarea.View()
	help := m.renderInputHelp()
	return lipgloss.JoinVertical(lipgloss.Left, input, help)
}

// renderInputHelp renders help text for the input area
func (m *Model) renderInputHelp() string {
	// Use bubbles/help view if available, or fallback to simple string
	return m.help.View(m.keymap)
}

// renderStatusBar renders the bottom status bar
func (m *Model) renderStatusBar() string {
	return m.statusBar.View()
}

// Helper functions for styling specific content types...
// (Keep existing RenderUserMessage etc if needed)
