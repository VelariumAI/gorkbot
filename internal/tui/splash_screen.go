// Package tui — splash_screen.go
//
// Full-screen animated startup splash shown while !m.splashDone.
// Press Enter (when m.ready == true) to dismiss and enter the main TUI.
package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/velariumai/gorkbot/internal/platform"
)

// renderSplashScreen renders the full-screen startup splash.
// Shown while !m.splashDone. Transitions to main TUI on Enter (when m.ready).
func (m *Model) renderSplashScreen() string {
	// Animated GORKBOT ASCII art (reuse existing glistenPos/spotlightPos).
	art := RenderGorkbotHeaderWithLogo(
		platform.Version, m.width,
		m.glistenPos, m.spotlightPos,
		m.logoLines, m.logoWidth,
	)

	// Status line: spinner while loading, green check when ready.
	var status string
	if !m.ready {
		status = m.spinner.View() + "  Initializing..."
	} else {
		status = lipgloss.NewStyle().
			Foreground(lipgloss.Color(SuccessGreen)).
			Render("✓ Ready")
	}

	// Call-to-action — only shown once ready.
	cta := ""
	if m.ready {
		cta = lipgloss.NewStyle().
			Foreground(lipgloss.Color(TextGray)).
			Italic(true).
			Render("Press Enter to begin")
	}

	content := lipgloss.JoinVertical(lipgloss.Center,
		art,
		"",
		status,
		"",
		cta,
	)

	// Centre the content on a pure-black full-screen canvas.
	return lipgloss.NewStyle().
		Background(lipgloss.Color("0")).
		Width(m.width).
		Height(m.height).
		Render(
			lipgloss.Place(m.width, m.height,
				lipgloss.Center, lipgloss.Center,
				content),
		)
}
