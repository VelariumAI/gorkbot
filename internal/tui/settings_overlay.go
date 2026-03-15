package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/velariumai/gorkbot/pkg/commands"
	pkgconfig "github.com/velariumai/gorkbot/pkg/config"
	"github.com/velariumai/gorkbot/pkg/tools"
)

// settingsTab enumerates the settings sections.
type settingsTab int

const (
	tabModels       settingsTab = iota // 0 — Model routing summary
	tabVerbosity                       // 1 — Debug / logging toggles
	tabTools                           // 2 — Tool group enable/disable
	tabProviders                       // 3 — API provider enable/disable
	tabIntegrations                    // 4 — Integration env vars (budget, webhook, etc.)
)

var tabLabels = []string{"Model Routing", "Verbosity", "Tool Groups", "API Providers", "Integrations"}

// SettingsOverlay is a four-tab modal for configuring runtime preferences.
// It implements the Overlay interface and persists changes immediately via
// the OrchestratorAdapter and tools.Registry.
type SettingsOverlay struct {
	BaseOverlay

	width  int
	height int

	activeTab settingsTab
	cursor    int

	orch           *commands.OrchestratorAdapter
	toolReg        *tools.Registry
	appStateSetter func(cats []string) error // writes disabled categories to disk

	// Verbosity section toggles (read from orch on construction)
	debugMode bool

	// Tool groups: sorted slice of category names + enabled state
	toolGroups []toolGroupRow

	// Provider rows: one row per known provider
	providerRows   []providerRow
	providerSetter func(ids []string) error // persists disabled provider list

	// Status line shown at the bottom of the modal after an action
	statusMsg   string
	statusIsErr bool

	// Integration settings (tabIntegrations)
	integrationGetter func() map[string]string      // returns current env var values
	integrationSetter func(key, value string) error // persists + os.Setenv

	// Inline text-editing state for tabIntegrations
	editingField bool
	editBuffer   string
	editOriginal string
}

type toolGroupRow struct {
	name    string
	enabled bool
}

type providerRow struct {
	id      string
	name    string
	enabled bool
}

// NewSettingsOverlay constructs a SettingsOverlay. Pass nil for any callback
// to skip persistence of that section.
func NewSettingsOverlay(
	w, h int,
	orch *commands.OrchestratorAdapter,
	toolReg *tools.Registry,
	appStateSetter func(cats []string) error,
	initialDebug bool,
	providerSetter func(ids []string) error,
	integrationGetter func() map[string]string,
	integrationSetter func(key, value string) error,
) *SettingsOverlay {
	s := &SettingsOverlay{
		width:             w,
		height:            h,
		orch:              orch,
		toolReg:           toolReg,
		appStateSetter:    appStateSetter,
		debugMode:         initialDebug,
		providerSetter:    providerSetter,
		integrationGetter: integrationGetter,
		integrationSetter: integrationSetter,
	}
	s.refreshToolGroups()
	s.refreshProviderRows()
	return s
}

func (s *SettingsOverlay) refreshToolGroups() {
	if s.toolReg == nil {
		return
	}
	cats := s.toolReg.Categories()
	rows := make([]toolGroupRow, 0, len(cats))
	for _, c := range cats {
		rows = append(rows, toolGroupRow{
			name:    string(c),
			enabled: s.toolReg.IsCategoryEnabled(c),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })
	s.toolGroups = rows
}

func (s *SettingsOverlay) refreshProviderRows() {
	if s.orch == nil || s.orch.GetProviderEnabled == nil {
		return
	}
	enabled := s.orch.GetProviderEnabled()
	// Use a fixed display order matching providerPriority.
	ordered := []string{"xai", "google", "anthropic", "minimax", "openai", "openrouter"}
	names := map[string]string{
		"xai":        "xAI",
		"google":     "Google",
		"anthropic":  "Anthropic",
		"minimax":    "MiniMax",
		"openai":     "OpenAI",
		"openrouter": "OpenRouter",
	}
	rows := make([]providerRow, 0, len(ordered))
	for _, id := range ordered {
		e, ok := enabled[id]
		if !ok {
			e = true // default to enabled if not in map
		}
		rows = append(rows, providerRow{
			id:      id,
			name:    names[id],
			enabled: e,
		})
	}
	s.providerRows = rows
}

// ── Overlay interface ─────────────────────────────────────────────────────────

func (s *SettingsOverlay) Init() tea.Cmd { return nil }

func (s *SettingsOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}

	// ── In-field text editing mode ────────────────────────────────────────────
	if s.editingField {
		switch keyMsg.String() {
		case "esc", "ctrl+c":
			// Cancel: restore original value.
			s.editBuffer = s.editOriginal
			s.editingField = false
			s.statusMsg = "Edit cancelled"
		case "enter":
			// Save value.
			s.saveIntegrationField()
			s.editingField = false
		case "backspace", "ctrl+h":
			if len(s.editBuffer) > 0 {
				runes := []rune(s.editBuffer)
				s.editBuffer = string(runes[:len(runes)-1])
			}
		case "ctrl+u":
			s.editBuffer = ""
		default:
			// Append printable characters.
			if len(keyMsg.Runes) > 0 && keyMsg.Runes[0] >= 32 {
				s.editBuffer += string(keyMsg.Runes)
			}
		}
		return s, nil
	}

	s.statusMsg = "" // clear status on any key

	switch keyMsg.String() {
	case "esc":
		return nil, nil // signals Model.Update to close the overlay

	case "tab":
		s.activeTab = settingsTab((int(s.activeTab) + 1) % len(tabLabels))
		s.cursor = 0
	case "shift+tab":
		s.activeTab = settingsTab((int(s.activeTab) + len(tabLabels) - 1) % len(tabLabels))
		s.cursor = 0

	case "up", "k":
		if s.cursor > 0 {
			s.cursor--
		}
	case "down", "j":
		maxCursor := s.maxCursor()
		if s.cursor < maxCursor {
			s.cursor++
		}

	case "enter":
		if s.activeTab == tabIntegrations {
			s.startEditingField()
		} else {
			s.handleAction()
		}
	case " ":
		if s.activeTab != tabIntegrations {
			s.handleAction()
		}
	}

	return s, nil
}

