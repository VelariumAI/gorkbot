package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/commands"
	"github.com/velariumai/gorkbot/pkg/providers"
)

// providerOrder is the canonical display order of providers.
var providerOrder = []string{
	providers.ProviderXAI,
	providers.ProviderGoogle,
	providers.ProviderAnthropic,
	providers.ProviderOpenAI,
	providers.ProviderMiniMax,
}

// ── initModelSelectState ──────────────────────────────────────────────────────

// initModelSelectState initialises the dual-pane state if it hasn't been yet.
func (m *Model) initModelSelectState() {
	if m.modelSelect.refreshing != nil {
		return
	}
	m.modelSelect.refreshing = make(map[string]bool)

	// Build primary list
	primaryList := list.New(nil, list.NewDefaultDelegate(), m.width, m.height-10)
	primaryList.Title = "PRIMARY MODEL"
	primaryList.SetShowStatusBar(false)
	primaryList.SetFilteringEnabled(true)
	m.modelSelect.primaryList = primaryList

	// Build secondary list with "Auto" as first item
	secondaryList := list.New(nil, list.NewDefaultDelegate(), m.width, m.height-10)
	secondaryList.Title = "SECONDARY MODEL"
	secondaryList.SetShowStatusBar(false)
	secondaryList.SetFilteringEnabled(true)
	m.modelSelect.secondaryList = secondaryList

	m.refreshModelSelectLists()
}

// refreshModelSelectLists repopulates the list items from availableModels.
// It is a no-op if the dual-pane state hasn't been initialised yet (i.e. the
// model select view has never been opened). Models accumulate in m.availableModels
// and will be shown the first time the view is rendered.
func (m *Model) refreshModelSelectLists() {
	if m.modelSelect.refreshing == nil {
		return // not yet initialised; initModelSelectState will call us when the view opens
	}
	filter := m.modelSelect.providerFilter
	primaryID := m.currentModel
	consultantID := ""
	if m.orchestrator != nil {
		if consultant := m.orchestrator.Consultant(); consultant != nil {
			consultantID = consultant.GetMetadata().ID
		}
	}

	// Primary items
	var primaryItems []list.Item
	for _, mi := range m.availableModels {
		if filter != "" && mi.Provider != filter {
			continue
		}
		primaryItems = append(primaryItems, modelItem{
			id:       mi.ID,
			name:     mi.Name,
			provider: mi.Provider,
			thinking: mi.Thinking,
			active:   mi.ID == primaryID,
		})
	}
	m.modelSelect.primaryList.SetItems(primaryItems)

	// Secondary items — "Auto" first, then all models
	secondaryItems := []list.Item{
		modelItem{isAuto: true, active: consultantID == ""},
	}
	for _, mi := range m.availableModels {
		if filter != "" && mi.Provider != filter {
			continue
		}
		secondaryItems = append(secondaryItems, modelItem{
			id:       mi.ID,
			name:     mi.Name,
			provider: mi.Provider,
			thinking: mi.Thinking,
			active:   mi.ID == consultantID,
		})
	}
	m.modelSelect.secondaryList.SetItems(secondaryItems)
}

// ── renderModelSelectView ─────────────────────────────────────────────────────

// renderModelSelectView renders the dual-pane model selection UI.
func (m *Model) renderModelSelectView() string {
	m.initModelSelectState()

	borderColor := lipgloss.Color("99")
	activeBorder := lipgloss.Color("213")

	paneStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1)

	activePaneStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(activeBorder).
		Padding(0, 1)

	halfW := (m.width - 6) / 2

	// Provider filter bar
	filterBar := m.renderProviderFilterBar()

	// Left pane: PRIMARY
	primaryStyle := paneStyle
	if m.modelSelect.activePane == 0 {
		primaryStyle = activePaneStyle
	}
	m.modelSelect.primaryList.SetSize(halfW-4, m.height-16)
	leftPane := primaryStyle.Width(halfW).Render(m.modelSelect.primaryList.View())

	// Right pane: SECONDARY
	secondaryStyle := paneStyle
	if m.modelSelect.activePane == 1 {
		secondaryStyle = activePaneStyle
	}
	m.modelSelect.secondaryList.SetSize(halfW-4, m.height-16)
	rightPane := secondaryStyle.Width(halfW).Render(m.modelSelect.secondaryList.View())

	// Side by side
	panes := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, "  ", rightPane)

	// API key status footer
	keyFooter := m.renderKeyStatusFooter()

	// Help line
	helpLine := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).
		Render("  Tab=switch pane  ↑↓=navigate  Enter=select  r=refresh  k=add key  p=cycle provider  Esc=back")

	return lipgloss.JoinVertical(lipgloss.Left,
		filterBar,
		panes,
		keyFooter,
		helpLine,
	)
}

