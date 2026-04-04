package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/velariumai/gorkbot/internal/engine"
	"github.com/velariumai/gorkbot/pkg/hitl"
)

// HITLOverlay renders a rich HITL approval dialog with risk/confidence visualization.
type HITLOverlay struct {
	Request     engine.HITLRequest
	SelectedIdx int // For keyboard navigation (Approve=0, Reject=1)
	IsVisible   bool
}

// NewHITLOverlay creates a new HITL overlay for the given request.
func NewHITLOverlay(req engine.HITLRequest) *HITLOverlay {
	return &HITLOverlay{
		Request:     req,
		SelectedIdx: 0, // Default to Approve
		IsVisible:   true,
	}
}

// Render generates the complete overlay UI.
func (h *HITLOverlay) Render(width int) string {
	var sb strings.Builder

	// Title bar
	sb.WriteString(h.renderTitle())
	sb.WriteString("\n\n")

	// Risk & Confidence metrics
	sb.WriteString(h.renderMetrics(width))
	sb.WriteString("\n\n")

	// Tool details
	sb.WriteString(h.renderToolDetails())
	sb.WriteString("\n\n")

	// Execution plan
	sb.WriteString(h.renderPlan(width))
	sb.WriteString("\n\n")

	// Action buttons
	sb.WriteString(h.renderButtons())

	// Instructions
	sb.WriteString("\n\n")
	sb.WriteString(h.renderInstructions())

	return sb.String()
}

// renderTitle creates the dialog title with risk indicator.
func (h *HITLOverlay) renderTitle() string {
	riskSymbol := h.Request.RiskLevel.Symbol()

	title := fmt.Sprintf("%s HUMAN-IN-THE-LOOP APPROVAL REQUIRED", riskSymbol)

	return lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")).
		Render(title)
}

// renderMetrics displays risk and confidence indicators side by side.
func (h *HITLOverlay) renderMetrics(width int) string {
	const metricWidth = 40

	// Risk section
	riskBar := h.renderRiskBar()
	riskText := fmt.Sprintf("Risk: %s (%s)", h.Request.RiskLevel.String(), h.Request.RiskReason)

	// Confidence section
	confidenceBar := h.renderConfidenceBar()
	confidenceText := fmt.Sprintf("Confidence: %d%% | Precedent: %d similar approvals",
		h.Request.ConfidenceScore, h.Request.Precedent)

	riskSection := lipgloss.JoinVertical(lipgloss.Left,
		riskBar,
		riskText,
	)

	confSection := lipgloss.JoinVertical(lipgloss.Left,
		confidenceBar,
		confidenceText,
	)

	return lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(metricWidth).Render(riskSection),
		"  ",
		lipgloss.NewStyle().Width(metricWidth).Render(confSection),
	)
}

// renderRiskBar shows a visual representation of risk level.
func (h *HITLOverlay) renderRiskBar() string {
	const barLength = 30
	filledChars := map[hitl.RiskLevel]int{
		hitl.RiskLow:      5,
		hitl.RiskMedium:   15,
		hitl.RiskHigh:     22,
		hitl.RiskCritical: 30,
	}

	filled := filledChars[h.Request.RiskLevel]
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barLength-filled)

	color := h.Request.RiskLevel.Color()
	return fmt.Sprintf("%s%s\033[0m", color, bar)
}

// renderConfidenceBar shows AI confidence as a progress bar.
func (h *HITLOverlay) renderConfidenceBar() string {
	const barLength = 30
	filled := (h.Request.ConfidenceScore * barLength) / 100
	if filled > barLength {
		filled = barLength
	}

	var color string
	if h.Request.ConfidenceScore >= 85 {
		color = "\033[32m" // Green
	} else if h.Request.ConfidenceScore >= 70 {
		color = "\033[36m" // Cyan
	} else if h.Request.ConfidenceScore >= 50 {
		color = "\033[33m" // Yellow
	} else {
		color = "\033[31m" // Red
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", barLength-filled)
	return fmt.Sprintf("%s%s\033[0m", color, bar)
}

// renderToolDetails shows the tool name and context.
func (h *HITLOverlay) renderToolDetails() string {
	var sb strings.Builder

	toolStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	sb.WriteString("Tool: ")
	sb.WriteString(toolStyle.Render(h.Request.ToolName))
	sb.WriteString("\n\n")

	if h.Request.Context != "" {
		sb.WriteString("Context: ")
		sb.WriteString(h.Request.Context)
		sb.WriteString("\n")
	}

	return sb.String()
}

// renderPlan shows the execution plan.
func (h *HITLOverlay) renderPlan(width int) string {
	planBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("8")).
		Padding(1).
		Width(width - 4).
		Render(h.Request.Plan)

	return planBox
}

// renderButtons creates the approve/reject button UI.
func (h *HITLOverlay) renderButtons() string {
	approveStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("2")).
		Foreground(lipgloss.Color("15")).
		Padding(0, 2).
		Bold(true)

	rejectStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("1")).
		Foreground(lipgloss.Color("15")).
		Padding(0, 2).
		Bold(true)

	if h.SelectedIdx == 0 {
		approveStyle = approveStyle.Underline(true)
	} else {
		rejectStyle = rejectStyle.Underline(true)
	}

	approve := approveStyle.Render("[ Approve ]")
	reject := rejectStyle.Render("[ Reject ]")

	return lipgloss.JoinHorizontal(lipgloss.Center, approve, "    ", reject)
}

// renderInstructions shows keyboard shortcuts.
func (h *HITLOverlay) renderInstructions() string {
	instrStyle := lipgloss.NewStyle().
		Faint(true).
		Italic(true).
		Foreground(lipgloss.Color("8"))

	return instrStyle.Render("Use [Tab] or [←→] to toggle choice, [Enter] to submit, [Esc] to cancel")
}

// HandleKeypress processes keyboard input for the overlay.
// Returns (approved, shouldClose) booleans.
func (h *HITLOverlay) HandleKeypress(key string) (approved bool, shouldClose bool) {
	switch key {
	case "tab", "right", "j":
		h.SelectedIdx = (h.SelectedIdx + 1) % 2
		return false, false

	case "left", "k", "shift+tab":
		h.SelectedIdx = (h.SelectedIdx - 1 + 2) % 2
		return false, false

	case "enter":
		approved := h.SelectedIdx == 0
		return approved, true

	case "esc":
		return false, true

	default:
		return false, false
	}
}
