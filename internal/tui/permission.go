package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/velariumai/gorkbot/pkg/tools"
)

// PermissionPrompt represents a permission request UI
type PermissionPrompt struct {
	ToolName    string
	Description string
	Parameters  map[string]interface{}
	Selected    int // 0=Always, 1=Session, 2=Once, 3=Never
}

// NewPermissionPrompt creates a new permission prompt
func NewPermissionPrompt(toolName, description string, params map[string]interface{}) *PermissionPrompt {
	return &PermissionPrompt{
		ToolName:    toolName,
		Description: description,
		Parameters:  params,
		Selected:    2, // Default to "Once"
	}
}

// GetPermissionLevel returns the selected permission level
func (p *PermissionPrompt) GetPermissionLevel() tools.PermissionLevel {
	switch p.Selected {
	case 0:
		return tools.PermissionAlways
	case 1:
		return tools.PermissionSession
	case 2:
		return tools.PermissionOnce
	case 3:
		return tools.PermissionNever
	default:
		return tools.PermissionOnce
	}
}

// MoveUp moves selection up
func (p *PermissionPrompt) MoveUp() {
	if p.Selected > 0 {
		p.Selected--
	}
}

// MoveDown moves selection down
func (p *PermissionPrompt) MoveDown() {
	if p.Selected < 3 {
		p.Selected++
	}
}

// Render renders the permission prompt
func (p *PermissionPrompt) Render(width int) string {
	// Box style
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(WarningYellow)).
		Padding(1, 2).
		Width(width - 4)

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(WarningYellow)).
		Bold(true)

	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(GrokBlue)).
		Bold(true).
		Reverse(true)

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(TextGray))

	// Build content
	var content string

	// Title
	content += titleStyle.Render("🔐 Permission Request") + "\n\n"

	// Tool info
	content += fmt.Sprintf("Tool: %s\n", lipgloss.NewStyle().Foreground(lipgloss.Color(TextWhite)).Bold(true).Render(p.ToolName))
	content += fmt.Sprintf("Description: %s\n\n", p.Description)

	// Parameters
	if len(p.Parameters) > 0 {
		content += "Parameters:\n"
		for key, value := range p.Parameters {
			content += fmt.Sprintf("  • %s: %v\n", key, value)
		}
		content += "\n"
	}

	// Question
	content += lipgloss.NewStyle().Foreground(lipgloss.Color(TextWhite)).Render("Allow this tool to execute?") + "\n\n"

	// Options
	options := []string{"Always", "Session", "Once", "Never"}
	descriptions := []string{
		"Grant permanent permission",
		"Allow for this session only",
		"Ask every time (recommended)",
		"Block permanently",
	}

	for i, option := range options {
		var line string
		if i == p.Selected {
			line = selectedStyle.Render(fmt.Sprintf(" ▶ [%s] %s", option, descriptions[i]))
		} else {
			line = normalStyle.Render(fmt.Sprintf("   [%s] %s", option, descriptions[i]))
		}
		content += line + "\n"
	}

	content += "\n"
	content += lipgloss.NewStyle().Foreground(lipgloss.Color(TextGray)).Italic(true).
		Render("Use ↑/↓ to select, Enter to confirm, Esc to deny")

	return boxStyle.Render(content)
}