// renderProviderFilterBar renders the provider filter pill row.
func (m *Model) renderProviderFilterBar() string {
	filter := m.modelSelect.providerFilter
	activeStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("99")).
		Foreground(lipgloss.Color("255")).
		Padding(0, 1)
	inactiveStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Padding(0, 1)

	pills := []string{
		lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("  Providers:"),
	}
	allStyle := inactiveStyle
	if filter == "" {
		allStyle = activeStyle
	}
	pills = append(pills, allStyle.Render("[All]"))

	for _, p := range providerOrder {
		style := inactiveStyle
		if filter == p {
			style = activeStyle
		}
		icon := "✗"
		for _, ps := range m.modelSelect.providerKeys {
			if ps.Provider == p {
				switch ps.Status {
				case 2: // valid
					icon = "●"
				case 1: // unverified
					icon = "?"
				}
			}
		}
		pills = append(pills, style.Render(fmt.Sprintf("[%s%s]", providers.ProviderName(p), icon)))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, pills...)
}

// renderKeyStatusFooter renders the provider key status line at the bottom.
func (m *Model) renderKeyStatusFooter() string {
	if len(m.modelSelect.providerKeys) == 0 {
		return ""
	}
	var parts []string
	for _, ps := range m.modelSelect.providerKeys {
		icon := "✗"
		color := lipgloss.Color("196")
		switch ps.Status {
		case 2: // valid
			icon = "●"
			color = lipgloss.Color("76")
		case 1: // unverified
			icon = "?"
			color = lipgloss.Color("226")
		}
		parts = append(parts, lipgloss.NewStyle().Foreground(color).Render(
			fmt.Sprintf(" %s %s", icon, providers.ProviderName(ps.Provider)),
		))
	}
	return "  " + strings.Join(parts, "  ")
}

// ── model select update handler ────────────────────────────────────────────────

// updateModelSelectView handles key events in the model select view.
func (m *Model) updateModelSelectView(msg tea.Msg) (tea.Model, tea.Cmd) {
	// API key prompt captures all input when active
	if m.apiKeyPrompt.active {
		return m.updateAPIKeyPrompt(msg)
	}

	keyMsg, isKey := msg.(tea.KeyMsg)
	if !isKey {
		// Let the active list handle non-key messages
		var cmd tea.Cmd
		if m.modelSelect.activePane == 0 {
			m.modelSelect.primaryList, cmd = m.modelSelect.primaryList.Update(msg)
		} else {
			m.modelSelect.secondaryList, cmd = m.modelSelect.secondaryList.Update(msg)
		}
		return m, cmd
	}

	switch keyMsg.String() {
	case "esc":
		if m.modelSelect.activePane == 0 &&
			m.modelSelect.primaryList.FilterState() == list.Filtering {
			break // let list clear filter
		}
		if m.modelSelect.activePane == 1 &&
			m.modelSelect.secondaryList.FilterState() == list.Filtering {
			break
		}
		m.state = chatView
		return m, nil

	case "tab", "shift+tab":
		if m.modelSelect.activePane == 0 {
			m.modelSelect.activePane = 1
		} else {
			m.modelSelect.activePane = 0
		}
		return m, nil

	case "enter":
		return m.handleModelSelectEnter()

	case "r":
		return m.handleModelSelectRefresh()

	case "k":
		return m.handleModelSelectAddKey()

	case "p":
		return m.handleModelSelectCycleProvider()
	}

	// Delegate navigation to the active list
	var cmd tea.Cmd
	if m.modelSelect.activePane == 0 {
		m.modelSelect.primaryList, cmd = m.modelSelect.primaryList.Update(msg)
	} else {
		m.modelSelect.secondaryList, cmd = m.modelSelect.secondaryList.Update(msg)
	}
	return m, cmd
}