// startEditingField enters inline-edit mode for the currently selected integration row.
func (s *SettingsOverlay) startEditingField() {
	if s.cursor >= len(pkgconfig.IntegrationKeys) {
		return
	}
	key := pkgconfig.IntegrationKeys[s.cursor].Key
	current := ""
	if s.integrationGetter != nil {
		current = s.integrationGetter()[key]
	}
	s.editBuffer = current
	s.editOriginal = current
	s.editingField = true
	s.statusMsg = ""
}

// saveIntegrationField persists the editBuffer for the currently selected row.
func (s *SettingsOverlay) saveIntegrationField() {
	if s.cursor >= len(pkgconfig.IntegrationKeys) {
		return
	}
	key := pkgconfig.IntegrationKeys[s.cursor].Key
	if s.integrationSetter != nil {
		if err := s.integrationSetter(key, s.editBuffer); err != nil {
			s.statusMsg = fmt.Sprintf("Save failed: %v", err)
			s.statusIsErr = true
			return
		}
	}
	s.statusMsg = fmt.Sprintf("%s updated", pkgconfig.IntegrationKeys[s.cursor].Label)
	s.statusIsErr = false
}

func (s *SettingsOverlay) maxCursor() int {
	switch s.activeTab {
	case tabVerbosity:
		return 0 // one toggle
	case tabTools:
		if len(s.toolGroups) == 0 {
			return 0
		}
		return len(s.toolGroups) - 1
	case tabProviders:
		if len(s.providerRows) == 0 {
			return 0
		}
		return len(s.providerRows) - 1
	case tabIntegrations:
		return len(pkgconfig.IntegrationKeys) - 1
	default:
		return 0
	}
}

func (s *SettingsOverlay) handleAction() {
	switch s.activeTab {
	case tabVerbosity:
		if s.orch != nil && s.orch.ToggleDebug != nil {
			s.debugMode = s.orch.ToggleDebug()
			if s.debugMode {
				s.statusMsg = "Debug mode ON — raw tool JSON visible"
			} else {
				s.statusMsg = "Debug mode OFF"
			}
		}

	case tabTools:
		if s.toolReg == nil || s.cursor >= len(s.toolGroups) {
			return
		}
		row := &s.toolGroups[s.cursor]
		row.enabled = !row.enabled
		s.toolReg.SetCategoryEnabled(tools.ToolCategory(row.name), row.enabled)
		if row.enabled {
			s.statusMsg = fmt.Sprintf("Category %q enabled", row.name)
		} else {
			s.statusMsg = fmt.Sprintf("Category %q disabled", row.name)
		}
		// Persist disabled list
		if s.appStateSetter != nil {
			var disabled []string
			for _, r := range s.toolGroups {
				if !r.enabled {
					disabled = append(disabled, r.name)
				}
			}
			if err := s.appStateSetter(disabled); err != nil {
				s.statusMsg += " (save failed)"
				s.statusIsErr = true
			}
		}

	case tabProviders:
		if s.orch == nil || s.orch.ToggleProvider == nil || s.cursor >= len(s.providerRows) {
			return
		}
		row := &s.providerRows[s.cursor]
		enabled, msg := s.orch.ToggleProvider(row.id)
		row.enabled = enabled
		s.statusMsg = msg
		s.statusIsErr = false
		// Persist full disabled list via providerSetter.
		if s.providerSetter != nil {
			var disabled []string
			for _, r := range s.providerRows {
				if !r.enabled {
					disabled = append(disabled, r.id)
				}
			}
			if err := s.providerSetter(disabled); err != nil {
				s.statusMsg += " (save failed)"
				s.statusIsErr = true
			}
		}
	}
}

