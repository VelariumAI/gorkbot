package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/velariumai/gorkbot/pkg/tools"
)

// Color constants for tool categories (mirrored from style.go to avoid circular imports)
const (
	toolCyanColor   = "#8BE9FD" // File operations
	toolOrangeColor = "#FFB86C" // Bash/shell
	toolPinkColor   = "#FF79C6" // Git operations
	toolPurpleColor = "#BD93F9" // Web operations
	toolRedColor    = "#FF5555" // Security tools
	toolBlueColor   = "#00D9FF" // Agent/system tools
	toolYellowColor = "#F1FA8C" // Hashed/special
	toolGreenColor  = "#50FA7B" // Default
)

// liveBoxState represents the current expansion state of a tool box
type liveBoxState int

const (
	liveBoxCompact  liveBoxState = iota // 1 line
	liveBoxExpanded                     // 5 lines
	liveBoxFull                         // 15 lines
)

// LiveToolBox tracks one in-flight or recently-completed tool
type LiveToolBox struct {
	name        string
	params      map[string]interface{}
	elapsed     time.Duration
	done        bool
	success     bool
	result      *tools.ToolResult
	startTime   time.Time
	state       liveBoxState
	icon        string // ⟳ running, ✓ success, ✗ failed
	accentColor string
	completedAt time.Time
}

// LiveToolsPanel manages the live display of tools running in parallel
type LiveToolsPanel struct {
	boxes        []*LiveToolBox
	focusIdx     int // -1 = none
	width        int
	totalElapsed time.Duration
	sessionStart time.Time
	stats        interface{} // *tools.ExecutionStats (avoid circular import)
}

// NewLiveToolsPanel creates a new empty live panel
func NewLiveToolsPanel() *LiveToolsPanel {
	return &LiveToolsPanel{
		boxes:    make([]*LiveToolBox, 0),
		focusIdx: -1,
		width:    80,
	}
}

// SetStats sets the ExecutionStats for progress estimation
func (p *LiveToolsPanel) SetStats(stats interface{}) {
	p.stats = stats
}

// StartTool adds a new tool to the panel
func (p *LiveToolsPanel) StartTool(name string, params map[string]interface{}) {
	// Check if tool already exists (shouldn't happen in normal flow)
	for _, box := range p.boxes {
		if box.name == name && !box.done {
			return
		}
	}

	box := &LiveToolBox{
		name:        name,
		params:      params,
		startTime:   time.Now(),
		done:        false,
		state:       liveBoxCompact,
		icon:        "⟳",
		accentColor: computeToolColor(name),
	}
	p.boxes = append(p.boxes, box)
}

// UpdateElapsed updates the elapsed time for a running tool
func (p *LiveToolsPanel) UpdateElapsed(name string, elapsed time.Duration) {
	for _, box := range p.boxes {
		if box.name == name && !box.done {
			box.elapsed = elapsed
			return
		}
	}
}

// CompleteTool marks a tool as done and stores its result
func (p *LiveToolsPanel) CompleteTool(name string, result *tools.ToolResult, elapsed time.Duration) {
	for _, box := range p.boxes {
		if box.name == name {
			box.elapsed = elapsed
			box.done = true
			box.success = result != nil && result.Success
			box.result = result
			box.completedAt = time.Now()
			if box.success {
				box.icon = "✓"
			} else {
				box.icon = "✗"
			}
			return
		}
	}
}

// Clear removes all boxes from the panel
func (p *LiveToolsPanel) Clear() {
	p.boxes = p.boxes[:0]
	p.focusIdx = -1
}

// ActiveCount returns the number of currently running tools
func (p *LiveToolsPanel) ActiveCount() int {
	count := 0
	for _, box := range p.boxes {
		if !box.done {
			count++
		}
	}
	return count
}

// HasActivity returns true if there are active tools or recently completed ones (< 3s)
func (p *LiveToolsPanel) HasActivity() bool {
	for _, box := range p.boxes {
		if !box.done {
			return true
		}
		if !box.completedAt.IsZero() && time.Since(box.completedAt) < 3*time.Second {
			return true
		}
	}
	return false
}

// Height returns the total height needed to render the panel
func (p *LiveToolsPanel) Height() int {
	if !p.HasActivity() {
		return 0
	}

	// Header (1) + boxes + spacing
	height := 1
	for _, box := range p.boxes {
		if time.Since(box.completedAt) > 3*time.Second && box.done {
			continue // Don't count faded-out boxes
		}
		height += p.boxHeight(box) + 1 // +1 for spacing
	}
	return height + 1 // +1 for bottom border
}

// boxHeight returns the height of a single tool box
func (p *LiveToolsPanel) boxHeight(box *LiveToolBox) int {
	switch box.state {
	case liveBoxCompact:
		return 1
	case liveBoxExpanded:
		return 5
	case liveBoxFull:
		return 15
	default:
		return 1
	}
}

// FocusNext moves focus to the next visible box
func (p *LiveToolsPanel) FocusNext() {
	if len(p.boxes) == 0 {
		p.focusIdx = -1
		return
	}
	p.focusIdx++
	if p.focusIdx >= len(p.boxes) {
		p.focusIdx = 0
	}
}

