package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines the keybindings for the application
type KeyMap struct {
	// Global
	Quit key.Binding
	Help key.Binding

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
	CycleMode    key.Binding
	CycleModeTab key.Binding // Shift+Tab alternative

	// Tools View
	ShowTools key.Binding

	// Fold/unfold collapsible frames (internal reasoning, a2a messages)
	FoldFrames key.Binding

	// System Diagnostics view
	ShowDiagnostics key.Binding

	// Conversation Bookmarks overlay
	ShowBookmarks key.Binding

	// Toggle compact tab bar
	ToggleTabs key.Binding

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
		// Tab (0x09) is no longer intercepted by the hotkey manager.
		FocusInput: key.NewBinding(
			key.WithKeys("ctrl+i"),
			key.WithHelp("tab", "focus input"),
		),
		// ctrl+l is intercepted by hotkey (CmdClearReset) — kept here for help display.
		ClearScreen: key.NewBinding(
			key.WithKeys("ctrl+l"),
			key.WithHelp("ctrl+l", "clear history"),
		),
		Interrupt: key.NewBinding(
			key.WithKeys("ctrl+x"),
			key.WithHelp("ctrl+x", "interrupt"),
		),
		// ctrl+g (0x07) is intercepted by the hotkey manager as CmdModelsSelect.
		// Also reachable via the Esc+M leader sequence.
		SelectModel: key.NewBinding(
			key.WithKeys("ctrl+g"),
			key.WithHelp("ctrl+g", "models"),
		),
		// ctrl+s intercepted by hotkey (CmdSettings) — kept for help display only.
		ShowSettings: key.NewBinding(
			key.WithKeys("ctrl+s"),
			key.WithHelp("ctrl+s", "settings"),
		),
		// ctrl+n is NOT intercepted — reaches BubbleTea to cycle execution mode.
		CycleMode: key.NewBinding(
			key.WithKeys("ctrl+n"),
			key.WithHelp("ctrl+n", "cycle mode"),
		),
		CycleModeTab: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "cycle mode"),
		),
		// ctrl+t intercepted by hotkey (CmdToolsMenu) — kept for help display only.
		ShowTools: key.NewBinding(
			key.WithKeys("ctrl+t"),
			key.WithHelp("ctrl+t", "tools"),
		),
		// ctrl+f is NOT intercepted — reaches BubbleTea to fold/unfold reasoning.
		FoldFrames: key.NewBinding(
			key.WithKeys("ctrl+f"),
			key.WithHelp("ctrl+f", "fold reasoning"),
		),
		// ctrl+\ (0x1C) is NOT intercepted — reaches BubbleTea directly.
		ShowDiagnostics: key.NewBinding(
			key.WithKeys("ctrl+\\"),
			key.WithHelp("ctrl+\\", "diagnostics"),
		),
		ShowBookmarks: key.NewBinding(
			key.WithKeys("ctrl+b"),
			key.WithHelp("ctrl+b", "bookmarks"),
		),
		ToggleTabs: key.NewBinding(
			key.WithKeys("ctrl+u"),
			key.WithHelp("ctrl+u", "compact tabs"),
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

// ShortHelp returns keybindings to be shown in the mini help view.
// Only lists keys that actually reach BubbleTea (not intercepted by hotkey manager).
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Submit, k.SelectModel, k.ShowTools, k.ShowSettings, k.CycleMode, k.FoldFrames, k.Quit}
}

// FullHelp returns keybindings for the full help view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Submit, k.NewLine, k.Interrupt},
		{k.ScrollUp, k.ScrollDown, k.FocusInput},
		{k.SelectModel, k.ShowTools, k.ShowSettings},
		{k.CycleMode, k.FoldFrames, k.ShowDiagnostics},
		{k.ShowBookmarks, k.ClearScreen, k.Quit},
	}
}