// View renders the settings modal. Self-contained box using lipgloss.
func (s *SettingsOverlay) View() string {
	boxW := s.width - 8
	if boxW < 50 {
		boxW = 50
	}
	if boxW > 90 {
		boxW = 90
	}

	// Styles
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("213")).Bold(true)
	activeTabStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("99")).Foreground(lipgloss.Color("255")).Padding(0, 1)
	inactiveTabStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).Padding(0, 1)
	cursorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("213")).Bold(true)
	checkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("76"))
	uncheckStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("76"))
	if s.statusIsErr {
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	}
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	var lines []string

	// Title
	lines = append(lines, titleStyle.Render("⚙  Settings"))
	lines = append(lines, "")

	// Tab bar
	tabs := make([]string, len(tabLabels))
	for i, label := range tabLabels {
		if settingsTab(i) == s.activeTab {
			tabs[i] = activeTabStyle.Render(label)
		} else {
			tabs[i] = inactiveTabStyle.Render(label)
		}
	}
	lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, tabs...))
	lines = append(lines, strings.Repeat("─", boxW-4))
	lines = append(lines, "")

	// Section content
	switch s.activeTab {
	case tabModels:
		lines = append(lines, s.renderModelsSection(dimStyle)...)
	case tabVerbosity:
		lines = append(lines, s.renderVerbositySection(cursorStyle, checkStyle, uncheckStyle)...)
	case tabTools:
		lines = append(lines, s.renderToolsSection(cursorStyle, checkStyle, uncheckStyle, dimStyle)...)
	case tabProviders:
		lines = append(lines, s.renderProvidersSection(cursorStyle, checkStyle, uncheckStyle, dimStyle)...)
	case tabIntegrations:
		lines = append(lines, s.renderIntegrationsSection(cursorStyle, dimStyle, boxW)...)
	}

	// Status
	lines = append(lines, "")
	if s.statusMsg != "" {
		lines = append(lines, statusStyle.Render("  "+s.statusMsg))
	} else {
		lines = append(lines, "")
	}

	// Help line
	lines = append(lines, strings.Repeat("─", boxW-4))
	var helpText string
	if s.editingField {
		helpText = "  Type value  Enter=save  Esc=cancel  Ctrl+U=clear"
	} else if s.activeTab == tabIntegrations {
		helpText = "  Tab=switch section  ↑↓=navigate  Enter=edit field  Esc=close"
	} else {
		helpText = "  Tab=switch section  ↑↓=navigate  Enter/Space=toggle  Esc=close"
	}
	lines = append(lines, helpStyle.Render(helpText))

	content := strings.Join(lines, "\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("99")).
		Padding(1, 2).
		Width(boxW).
		Render(content)

	return lipgloss.Place(s.width, s.height,
		lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceChars("░"),
		lipgloss.WithWhitespaceForeground(lipgloss.Color("235")))
}

func (s *SettingsOverlay) renderModelsSection(dim lipgloss.Style) []string {
	var lines []string
	if s.orch == nil {
		lines = append(lines, dim.Render("  Orchestrator not available."))
		return lines
	}
	// Show current primary/secondary from provider status
	status := ""
	if s.orch.GetProviderStatus != nil {
		status = s.orch.GetProviderStatus()
	}
	if status != "" {
		for _, l := range strings.Split(status, "\n") {
			lines = append(lines, "  "+l)
		}
	} else {
		lines = append(lines, dim.Render("  No provider status available."))
	}
	lines = append(lines, "")
	lines = append(lines, dim.Render("  Press Ctrl+T to open the Model Selector."))

	// ── Live System State (Phase 4.3) ──────────────────────────────────────
	if s.orch != nil && s.orch.GetDiagnosticReport != nil {
		report := s.orch.GetDiagnosticReport()
		if report != "" {
			lines = append(lines, "")
			lines = append(lines, lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("213")).
				Render("── System State ──────────────────────────────"))
			for _, l := range strings.Split(report, "\n") {
				if strings.TrimSpace(l) != "" && !strings.HasPrefix(l, "##") {
					lines = append(lines, "  "+l)
				}
			}
		}
	}
	return lines
}

func (s *SettingsOverlay) renderVerbositySection(cursor, check, uncheck lipgloss.Style) []string {
	var lines []string
	lines = append(lines, s.renderToggleRow(0, "Debug mode (show raw tool JSON)", s.debugMode, cursor, check, uncheck))
	return lines
}