// FocusPrev moves focus to the previous visible box
func (p *LiveToolsPanel) FocusPrev() {
	if len(p.boxes) == 0 {
		p.focusIdx = -1
		return
	}
	p.focusIdx--
	if p.focusIdx < 0 {
		p.focusIdx = len(p.boxes) - 1
	}
}

// ExpandFocused cycles the focused box through expansion states
func (p *LiveToolsPanel) ExpandFocused() {
	if p.focusIdx < 0 || p.focusIdx >= len(p.boxes) {
		return
	}
	box := p.boxes[p.focusIdx]
	switch box.state {
	case liveBoxCompact:
		box.state = liveBoxExpanded
	case liveBoxExpanded:
		box.state = liveBoxFull
	case liveBoxFull:
		box.state = liveBoxCompact
	}
}

// CollapseFocused collapses the focused box to compact view
func (p *LiveToolsPanel) CollapseFocused() {
	if p.focusIdx < 0 || p.focusIdx >= len(p.boxes) {
		return
	}
	p.boxes[p.focusIdx].state = liveBoxCompact
}

// View renders the live tools panel
func (p *LiveToolsPanel) View(styles *Styles) string {
	if !p.HasActivity() {
		return ""
	}

	activeCount := p.ActiveCount()
	var lines []string

	// Header line
	headerText := fmt.Sprintf("GORKY ▸ %d tools running", activeCount)
	if p.totalElapsed > 0 {
		headerText += fmt.Sprintf(" · %.1fs", p.totalElapsed.Seconds())
	}
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(GrokBlue)).
		Bold(true)
	lines = append(lines, " "+headerStyle.Render(headerText))

	// Render each box
	for i, box := range p.boxes {
		if time.Since(box.completedAt) > 3*time.Second && box.done {
			continue // Skip faded-out boxes
		}

		isFocused := (i == p.focusIdx)
		line := p.renderBox(box, styles, isFocused)
		lines = append(lines, line)
	}

	// Assemble with border
	content := strings.Join(lines, "\n")
	border := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(GrokBlue)).
		Padding(0, 1)

	return border.Render(content)
}

// renderBox renders a single tool box
func (p *LiveToolsPanel) renderBox(box *LiveToolBox, styles *Styles, focused bool) string {
	switch box.state {
	case liveBoxCompact:
		return p.renderCompactBox(box, focused)
	case liveBoxExpanded:
		return p.renderExpandedBox(box, focused)
	case liveBoxFull:
		return p.renderFullBox(box, focused)
	default:
		return p.renderCompactBox(box, focused)
	}
}

// renderCompactBox renders a single-line tool box
func (p *LiveToolsPanel) renderCompactBox(box *LiveToolBox, focused bool) string {
	// Format: ⟳ tool_name   param_preview  [progress]  elapsed
	icon := box.icon
	if !box.done {
		// Animate the spinner
		icon = "⟳"
	}

	// Color the icon and name by category
	colorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(box.accentColor))
	coloredIcon := colorStyle.Render(icon)
	coloredName := colorStyle.Render(padRight(box.name, 12))
	name := coloredName

	// Build param preview
	paramPreview := ""
	if len(box.params) > 0 {
		for k, v := range box.params {
			paramPreview = fmt.Sprintf("%s=%v", k, v)
			break
		}
	}

	// Truncate to available width
	maxParamWidth := p.width - 40
	if maxParamWidth < 10 {
		maxParamWidth = 10
	}
	if len(paramPreview) > maxParamWidth {
		paramPreview = paramPreview[:maxParamWidth-3] + "..."
	}
	paramPreview = padRight(paramPreview, maxParamWidth)

	// Format elapsed
	elapsedStr := fmt.Sprintf("%.1fs", box.elapsed.Seconds())
	elapsedStr = strings.TrimPrefix(elapsedStr, "0")

	// Build progress bar section if width allows
	var progressBar string
	if p.width > 80 && !box.done {
		// Show progress bar on wider terminals
		progress := p.estimateProgress(box.name, box.elapsed)
		if progress > 0 {
			progressBar = " " + p.renderProgressBar(progress)
		}
	}

	line := fmt.Sprintf("  %s %s %s%s %s", coloredIcon, name, paramPreview, progressBar, elapsedStr)

	if focused {
		line = lipgloss.NewStyle().
			Background(lipgloss.Color(BorderGray)).
			Foreground(lipgloss.Color(TextWhite)).
			Render(line)
	}

	return line
}

// estimateProgress estimates progress based on historical execution data
func (p *LiveToolsPanel) estimateProgress(toolName string, elapsed time.Duration) float64 {
	if p.stats == nil {
		return 0
	}
	// Type-assert to ExecutionStats without importing tools package
	// This is a safe workaround to avoid circular imports
	statsVal := fmt.Sprintf("%v", p.stats)
	if statsVal == "<nil>" {
		return 0
	}
	// For now, just return 0 - full integration requires reflection or interface{}
	// This is ready for future enhancement
	return 0
}

