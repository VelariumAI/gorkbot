package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

const navRailWidth = 16

// workspaceNavEntry represents a single workspace in the nav rail.
type workspaceNavEntry struct {
	ID      WorkspaceID
	Icon    string
	Label   string
	KeyHint string
}

// workspaceEntries defines the 7 workspace entries displayed in the nav rail.
var workspaceEntries = []workspaceNavEntry{
	{WorkspaceChat, "💬", "Chat", "^1"},
	{WorkspaceTasks, "📋", "Tasks", "^2"},
	{WorkspaceTools, "🔧", "Tools", "^3"},
	{WorkspaceAgents, "🤖", "Agents", "^4"},
	{WorkspaceMemory, "🧠", "Memory", "^5"},
	{WorkspaceAnalytics, "📊", "Analytics", "^6"},
	{WorkspaceSettings, "⚙", "Settings", "^7"},
}

// renderWorkspaceNav returns a fixed 16-char-wide vertical nav rail.
// Active item has an accent border-left; inactive items use dim text.
// TokenStyles are used when available, fallback to hard-coded colors.
func (m *Model) renderWorkspaceNav() string {
	result := ""

	// Get styles
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(TextGray))
	accentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(GrokBlue)).
		Bold(true).
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(GrokBlue))

	for i, entry := range workspaceEntries {
		isActive := m.currentWorkspace == entry.ID

		// Render label with icon
		label := fmt.Sprintf(" %s %s", entry.Icon, entry.Label)
		if len(label) > 14 {
			label = label[:14]
		}

		// Pad to width (15 chars for left border)
		line := lipgloss.NewStyle().Width(15).Render(label)

		// Apply active or inactive style
		if isActive {
			// Active: blue accent, bold, border-left
			line = accentStyle.Render(line)
		} else {
			// Inactive: dim gray
			line = dimStyle.Render(line)
		}

		result += line
		if i < len(workspaceEntries)-1 {
			result += "\n"
		}
	}

	// Wrap nav rail in a box with width constraint
	navStyle := lipgloss.NewStyle().
		Width(navRailWidth).
		BorderRight(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(BorderGray)).
		Padding(1, 0)

	return navStyle.Render(result)
}
