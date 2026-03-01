package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/commands"
	"github.com/velariumai/gorkbot/pkg/providers"
)

// renderAPIKeyPrompt renders the API key entry modal overlay.
// It appears over any view — rendered as a full-screen overlay in view.go.
func (m *Model) renderAPIKeyPrompt() string {
	p := &m.apiKeyPrompt
	if !p.active {
		return ""
	}

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("213")).
		Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	urlStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	inputStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("99")).
		Padding(0, 1).
		Width(44)
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("76"))

	providerDisplay := providers.ProviderName(p.provider)
	website := p.websiteURL

	var lines []string
	lines = append(lines, titleStyle.Render("API Key Required — "+providerDisplay))
	lines = append(lines, "")
	lines = append(lines, labelStyle.Render("Get your key at: ")+urlStyle.Render("https://"+website))
	lines = append(lines, "")

	if p.validating {
		// Show a validating state instead of the input
		lines = append(lines, okStyle.Render("Validating key, please wait..."))
		lines = append(lines, "")
		lines = append(lines, helpStyle.Render("[Esc]=Cancel"))
	} else {
		// Show the input field
		displayVal := p.inputVal
		if len(displayVal) > 42 {
			displayVal = "..." + displayVal[len(displayVal)-39:]
		}
		cursor := lipgloss.NewStyle().Background(lipgloss.Color("255")).Foreground(lipgloss.Color("0")).Render(" ")
		inputDisplay := displayVal + cursor

		lines = append(lines, labelStyle.Render("Paste or type your API key:"))
		lines = append(lines, inputStyle.Render(inputDisplay))
		lines = append(lines, "")
		lines = append(lines, helpStyle.Render("Ctrl+V / long-press to paste  •  Enter=Validate  •  Esc=Cancel"))

		if p.errMsg != "" {
			lines = append(lines, "")
			lines = append(lines, errStyle.Render("✗  "+p.errMsg))
			lines = append(lines, errStyle.Render("   Check the key and try again."))
		}
	}

	content := strings.Join(lines, "\n")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("99")).
		Padding(1, 3)

	box := boxStyle.Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceChars("░"),
		lipgloss.WithWhitespaceForeground(lipgloss.Color("235")))
}

// updateAPIKeyPrompt handles key input for the API key modal.
// Called from both top-level Update() and updateModelSelectView() when prompt is active.
func (m *Model) updateAPIKeyPrompt(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		// Non-key messages (window resize, ticks) pass through without consuming.
		return m, nil
	}

	p := &m.apiKeyPrompt

	// While validating, only allow Esc to cancel.
	if p.validating {
		if keyMsg.Type == tea.KeyEsc {
			p.active = false
			p.validating = false
			p.inputVal = ""
			p.errMsg = ""
		}
		return m, nil
	}

	switch keyMsg.Type {
	case tea.KeyEsc:
		p.active = false
		p.inputVal = ""
		p.errMsg = ""
		return m, nil

	case tea.KeyEnter:
		trimmed := strings.TrimSpace(p.inputVal)
		if trimmed == "" {
			p.errMsg = "Key cannot be empty."
			return m, nil
		}
		// Enter validating state — keep prompt open so user sees progress.
		key := trimmed
		provider := p.provider
		p.validating = true
		p.errMsg = ""
		return m, func() tea.Msg {
			pm := providers.GetGlobalProviderManager()
			if pm == nil {
				return APIKeySavedMsg{Provider: provider, Valid: false, ErrMsg: "provider manager unavailable"}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := pm.SetKey(ctx, provider, key, true)
			if err != nil {
				return APIKeySavedMsg{Provider: provider, Valid: false, ErrMsg: err.Error()}
			}
			return APIKeySavedMsg{Provider: provider, Valid: true}
		}

	case tea.KeyBackspace, tea.KeyDelete:
		if len(p.inputVal) > 0 {
			// Remove last rune (handles multi-byte)
			runes := []rune(p.inputVal)
			p.inputVal = string(runes[:len(runes)-1])
		}
		p.errMsg = ""
		return m, nil

	case tea.KeyRunes:
		p.inputVal += string(keyMsg.Runes)
		p.errMsg = ""
		return m, nil

	case tea.KeyCtrlU:
		// Clear entire input (common terminal shortcut)
		p.inputVal = ""
		p.errMsg = ""
		return m, nil
	}

	return m, nil
}

// handleAPIKeySaved processes the result of a key validation attempt.
//   - On success: closes the prompt, refreshes provider statuses, triggers model fetch.
//   - On failure: keeps the prompt OPEN with an inline error message (no lost feedback).
func (m *Model) handleAPIKeySaved(msg APIKeySavedMsg) (tea.Model, tea.Cmd) {
	p := &m.apiKeyPrompt
	p.validating = false

	if msg.Valid {
		// Close prompt cleanly.
		p.active = false
		p.inputVal = ""
		p.errMsg = ""

		m.addSystemMessage(fmt.Sprintf(
			"**%s** API key verified and saved.",
			providers.ProviderName(msg.Provider),
		))
		// Refresh provider status indicators.
		m.updateProviderKeyStatuses()
		// Trigger background model fetch for the newly available provider.
		return m, m.fetchProviderModels(msg.Provider)
	}

	// Failure — keep prompt open so user can correct the key.
	// Condense the error to the most useful part.
	errMsg := msg.ErrMsg
	if len(errMsg) > 120 {
		errMsg = errMsg[:120] + "..."
	}
	p.errMsg = errMsg
	// Re-open prompt if it was somehow closed.
	p.active = true
	return m, nil
}

// fetchProviderModels fires a background Cmd that polls one provider for its
// live model list and returns a ModelRefreshMsg.
//
// Timeout: 12 s total (each provider's FetchModels also enforces 10 s internally).
// If the poll fails or returns empty, safe static models from ai.SafeModelDefs
// are used as a fallback so the list is never blank after a key is saved.
func (m *Model) fetchProviderModels(provider string) tea.Cmd {
	return func() tea.Msg {
		pm := providers.GetGlobalProviderManager()

		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()

		var infos []commands.ModelInfo

		if pm != nil {
			defs, err := pm.PollProvider(ctx, provider)
			if err == nil && len(defs) > 0 {
				for _, d := range defs {
					infos = append(infos, commands.ModelInfo{
						ID:       string(d.ID),
						Name:     d.Name,
						Provider: string(d.Provider),
						Thinking: d.Capabilities.SupportsThinking,
					})
				}
			}
		}

		// Always ensure a non-empty list by falling back to safe statics.
		if len(infos) == 0 {
			for _, d := range ai.SafeModelDefs(provider) {
				infos = append(infos, commands.ModelInfo{
					ID:       string(d.ID),
					Name:     d.Name,
					Provider: string(d.Provider),
					Thinking: d.Capabilities.SupportsThinking,
				})
			}
		}

		if len(infos) == 0 {
			return nil
		}
		return ModelRefreshMsg{Provider: provider, Models: infos}
	}
}

// updateProviderKeyStatuses refreshes the provider key status list in modelSelectState.
func (m *Model) updateProviderKeyStatuses() {
	pm := providers.GetGlobalProviderManager()
	if pm == nil {
		return
	}
	statuses := pm.KeyStore().StatusLine()
	m.modelSelect.providerKeys = make([]providerStatus, len(statuses))
	for i, s := range statuses {
		m.modelSelect.providerKeys[i] = providerStatus{
			Provider: s.Provider,
			Status:   int(s.Status),
		}
	}
}