// renderProgressBar renders a simple 12-block progress bar
func (p *LiveToolsPanel) renderProgressBar(progress float64) string {
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}

	const barWidth = 12
	filled := int(progress * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}

	// Use Unicode box-drawing characters for progress bar
	filledChar := "█"
	emptyChar := "░"

	bar := strings.Repeat(filledChar, filled) + strings.Repeat(emptyChar, barWidth-filled)
	return fmt.Sprintf("%s %.0f%%", bar, progress*100)
}

// renderExpandedBox renders a 5-line tool box with more details
func (p *LiveToolsPanel) renderExpandedBox(box *LiveToolBox, focused bool) string {
	colorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(box.accentColor))
	coloredIcon := colorStyle.Render(box.icon)
	coloredName := colorStyle.Render(box.name)
	lines := []string{
		"╭─ " + coloredIcon + " " + coloredName + " " + strings.Repeat("─", max(0, p.width-20)) + " " + fmt.Sprintf("%.1fs", box.elapsed.Seconds()) + " ─╮",
	}

	// Add parameter display
	if len(box.params) > 0 {
		for k, v := range box.params {
			paramLine := fmt.Sprintf("│  %s: %v", k, v)
			if len(paramLine) > p.width {
				paramLine = paramLine[:p.width-3] + "..."
			}
			paramLine += strings.Repeat(" ", max(0, p.width-len(paramLine)))
			paramLine += "│"
			lines = append(lines, paramLine)
		}
	}

	// Status line
	if box.done {
		status := "✓ Complete"
		if !box.success {
			status = "✗ Failed"
		}
		lines = append(lines, "│  Status: "+padRight(status, p.width-12)+"│")
	} else {
		lines = append(lines, "│  Status: "+padRight("Running...", p.width-12)+"│")
	}

	lines = append(lines, "│                                                                            │")
	lines = append(lines, "│  [tab] next  [ctrl+o] collapse  [ctrl+e] full view                        │")
	lines = append(lines, "╰"+strings.Repeat("─", p.width-2)+"╯")

	return strings.Join(lines, "\n")
}

// renderFullBox renders a 15-line tool box with output preview
func (p *LiveToolsPanel) renderFullBox(box *LiveToolBox, focused bool) string {
	colorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(box.accentColor))
	coloredIcon := colorStyle.Render(box.icon)
	coloredName := colorStyle.Render(box.name)
	lines := []string{
		"╭─ " + coloredIcon + " " + coloredName + " (full view) " + strings.Repeat("─", max(0, p.width-30)) + " ─╮",
	}

	// Show first 12 lines of output
	if box.result != nil && box.result.Output != "" {
		outputLines := strings.Split(box.result.Output, "\n")
		for i := 0; i < minPanel(12, len(outputLines)); i++ {
			line := outputLines[i]
			if len(line) > p.width-4 {
				line = line[:p.width-7] + "..."
			}
			lines = append(lines, "│  "+padRight(line, p.width-4)+"│")
		}
		if len(outputLines) > 12 {
			remaining := len(outputLines) - 12
			lines = append(lines, fmt.Sprintf("│  ↓ %d more lines...", remaining)+strings.Repeat(" ", max(0, p.width-20))+"│")
		}
	} else if box.done {
		lines = append(lines, "│  (no output)"+strings.Repeat(" ", max(0, p.width-12))+"│")
	} else {
		lines = append(lines, "│  (running...)"+strings.Repeat(" ", max(0, p.width-13))+"│")
	}

	// Padding
	for len(lines) < 13 {
		lines = append(lines, "│"+strings.Repeat(" ", p.width-2)+"│")
	}

	lines = append(lines, "│  [tab] next  [ctrl+o] collapse to compact                               │")
	lines = append(lines, "╰"+strings.Repeat("─", p.width-2)+"╯")

	return strings.Join(lines, "\n")
}

// Helper functions
func padRight(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

func minPanel(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// computeToolColor returns a color hex string based on the tool category
func computeToolColor(toolName string) string {
	switch {
	case toolName == "bash":
		return toolOrangeColor
	case strings.HasPrefix(toolName, "read_") || strings.HasPrefix(toolName, "write_") ||
		strings.HasPrefix(toolName, "list_") || strings.HasPrefix(toolName, "grep_") ||
		strings.HasPrefix(toolName, "file_") || strings.HasPrefix(toolName, "delete_"):
		return toolCyanColor
	case strings.HasPrefix(toolName, "git_"):
		return toolPinkColor
	case strings.HasPrefix(toolName, "web_") || strings.HasPrefix(toolName, "http_") ||
		strings.HasPrefix(toolName, "browser_"):
		return toolPurpleColor
	case strings.HasPrefix(toolName, "nmap") || strings.HasPrefix(toolName, "sqlmap") ||
		strings.HasPrefix(toolName, "nuclei"):
		return toolRedColor
	case toolName == "spawn_agent" || toolName == "collect_agent" || toolName == "list_agents":
		return toolBlueColor
	case strings.HasSuffix(toolName, "_hashed"):
		return toolYellowColor
	default:
		return toolGreenColor
	}
}
