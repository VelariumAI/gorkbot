package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/velariumai/gorkbot/pkg/discovery"
)

// ─── styles (local to this file) ─────────────────────────────────────────────

var (
	discHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39")). // bright cyan
			MarginBottom(1)

	discProviderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("99")) // soft purple

	discModelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	discCapStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")). // amber
			Italic(true)

	discEmptyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Italic(true)

	treeRunningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("33"))   // blue
	treeDoneStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))   // green
	treeFailStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))  // red
	treeVerifyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))  // yellow
	treeDepthStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))  // dim

	splitDivider = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))
)

// renderDiscoveryView renders the "Cloud Brains" tab: discovered models on the
// left and the active delegation tree on the right.
func (m *Model) renderDiscoveryView() string {
	halfW := m.width / 2

	left := m.renderModelSidebar(halfW - 2)
	right := m.renderAgentTree(m.width - halfW - 2)

	divider := splitDivider.Render(strings.Repeat("│\n", 30))

	row := lipgloss.JoinHorizontal(
		lipgloss.Top,
		lipgloss.NewStyle().Width(halfW-1).Render(left),
		divider,
		lipgloss.NewStyle().Width(m.width-halfW-1).Render(right),
	)
	return lipgloss.NewStyle().
		Height(m.height - 6). // leave room for header, tabs, status bar
		Render(row)
}

// renderModelSidebar renders the left panel listing all discovered cloud models.
// The model at m.discCursor is highlighted.
func (m *Model) renderModelSidebar(width int) string {
	var b strings.Builder
	b.WriteString(discHeaderStyle.Render("Cloud Brains  [↑↓ navigate | Enter=primary | s=secondary | t=test]"))
	b.WriteString("\n")

	if len(m.discoveredModels) == 0 {
		b.WriteString(discEmptyStyle.Render("  Polling APIs… (updates every 30 min)"))
		return lipgloss.NewStyle().Width(width).Render(b.String())
	}

	// Render all models with cursor indicator
	for i, mod := range m.discoveredModels {
		selected := i == m.discCursor
		b.WriteString(renderModelRowSelected(mod, selected))
		b.WriteString("\n")
	}

	// Show total count and hint.
	total := len(m.discoveredModels)
	footer := fmt.Sprintf("\n  %d models discovered", total)
	b.WriteString(discEmptyStyle.Render(footer))

	// Show test result overlay if active
	if m.discTestActive {
		b.WriteString("\n\n")
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214")).Render("Test prompt: "))
		b.WriteString(m.discTestInput + "█")
		if m.discTestResult != "" {
			b.WriteString("\n\n")
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Render(m.discTestResult))
		}
	}

	return lipgloss.NewStyle().Width(width).Render(b.String())
}

func renderModelRow(mod discoveryModel) string {
	return renderModelRowSelected(mod, false)
}

func renderModelRowSelected(mod discoveryModel, selected bool) string {
	icon := capIcon(mod.BestCap)
	name := mod.Name
	if name == "" {
		name = mod.ID
	}
	capLabel := discCapStyle.Render(fmt.Sprintf("[%s %s]", icon, mod.BestCap))
	row := "  " + discModelStyle.Render(name) + " " + capLabel
	if selected {
		row = lipgloss.NewStyle().
			Background(lipgloss.Color("57")).
			Foreground(lipgloss.Color("255")).
			Bold(true).
			Render("> " + name + " " + capLabel)
	}
	return row
}

func capIcon(cap string) string {
	switch cap {
	case "reasoning":
		return "🧠"
	case "speed":
		return "⚡"
	case "coding":
		return "💻"
	default:
		return "✦"
	}
}

func filterByProvider(models []discoveryModel, provider string) []discoveryModel {
	var out []discoveryModel
	for _, m := range models {
		if m.Provider == provider {
			out = append(out, m)
		}
	}
	return out
}

// renderAgentTree renders the right panel: a hierarchical tree of active sub-agents.
// It reads directly from the discoveryMgr if available; otherwise shows a placeholder.
func (m *Model) renderAgentTree(width int) string {
	var b strings.Builder
	b.WriteString(discHeaderStyle.Render("Active Delegation Tree"))
	b.WriteString("\n")

	// Get the orchestrator's discovery manager via the orchestrator field.
	// We access it through the interface to avoid a direct import cycle.
	var nodes []*discovery.AgentNode
	if m.orchestrator != nil && m.orchestrator.Discovery != nil {
		nodes = m.orchestrator.Discovery.AgentTree()
	}

	if len(nodes) == 0 {
		b.WriteString(discEmptyStyle.Render("  No active sub-agents"))
		return lipgloss.NewStyle().Width(width).Render(b.String())
	}

	for i, node := range nodes {
		isLast := i == len(nodes)-1
		renderAgentNode(&b, node, "", isLast)
	}

	return lipgloss.NewStyle().Width(width).Render(b.String())
}

func renderAgentNode(b *strings.Builder, node *discovery.AgentNode, prefix string, isLast bool) {
	connector := "├─"
	childPrefix := prefix + "│  "
	if isLast {
		connector = "└─"
		childPrefix = prefix + "   "
	}

	icon := node.StatusIcon()
	styledIcon := statusStyle(node.Status).Render(icon)
	depthLabel := treeDepthStyle.Render(fmt.Sprintf("[d%d]", node.Depth))
	modelLabel := discCapStyle.Render(truncateStr(node.ModelID, 20))
	taskLabel := discModelStyle.Render(truncateStr(node.Task, 30))

	line := fmt.Sprintf("%s%s %s %s %s %s",
		treeDepthStyle.Render(prefix+connector+" "),
		styledIcon,
		depthLabel,
		modelLabel,
		"→",
		taskLabel,
	)
	b.WriteString("  " + line + "\n")

	for i, child := range node.Children {
		renderAgentNode(b, child, childPrefix, i == len(node.Children)-1)
	}
}

func statusStyle(status string) lipgloss.Style {
	switch status {
	case "running":
		return treeRunningStyle
	case "verifying":
		return treeVerifyStyle
	case "done":
		return treeDoneStyle
	case "failed":
		return treeFailStyle
	default:
		return treeDepthStyle
	}
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
