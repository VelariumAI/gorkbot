package tui

import (
	"fmt"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// modelItem represents a model in the selection list
type modelItem struct {
	id          string
	name        string
	provider    string
	description string
	thinking    bool
	// v2.7.0 dual-pane fields
	isAuto bool // special "Auto" entry for secondary pane
	active bool // currently selected/active
}

func (i modelItem) Title() string {
	if i.isAuto {
		suffix := ""
		if i.active {
			suffix = " ✓ ACTIVE"
		}
		return "[Auto] — AI picks best model per task" + suffix
	}
	icon := ""
	if i.thinking {
		icon = " 🧠"
	}
	suffix := ""
	if i.active {
		suffix = " ✓"
	}
	return fmt.Sprintf("%s%s%s", i.name, icon, suffix)
}

func (i modelItem) Description() string {
	if i.isAuto {
		return ""
	}
	return fmt.Sprintf("%s • %s", i.provider, i.id)
}

func (i modelItem) FilterValue() string { return i.name + i.provider + i.id }

// initModelList creates a new list model for model selection
func initModelList(width, height int) list.Model {
	l := list.New([]list.Item{}, list.NewDefaultDelegate(), width, height)
	l.Title = "Select Model"
	l.SetShowHelp(true)
	l.SetFilteringEnabled(true)
	l.Styles.Title = list.DefaultStyles().Title.Background(lipgloss.Color("62")).Foreground(lipgloss.Color("230"))
	return l
}

// updateModelListItems updates the items in the model list from the stored available models.
// Also refreshes the dual-pane model select lists if initialised.
func (m *Model) updateModelListItems() tea.Cmd {
	if len(m.availableModels) == 0 {
		return nil
	}

	items := make([]list.Item, len(m.availableModels))
	primaryID := m.currentModel
	for i, model := range m.availableModels {
		items[i] = modelItem{
			id:          model.ID,
			name:        model.Name,
			provider:    model.Provider,
			description: model.ID,
			thinking:    model.Thinking,
			active:      model.ID == primaryID,
		}
	}

	cmd := m.modelList.SetItems(items)
	// Also refresh dual-pane lists if initialised (v2.7.0)
	if m.modelSelect.refreshing != nil {
		m.refreshModelSelectLists()
	}
	return cmd
}

// getSelectedModel returns the selected model item
func (m *Model) getSelectedModel() *modelItem {
	if i, ok := m.modelList.SelectedItem().(modelItem); ok {
		return &i
	}
	return nil
}
