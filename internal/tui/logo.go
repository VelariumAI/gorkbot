package tui

// logo.go — loads the Gorkbot mascot PNG (embedded at compile time) into Unicode
// block-art lines for display in the TUI header alongside the ASCII "GORKBOT" text.

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/velariumai/gorkbot/blockmosaic"
	"github.com/velariumai/gorkbot/internal/assets"
)

// logoConfig controls the mascot render size.
// MaxHeight=7 gives 7 character rows (≈ the 5-line ASCII art + breathing room).
// A roughly square image at AspectRatio=0.5 fills ~14 chars wide.
var logoConfig = blockmosaic.Config{
	MaxWidth:     18,
	MaxHeight:    7,
	AspectRatio:  0.5,
	UseTrueColor: true,
}

// loadLogoLines decodes the embedded gorkbot.png and returns ANSI-colored block-art
// lines plus the visible character width. Returns nil, 0 on any error — the header
// falls back gracefully to ASCII-only mode.
func loadLogoLines() (lines []string, width int) {
	art, err := blockmosaic.RenderFromBytes(assets.GorkbotPNG, logoConfig)
	if err != nil {
		return nil, 0
	}
	// Split into individual lines (strip trailing empty line from final \n).
	raw := strings.Split(strings.TrimRight(art, "\n"), "\n")
	if len(raw) == 0 {
		return nil, 0
	}
	// Measure the visible width of the widest line (ANSI-stripped by lipgloss.Width).
	maxW := 0
	for _, l := range raw {
		if w := lipgloss.Width(l); w > maxW {
			maxW = w
		}
	}
	return raw, maxW
}
