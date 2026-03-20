package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
)

// ArcaneSpinner creates a custom arcane-themed spinner with a narrative arc
// The animation tells a story: Void → Convergence → Manifestation → Pulse
// Uses Mathematical Bold Capital G (𝐆) as the central arcane symbol
func ArcaneSpinner() spinner.Spinner {
	const centerChar = "𝐆"

	return spinner.Spinner{
		Frames: []string{
			// Phase 1: The Void (Empty/Sparse) - The quiet before the magic
			"           ", // 1. Emptiness
			" .       . ", // 2. First sparks
			"   .   .   ", // 3. Energy gathering

			// Phase 2: Convergence (Building Up) - Power coalescing
			"    o o    ", // 4. Particles forming
			"     o     ", // 5. Focusing inward

			// Phase 3: Manifestation (The G appears) - The symbol emerges
			"     " + centerChar + "     ", // 6. The arcane symbol materializes

			// Phase 4: The Pulse (Magical Energy Loop) - Living, breathing magic
			"   * " + centerChar + " *  ",  // 7. Energy expands
			"  ✨ " + centerChar + " ✨  ",   // 8. Peak magical power
			"   * " + centerChar + " *  ",  // 9. Energy contracts
			"     " + centerChar + "     ", // 10. Stabilize (cycle ready to repeat)
		},
		FPS: 120 * time.Millisecond, // 120ms per frame - weighty, magical timing
	}
}

// PulseSpinner creates a simpler pulsing arcane spinner
// Good for less intensive operations or consultant mode
func PulseSpinner() spinner.Spinner {
	const centerChar = "𝐆"

	return spinner.Spinner{
		Frames: []string{
			" " + centerChar + " ",
			" ·" + centerChar + "· ",
			" °" + centerChar + "° ",
			" *" + centerChar + "* ",
			" ✨" + centerChar + "✨ ",
			" *" + centerChar + "* ",
			" °" + centerChar + "° ",
			" ·" + centerChar + "· ",
		},
		FPS: 100 * time.Millisecond, // Slightly faster for quick operations
	}
}

// ClassicSpinner returns the default Gorkbot spinner (dot style)
// Available as fallback or for minimal visual distraction
func ClassicSpinner() spinner.Spinner {
	return spinner.Dot
}

// ── Multi-line Arcane Sigil Spinner ──────────────────────────────────────────
//
// ArcaneSigilSpinner returns a 3-line spinner string for the current frame.
// Call it directly (not via bubbles/spinner) — it is a raw frame renderer
// driven by the unified hookTick / SpinnerTickMsg loop.
//
// Vertical bounds: occupies exactly 3 terminal lines.
// Use only when m.height >= 10; otherwise call DegradedSigilSpinner.

// arcaneSigilFrames holds the 3-row string for each animation frame.
// Each frame is a 3-line string separated by "\n".
// Palette: blood-red ember (no ANSI — colours applied by caller via lipgloss).
var arcaneSigilFrames = []string{
	// Frame 0 — void (barely visible)
	"  ·     ·  \n   · · ·   \n  ·     ·  ",
	// Frame 1 — embers coalesce
	"  ░  𝐆  ░  \n ░  ╳ ╳  ░ \n  ░     ░  ",
	// Frame 2 — sigil awakens
	" ▒  𝐆  ▒   \n ▒ ╋━━━╋ ▒ \n  ▒     ▒  ",
	// Frame 3 — full blaze
	" ▓  𝐆  ▓   \n ▓ ╋━━━╋ ▓ \n  ▓  ◈  ▓  ",
	// Frame 4 — pulse peak
	"▓▓  𝐆  ▓▓  \n ▓ ╋━━━╋ ▓ \n ▓▓  ◈  ▓▓ ",
	// Frame 5 — contraction
	" ▒  𝐆  ▒   \n ▒ ╋━━━╋ ▒ \n  ▒  ·  ▒  ",
	// Frame 6 — embers scatter
	"  ░  𝐆  ░  \n  ░ · · ░  \n   ░   ░   ",
	// Frame 7 — near void
	"   · 𝐆 ·   \n    · ·    \n   ·   ·   ",
}

// ArcaneSigilFrameCount is the number of animation frames.
const ArcaneSigilFrameCount = 8

// ArcaneSigilFrame returns the 3-line frame string for the given index.
// The frame is styled with blood-red ember colours via lipgloss.
func ArcaneSigilFrame(frameIdx int) string {
	f := arcaneSigilFrames[frameIdx%ArcaneSigilFrameCount]

	// Colour mapping: ▓▓ = bright ember, ▒▒ = mid ember, ░░ = dim ember
	// Apply a single bold blood-red foreground — the caller may override.
	emberStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ArcaneEmber))
	sigilStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ArcanePrimary)).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ArcaneDim))

	// Replace the sigil character with styled version.
	f = strings.ReplaceAll(f, "𝐆", sigilStyle.Render("𝐆"))

	// Shade block characters.
	f = strings.ReplaceAll(f, "▓", emberStyle.Render("▓"))
	f = strings.ReplaceAll(f, "▒", emberStyle.Render("▒"))
	f = strings.ReplaceAll(f, "░", dimStyle.Render("░"))
	f = strings.ReplaceAll(f, "·", dimStyle.Render("·"))

	return f
}

// DegradedSigilFrame returns a single-line (1-row) pulse for small terminals.
// Used when m.height < 10 to avoid scroll tearing.
func DegradedSigilFrame(frameIdx int) string {
	glyphs := []string{"·", "°", "*", "𝐆", "*", "°", "·", " "}
	g := glyphs[frameIdx%len(glyphs)]
	emberStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ArcaneEmber))
	sigilStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ArcanePrimary)).Bold(true)
	if g == "𝐆" {
		return sigilStyle.Render(g)
	}
	return emberStyle.Render(g)
}