func (s *SettingsOverlay) renderToolsSection(cur, check, uncheck, dim lipgloss.Style) []string {
	var lines []string
	if len(s.toolGroups) == 0 {
		lines = append(lines, dim.Render("  No tool categories registered."))
		return lines
	}
	hdr := fmt.Sprintf("  %-20s  %s", "Category", "Status")
	lines = append(lines, dim.Render(hdr))
	for i, row := range s.toolGroups {
		lines = append(lines, s.renderToggleRow(i, row.name, row.enabled, cur, check, uncheck))
	}
	return lines
}

func (s *SettingsOverlay) renderToggleRow(idx int, label string, enabled bool, cursor, check, uncheck lipgloss.Style) string {
	arrow := "  "
	if s.activeTab != tabModels && idx == s.cursor {
		arrow = cursor.Render("> ")
	}
	box := uncheck.Render("[ ]")
	if enabled {
		box = check.Render("[x]")
	}
	return fmt.Sprintf("%s%s %-22s", arrow, box, label)
}

func (s *SettingsOverlay) renderProvidersSection(cur, check, uncheck, dim lipgloss.Style) []string {
	var lines []string
	if len(s.providerRows) == 0 {
		lines = append(lines, dim.Render("  No providers registered."))
		return lines
	}
	hdr := fmt.Sprintf("  %-20s  %s", "Provider", "Status")
	lines = append(lines, dim.Render(hdr))
	for i, row := range s.providerRows {
		lines = append(lines, s.renderToggleRow(i, row.name, row.enabled, cur, check, uncheck))
	}
	lines = append(lines, "")
	lines = append(lines, dim.Render("  Disabled providers are skipped during failover cascade."))
	return lines
}

func (s *SettingsOverlay) renderIntegrationsSection(cur, dim lipgloss.Style, boxW int) []string {
	var lines []string

	// Gather current values.
	vals := map[string]string{}
	if s.integrationGetter != nil {
		vals = s.integrationGetter()
	}

	editStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true) // amber for edit mode
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	sensitiveStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))

	// Group header names (derived from key prefixes).
	lastGroup := ""
	groupOf := func(key string) string {
		switch {
		case strings.HasPrefix(key, "BUDGET_"):
			return "Budget Limits"
		case strings.HasPrefix(key, "WEBHOOK_"):
			return "Webhook Server"
		case strings.HasPrefix(key, "SCHEDULER_"):
			return "Scheduler Notifications"
		default:
			return "Other"
		}
	}

	for i, entry := range pkgconfig.IntegrationKeys {
		// Emit group header when group changes.
		if g := groupOf(entry.Key); g != lastGroup {
			lastGroup = g
			if i > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, lipgloss.NewStyle().
				Foreground(lipgloss.Color("99")).Bold(true).
				Render("  ── "+g+" ──"))
		}

		isCurrent := i == s.cursor
		arrow := "  "
		if isCurrent && !s.editingField {
			arrow = cur.Render("> ")
		} else if isCurrent && s.editingField {
			arrow = editStyle.Render("> ")
		}

		// Build value display.
		var displayVal string
		if isCurrent && s.editingField {
			// Show edit buffer with blinking cursor character.
			buf := s.editBuffer
			if entry.Sensitive {
				buf = strings.Repeat("*", len([]rune(s.editBuffer)))
			}
			displayVal = editStyle.Render("[" + buf + "█]")
		} else {
			v := vals[entry.Key]
			if v == "" {
				displayVal = dim.Render("(not set)")
			} else if entry.Sensitive {
				displayVal = sensitiveStyle.Render(strings.Repeat("*", minInt(len(v), 12)))
			} else {
				maxLen := boxW - 28
				if maxLen < 8 {
					maxLen = 8
				}
				if len(v) > maxLen {
					v = v[:maxLen-1] + "…"
				}
				displayVal = valueStyle.Render(v)
			}
		}

		label := entry.Label
		if len(label) > 22 {
			label = label[:21] + "…"
		}
		line := fmt.Sprintf("%s%-23s %s", arrow, label, displayVal)
		lines = append(lines, line)

		// Show description below selected (non-editing) row.
		if isCurrent && !s.editingField {
			lines = append(lines, descStyle.Render("     "+entry.Description))
		}
	}

	if s.integrationGetter == nil {
		lines = append(lines, "")
		lines = append(lines, dim.Render("  (settings not configured — restart required)"))
	} else {
		lines = append(lines, "")
		lines = append(lines, dim.Render("  Budget/notification changes take effect immediately."))
		lines = append(lines, dim.Render("  Webhook port changes require restart."))
	}
	return lines
}

// min is a helper for int minimum (Go 1.21 has built-in but keeping compat).
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