func (m *Model) handleModelSelectEnter() (tea.Model, tea.Cmd) {
	var selected list.Item
	if m.modelSelect.activePane == 0 {
		selected = m.modelSelect.primaryList.SelectedItem()
	} else {
		selected = m.modelSelect.secondaryList.SelectedItem()
	}
	if selected == nil {
		return m, nil
	}
	mi, ok := selected.(modelItem)
	if !ok {
		return m, nil
	}

	if m.modelSelect.activePane == 1 && mi.isAuto {
		// Set secondary to auto mode
		// (Auto mode is handled by SetAutoSecondary command below)
		m.addSystemMessage("Secondary set to **Auto** — AI selects best consultant per task.")
		// Persist auto selection
		if m.commands != nil && m.commands.Orch != nil && m.commands.Orch.SetAutoSecondary != nil {
			m.commands.Orch.SetAutoSecondary()
		}
		m.state = chatView
		return m, nil
	}

	// Check if provider key is available
	hasKey := m.providerHasKey(mi.provider)
	if !hasKey {
		// Trigger key prompt
		m.apiKeyPrompt = apiKeyPromptState{
			active:     true,
			provider:   mi.provider,
			websiteURL: providers.ProviderWebsite(mi.provider),
		}
		return m, nil
	}

	// Switch model
	if m.modelSelect.activePane == 0 {
		if err := m.switchPrimaryModelByProvider(mi.provider, mi.id); err != nil {
			m.addSystemMessage(fmt.Sprintf("Failed to switch primary: %v", err))
		} else {
			m.addSystemMessage(fmt.Sprintf("Primary switched to **%s** (%s)", mi.name, providers.ProviderName(mi.provider)))
			m.refreshModelSelectLists()
			// Persist selection
			if m.commands != nil && m.commands.Orch != nil && m.commands.Orch.SetPrimary != nil {
				m.commands.Orch.SetPrimary(mi.provider, mi.id)
			}
		}
	} else {
		if err := m.switchSecondaryModelByProvider(mi.provider, mi.id); err != nil {
			m.addSystemMessage(fmt.Sprintf("Failed to switch secondary: %v", err))
		} else {
			m.addSystemMessage(fmt.Sprintf("Secondary switched to **%s** (%s)", mi.name, providers.ProviderName(mi.provider)))
			m.refreshModelSelectLists()
			// Persist selection
			if m.commands != nil && m.commands.Orch != nil && m.commands.Orch.SetSecondary != nil {
				m.commands.Orch.SetSecondary(mi.provider, mi.id)
			}
		}
	}
	m.state = chatView
	return m, nil
}

func (m *Model) handleModelSelectRefresh() (tea.Model, tea.Cmd) {
	activeProvider := m.getCurrentPaneProvider()
	if activeProvider == "" || m.modelSelect.refreshing[activeProvider] {
		return m, nil
	}
	m.modelSelect.refreshing[activeProvider] = true
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		var models []commands.ModelInfo
		pm := providers.GetGlobalProviderManager()
		if pm != nil {
			defs, err := pm.PollProvider(ctx, activeProvider)
			if err == nil && len(defs) > 0 {
				for _, d := range defs {
					models = append(models, commands.ModelInfo{
						ID:       string(d.ID),
						Name:     d.Name,
						Provider: string(d.Provider),
						Thinking: d.Capabilities.SupportsThinking,
					})
				}
			}
		}
		// Always fall back to safe statics when live poll fails or returns empty.
		if len(models) == 0 {
			for _, d := range ai.SafeModelDefs(activeProvider) {
				models = append(models, commands.ModelInfo{
					ID:       string(d.ID),
					Name:     d.Name,
					Provider: string(d.Provider),
					Thinking: d.Capabilities.SupportsThinking,
				})
			}
		}
		return ModelRefreshMsg{Provider: activeProvider, Models: models}
	}
}

