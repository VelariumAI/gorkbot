package tui

import (
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
)

// RadiationSpinner creates a smooth pulsating starburst spinner.
// Simulates cosmic radiation or stellar pulsation with organic breathing motion.
// Frames transition from dim → bright → dim across 14 stages for seamless looping.
func RadiationSpinner() spinner.Spinner {
	return spinner.Spinner{
		// Progressive pulsation: · ◦ ○ ◉ ⚬ ★ ✦ ✧ ✦ ★ ⚬ ◉ ○ ◦
		// Creates a smooth "breathing" effect that feels natural and calming
		Frames: []string{
			"·",   // 0. Very dim (void)
			"◦",   // 1. Dim (emergence)
			"○",   // 2. Medium-dim (gathering)
			"◉",   // 3. Medium (coalescing)
			"⚬",   // 4. Medium-bright (building)
			"★",   // 5. Bright (manifest)
			"✦",   // 6. Very bright (peak)
			"✧",   // 7. Peak brightness (zenith)
			"✦",   // 8. Very bright (descent begins)
			"★",   // 9. Bright (fading)
			"⚬",   // 10. Medium-bright (releasing)
			"◉",   // 11. Medium (dispersing)
			"○",   // 12. Medium-dim (scattering)
			"◦",   // 13. Dim (returning)
		},
		FPS: 80 * time.Millisecond, // Organic breathing pace (80ms per frame)
	}
}

// FastRadiationSpinner is a faster version of RadiationSpinner.
// Good for high-priority operations or user impatience scenarios.
func FastRadiationSpinner() spinner.Spinner {
	s := RadiationSpinner()
	s.FPS = 60 * time.Millisecond // Quickened heartbeat
	return s
}

// SlowRadiationSpinner is a slower, more meditative version.
// Perfect for introspective operations or background tasks requiring calm.
func SlowRadiationSpinner() spinner.Spinner {
	s := RadiationSpinner()
	s.FPS = 120 * time.Millisecond // Slow, contemplative pacing
	return s
}

// ── Multi-line Radiation Sigil Spinner ─────────────────────────────────────────
//
// RadiationSigilFrame returns a 3-line radiant spinner for the current frame.
// Call it directly (not via bubbles/spinner) — it is a raw frame renderer
// driven by the unified hookTick / SpinnerTickMsg loop.
//
// Vertical bounds: occupies exactly 3 terminal lines.
// Use only when m.height >= 10; otherwise call DegradedRadiationFrame.

// radiationSigilFrames holds the 3-row string for each animation frame.
// Each frame is a 3-line string separated by "\n".
// Palette: cyan/magenta radiation aura (colours applied by caller via lipgloss).
var radiationSigilFrames = []string{
	// Frame 0 — void (barely visible)
	"   ·       \n     ·     \n   ·       ",
	// Frame 1 — first sparks emerge
	"  ◦ · ◦   \n  · · ·   \n  ◦   ◦   ",
	// Frame 2 — energy gathering
	" ◦   ◦   \n  ○ · ○  \n ◦   ◦   ",
	// Frame 3 — particles forming
	" ◉  ◉  ◉ \n ◉ · ◉ \n ◉  ◉  ◉ ",
	// Frame 4 — building intensity
	"⚬   ⚬   \n ⚬ ★ ⚬ \n⚬   ⚬   ",
	// Frame 5 — manifest star
	"★   ★   \n ★ ✦ ★ \n★   ★   ",
	// Frame 6 — very bright (peak)
	"✦   ✦   \n ✦ ✧ ✦ \n✦   ✦   ",
	// Frame 7 — zenith (peak brightness)
	"✧   ✧   \n ✧ ✧ ✧ \n✧   ✧   ",
	// Frame 8 — descent begins
	"✦   ✦   \n ✦ ✧ ✦ \n✦   ✦   ",
	// Frame 9 — fading
	"★   ★   \n ★ ✦ ★ \n★   ★   ",
	// Frame 10 — releasing
	"⚬   ⚬   \n ⚬ ★ ⚬ \n⚬   ⚬   ",
	// Frame 11 — dispersing
	" ◉  ◉  ◉ \n ◉ · ◉ \n ◉  ◉  ◉ ",
	// Frame 12 — scattering
	" ◦   ◦   \n  ○ · ○  \n ◦   ◦   ",
	// Frame 13 — returning to void
	"  ◦ · ◦   \n  · · ·   \n  ◦   ◦   ",
}

// RadiationSigilFrameCount is the number of animation frames.
const RadiationSigilFrameCount = 14

// RadiationSigilFrame returns the 3-line radiant frame string for the given index.
// The frame is styled with cyan/magenta radiation aura via lipgloss.
// Call this for high-visual-impact operations (complex queries, research, etc).
func RadiationSigilFrame(frameIdx int) string {
	f := radiationSigilFrames[frameIdx%RadiationSigilFrameCount]

	// Color mapping: cyan for outer aura, magenta for core
	auraStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("51"))   // Cyan
	coreStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("201"))  // Magenta
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))   // Dark grey

	// Shade characters by intensity
	f = radReplace(f, "✧", coreStyle)           // Peak brightness (core)
	f = radReplace(f, "✦", auraStyle)           // Very bright (aura)
	f = radReplace(f, "★", auraStyle)           // Bright
	f = radReplace(f, "⚬", auraStyle)           // Medium-bright
	f = radReplace(f, "◉", auraStyle)           // Medium
	f = radReplace(f, "○", dimStyle)            // Medium-dim
	f = radReplace(f, "◦", dimStyle)            // Dim
	f = radReplace(f, "·", dimStyle)            // Very dim

	return f
}

// radReplace is a helper that applies a style to a character and replaces all occurrences.
func radReplace(s string, char string, style lipgloss.Style) string {
	styled := style.Render(char)
	var result string
	for i := 0; i < len(s); i++ {
		if i+len(char) <= len(s) && s[i:i+len(char)] == char {
			result += styled
			i += len(char) - 1
		} else {
			result += string(s[i])
		}
	}
	return result
}

// DegradedRadiationFrame returns a single-line (1-row) pulse for small terminals.
// Used when m.height < 10 to avoid scroll tearing.
// Provides smooth star pulsation on a single line.
func DegradedRadiationFrame(frameIdx int) string {
	glyphs := []string{"·", "◦", "○", "◉", "⚬", "★", "✦", "✧", "✦", "★", "⚬", "◉", "○", "◦"}
	g := glyphs[frameIdx%len(glyphs)]

	// Color based on intensity
	var style lipgloss.Style
	switch g {
	case "✧", "✦":
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("201")) // Magenta (bright)
	case "★", "⚬", "◉":
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("51")) // Cyan (medium)
	default:
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("240")) // Grey (dim)
	}

	return style.Render(g)
}

// GetLoadingSigilFrame returns the appropriate loading indicator frame.
// Chooses between radiation and arcane sigils based on operation phase.
// For research/web operations, prefers radiation sigil; otherwise uses arcane.
func GetLoadingSigilFrame(frameIdx int, genPhase int, isResearch bool) string {
	if isResearch {
		return RadiationSigilFrame(frameIdx)
	}
	return ArcaneSigilFrame(frameIdx)
}

// GetDegradedLoadingFrame returns the appropriate degraded (single-line) frame.
// Chooses between radiation and arcane degraded frames based on context.
func GetDegradedLoadingFrame(frameIdx int, isResearch bool) string {
	if isResearch {
		return DegradedRadiationFrame(frameIdx)
	}
	return DegradedSigilFrame(frameIdx)
}
