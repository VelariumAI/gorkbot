package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Overlay represents a modal component that sits on top of the main UI.
type Overlay interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (Overlay, tea.Cmd)
	View() string
}

// BaseOverlay is a helper to embed in specific overlays if needed.
type BaseOverlay struct{}

func (b BaseOverlay) Init() tea.Cmd { return nil }