func (m *Model) handleModelSelectAddKey() (tea.Model, tea.Cmd) {
	// Priority: active provider filter > highlighted item's provider > default (xAI).
	// When the user has cycled to a provider via 'p', the filter reflects their intent
	// even if the list is empty (no key yet → no models → no highlighted item).
	provider := m.modelSelect.providerFilter
	if provider == "" {
		provider = m.getCurrentPaneProvider()
	}
	if provider == "" {
		provider = providerOrder[0]
	}
	m.apiKeyPrompt = apiKeyPromptState{
		active:     true,
		provider:   provider,
		websiteURL: providers.ProviderWebsite(provider),
	}
	return m, nil
}

func (m *Model) handleModelSelectCycleProvider() (tea.Model, tea.Cmd) {
	allFilters := append([]string{""}, providerOrder...)
	current := m.modelSelect.providerFilter
	for i, f := range allFilters {
		if f == current {
			m.modelSelect.providerFilter = allFilters[(i+1)%len(allFilters)]
			break
		}
	}
	m.refreshModelSelectLists()
	return m, nil
}

// getCurrentPaneProvider returns the provider of the currently highlighted item.
func (m *Model) getCurrentPaneProvider() string {
	var selected list.Item
	if m.modelSelect.activePane == 0 {
		selected = m.modelSelect.primaryList.SelectedItem()
	} else {
		selected = m.modelSelect.secondaryList.SelectedItem()
	}
	if mi, ok := selected.(modelItem); ok && !mi.isAuto {
		return mi.provider
	}
	return ""
}

// providerHasKey checks if the given provider has a key available.
func (m *Model) providerHasKey(provider string) bool {
	pm := providers.GetGlobalProviderManager()
	if pm == nil {
		// Fallback: check if it's an already-initialised provider
		return provider == providers.ProviderXAI || provider == providers.ProviderGoogle
	}
	key, _ := pm.KeyStore().Get(provider)
	return key != ""
}

// switchPrimaryModelByProvider hot-swaps primary using the provider manager.
func (m *Model) switchPrimaryModelByProvider(providerName, modelID string) error {
	if m.orchestrator == nil {
		return fmt.Errorf("orchestrator not available")
	}
	ctx := context.Background()
	err := m.orchestrator.SetPrimary(ctx, providerName, modelID)
	if err != nil {
		return err
	}
	m.currentModel = modelID
	m.commands.UpdateCurrentPrimary(commands.ModelInfo{
		ID:       modelID,
		Provider: providerName,
	})
	return nil
}

// pollAllConfiguredProviders returns a tea.Cmd that fires fetchProviderModels for
// every provider that has a non-empty key in the keystore. Called from Init() so
// the model selection menus are populated with live (or safe-fallback) models on startup.
func (m *Model) pollAllConfiguredProviders() tea.Cmd {
	pm := providers.GetGlobalProviderManager()
	if pm == nil {
		return nil
	}
	var cmds []tea.Cmd
	for _, p := range providers.AllProviders() {
		key, _ := pm.KeyStore().Get(p)
		if key == "" {
			continue
		}
		provider := p // capture
		cmds = append(cmds, m.fetchProviderModels(provider))
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// switchSecondaryModelByProvider hot-swaps consultant using the provider manager.
func (m *Model) switchSecondaryModelByProvider(providerName, modelID string) error {
	if m.orchestrator == nil {
		return fmt.Errorf("orchestrator not available")
	}
	ctx := context.Background()
	return m.orchestrator.SetSecondary(ctx, providerName, modelID)
}
