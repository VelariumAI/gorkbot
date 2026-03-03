package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/velariumai/gorkbot/pkg/commands"
	"github.com/velariumai/gorkbot/pkg/tools"
)

// settingsTab enumerates the settings sections.
type settingsTab int

const (
	tabModels    settingsTab = iota // 0 — Model routing summary
	tabVerbosity                    // 1 — Debug / logging toggles
	tabTools                        // 2 — Tool group enable/disable
	tabProviders                    // 3 — API provider enable/disable
)

var tabLabels = []string{"Model Routing", "Verbosity", "Tool Groups", "API Providers"}

// SettingsOverlay is a four-tab modal for configuring runtime preferences.
// It implements the Overlay interface and persists changes immediately via
// the OrchestratorAdapter and tools.Registry.
type SettingsOverlay struct {
	BaseOverlay

	width  int
	height int

	activeTab settingsTab
	cursor    int

	orch        *commands.OrchestratorAdapter
	toolReg     *tools.Registry
	appStateSetter func(cats []string) error // writes disabled categories to disk

	// Verbosity section toggles (read from orch on construction)
	debugMode bool

	// Tool groups: sorted slice of category names + enabled state
	toolGroups []toolGroupRow

	// Provider rows: one row per known provider
	providerRows    []providerRow
	providerSetter  func(ids []string) error // persists disabled provider list

	// Status line shown at the bottom of the modal after an action
	statusMsg   string
	statusIsErr bool
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

// NewSettingsOverlay constructs a SettingsOverlay. Pass nil for appStateSetter
// or providerSetter to skip disk persistence of those sections.
func NewSettingsOverlay(
	w, h int,
	orch *commands.OrchestratorAdapter,
	toolReg *tools.Registry,
	appStateSetter func(cats []string) error,
	initialDebug bool,
	providerSetter func(ids []string) error,
) *SettingsOverlay {
	s := &SettingsOverlay{
		width:          w,
		height:         h,
		orch:           orch,
		toolReg:        toolReg,
		appStateSetter: appStateSetter,
		debugMode:      initialDebug,
		providerSetter: providerSetter,
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

	case "enter", " ":
		s.handleAction()
	}

	return s, nil
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
	lines = append(lines, helpStyle.Render("  Tab=switch section  ↑↓=navigate  Enter/Space=toggle  Esc=close"))

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
