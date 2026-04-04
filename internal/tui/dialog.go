package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// DialogStyle holds styling options for dialogs.
type DialogStyle struct {
	Title   lipgloss.Style
	Border  lipgloss.Style
	Content lipgloss.Style
	Footer  lipgloss.Style
}

// DialogConfig specifies dialog appearance and layout.
type DialogConfig struct {
	Title    string       // Title text (optional)
	Width    int          // Desired width; 0 = auto (60% of terminal)
	MinWidth int          // Minimum width (default 40)
	Footer   string       // Keyboard hint line (optional)
	Style    DialogStyle  // Styling (colors, borders)
}

// DialogModel provides a reusable dialog base for modal overlays.
type DialogModel struct {
	cfg DialogConfig
	styles *Styles
}

// NewDialogModel creates a new dialog model with the given config and styles.
func NewDialogModel(cfg DialogConfig, styles *Styles) *DialogModel {
	if cfg.MinWidth == 0 {
		cfg.MinWidth = 40
	}
	if styles == nil {
		// Fallback to a minimal style if none provided
		styles = &Styles{}
	}
	return &DialogModel{
		cfg:    cfg,
		styles: styles,
	}
}

// Wrap renders content inside a titled, bordered dialog box.
// The dialog is left-aligned and does not center on screen.
func (d *DialogModel) Wrap(content string, termW int) string {
	width := d.cfg.Width
	if width == 0 {
		// Auto: 60% of terminal width
		width = (termW * 60) / 100
	}
	if width < d.cfg.MinWidth {
		width = d.cfg.MinWidth
	}
	if width > termW-4 {
		width = termW - 4
	}

	// Build the dialog
	var result string

	// Use default styles if none provided
	titleStyle := d.cfg.Style.Title
	if titleStyle.String() == "" {
		titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	}

	borderStyle := d.cfg.Style.Border
	if borderStyle.String() == "" {
		borderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	}

	contentStyle := d.cfg.Style.Content
	if contentStyle.String() == "" {
		contentStyle = lipgloss.NewStyle()
	}

	// Title bar
	if d.cfg.Title != "" {
		result += titleStyle.Render(d.cfg.Title) + "\n"
	}

	// Top border
	result += borderStyle.Render("┌" + repeatStr("─", width-2) + "┐") + "\n"

	// Content with left/right borders
	contentLines := splitLines(content)
	for _, line := range contentLines {
		// Pad line to dialog width (accounting for borders)
		padded := padStr(line, width-4)
		result += borderStyle.Render("│ ") + contentStyle.Render(padded) + borderStyle.Render(" │") + "\n"
	}

	// Bottom border
	result += borderStyle.Render("└" + repeatStr("─", width-2) + "┘")

	// Footer if provided
	if d.cfg.Footer != "" {
		footerStyle := d.cfg.Style.Footer
		if footerStyle.String() == "" {
			footerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
		}
		result += "\n" + footerStyle.Render(d.cfg.Footer)
	}

	return result
}

// Center renders the dialog centered on screen with a backdrop.
// Uses '░' (light shade) as backdrop character for visual separation.
func (d *DialogModel) Center(content string, termW, termH int) string {
	width := d.cfg.Width
	if width == 0 {
		width = (termW * 60) / 100
	}
	if width < d.cfg.MinWidth {
		width = d.cfg.MinWidth
	}
	if width > termW-4 {
		width = termW - 4
	}

	// Build dialog content
	dialogContent := d.Wrap(content, termW)
	dialogLines := splitLines(dialogContent)

	// Center vertically
	dialogHeight := len(dialogLines) + 1 // +1 for spacing
	topPadding := (termH - dialogHeight) / 2
	if topPadding < 0 {
		topPadding = 0
	}

	var result string

	// Top padding
	for i := 0; i < topPadding; i++ {
		result += "\n"
	}

	// Centered dialog with backdrop
	for _, line := range dialogLines {
		// Pad line to terminal width and center
		placed := lipgloss.NewStyle().
			Width(termW).
			AlignHorizontal(lipgloss.Center).
			Render(line)
		result += placed + "\n"
	}

	return result
}

// Helper functions

// repeatStr returns a string repeated n times.
func repeatStr(s string, n int) string {
	if n <= 0 {
		return ""
	}
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}

// padStr pads a string to the right with spaces to the given width.
func padStr(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	return s + repeatStr(" ", width-len(s))
}

// splitLines splits a string by newlines.
func splitLines(s string) []string {
	if s == "" {
		return []string{""}
	}
	var result []string
	var current string
	for _, ch := range s {
		if ch == '\n' {
			result = append(result, current)
			current = ""
		} else {
			current += string(ch)
		}
	}
	if current != "" || s[len(s)-1] == '\n' {
		result = append(result, current)
	}
	return result
}
