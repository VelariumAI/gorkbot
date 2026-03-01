package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines the keybindings for the application
type KeyMap struct {
	// Global
	Quit        key.Binding
	Help        key.Binding

	// Chat
	Submit      key.Binding
	NewLine     key.Binding
	ScrollUp    key.Binding
	ScrollDown  key.Binding
	FocusInput  key.Binding
	ClearScreen key.Binding
	Interrupt   key.Binding // Cancel in-progress generation

	// Model Selection
	SelectModel key.Binding

	// Settings overlay
	ShowSettings key.Binding

	// Mode cycling (Normal → Plan → Auto)
	CycleMode key.Binding

	// Tools View
	ShowTools key.Binding

	// Fold/unfold collapsible frames (internal reasoning, a2a messages)
	FoldFrames key.Binding

	// System Diagnostics view
	ShowDiagnostics key.Binding

	// Conversation Bookmarks overlay
	ShowBookmarks key.Binding

	// List Navigation (for model selection)
	Up     key.Binding
	Down   key.Binding
	Select key.Binding
	Back   key.Binding
}

// DefaultKeyMap returns the default keybindings
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c", "ctrl+q"),
			key.WithHelp("ctrl+c", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys("ctrl+h"),
			key.WithHelp("ctrl+h", "help"),
		),
		Submit: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "send"),
		),
		NewLine: key.NewBinding(
			key.WithKeys("alt+enter"),
			key.WithHelp("alt+enter", "new line"),
		),
		ScrollUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("pgup", "scroll up"),
		),
		ScrollDown: key.NewBinding(
			key.WithKeys("pgdown"),
			key.WithHelp("pgdn", "scroll down"),
		),
		FocusInput: key.NewBinding(
			key.WithKeys("ctrl+i"),
			key.WithHelp("ctrl+i", "focus input"),
		),
		ClearScreen: key.NewBinding(
			key.WithKeys("ctrl+l"),
			key.WithHelp("ctrl+l", "clear"),
		),
		Interrupt: key.NewBinding(
			key.WithKeys("ctrl+x"),
			key.WithHelp("ctrl+x", "interrupt"),
		),
		SelectModel: key.NewBinding(
			key.WithKeys("ctrl+t"),
			key.WithHelp("ctrl+t", "models"),
		),
		ShowSettings: key.NewBinding(
			key.WithKeys("ctrl+g"),
			key.WithHelp("ctrl+g", "settings"),
		),
		CycleMode: key.NewBinding(
			key.WithKeys("ctrl+p"),
			key.WithHelp("ctrl+p", "cycle mode"),
		),
		ShowTools: key.NewBinding(
			key.WithKeys("ctrl+e"),
			key.WithHelp("ctrl+e", "show tools"),
		),
		FoldFrames: key.NewBinding(
			key.WithKeys("ctrl+r"),
			key.WithHelp("ctrl+r", "fold/unfold reasoning"),
		),
		ShowDiagnostics: key.NewBinding(
			key.WithKeys("ctrl+\\"),
			key.WithHelp("ctrl+\\", "diagnostics"),
		),
		ShowBookmarks: key.NewBinding(
			key.WithKeys("ctrl+b"),
			key.WithHelp("ctrl+b", "bookmarks"),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓", "down"),
		),
		Select: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
	}
}

// ShortHelp returns keybindings to be shown in the mini help view
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Quit, k.SelectModel, k.ShowTools, k.ShowSettings, k.CycleMode, k.FoldFrames}
}

// FullHelp returns keybindings for the full help view
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Submit, k.NewLine},
		{k.ScrollUp, k.ScrollDown},
		{k.ClearScreen, k.SelectModel},
		{k.CycleMode, k.Interrupt},
		{k.Quit, k.Help},
	}
}
