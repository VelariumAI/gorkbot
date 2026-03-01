package tui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ASCII art for GORKBOT using block characters
// Each letter is 5 lines tall, 9 runes wide (7 letters × 9 = 63 runes/line)
// G  O  R  K  B  O  T
var gorkbotASCII = []string{
	" ██████   ██████  ███████  ██   ██  ███████   ██████  ████████ ",
	"██       ██    ██ ██    ██ ██  ██   ██    ██ ██    ██    ██    ",
	"██  ████ ██    ██ ███████  █████    ███████  ██    ██    ██    ",
	"██    ██ ██    ██ ██  ██   ██  ██   ██    ██ ██    ██    ██    ",
	" ██████   ██████  ██   ██  ██   ██  ███████   ██████     ██    ",
}

// Magical color palette for the header - strictly Blood Red shades
var magicalColors = []string{
	"52",  // Darkest red
	"88",  // Dark red
	"124", // Medium red
	"160", // Bright red
	"196", // Brilliant red
}

// RenderGorkbotHeader creates a colorful ASCII art header with light glisten effect
func RenderGorkbotHeader(version string, width int, glistenPos, spotlightPos float64) string {
	var output strings.Builder

	// Generate colors for each letter based on glisten + spotlight positions
	// G O R K B O T = 7 letters
	letterColors := make([]string, 7)
	for i := 0; i < 7; i++ {
		letterColors[i] = getGlistenColor(i, 7, glistenPos, spotlightPos)
	}

	// Render each line of the ASCII art
	for _, line := range gorkbotASCII {
		coloredLine := colorizeGorkbotLine(line, letterColors)
		// Center the line
		centered := lipgloss.PlaceHorizontal(width, lipgloss.Center, coloredLine)
		output.WriteString(centered + "\n")
	}

	// Add version subscript — "with SENSE | v1.5.3"
	versionText := fmt.Sprintf("with SENSE | v%s  ·  by Todd Eddings / Velarium AI", version)
	versionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true).
		Faint(true)

	centeredVersion := lipgloss.PlaceHorizontal(width, lipgloss.Center, versionStyle.Render(versionText))
	output.WriteString(centeredVersion + "\n")

	return output.String()
}

// getGlistenColor calculates the color for a letter based on glisten wave + spotlight positions
// Uses blood red as base with brightness variations for the glisten effect
func getGlistenColor(letterIdx, totalLetters int, glistenPos, spotlightPos float64) string {
	// Calculate normalized position of this letter (0.0 to 1.0)
	letterPos := float64(letterIdx) / float64(totalLetters)

	// === MAIN GLISTEN WAVE (Background Pulse) ===
	// Slow, deep pulse using cosine wave
	pulsePhase := 2.0 * math.Pi * (glistenPos * 0.5) // Slower background pulse
	pulseBrightness := (math.Cos(pulsePhase) + 1.0) / 2.0 * 0.3 // Base brightness 0.0-0.3

	// === SEARCHLIGHT SWEEP (Deterministic Single Pass) ===
	// The spotlightPos goes from 0.0 to 1.0 representing the center of the beam
	dist := math.Abs(letterPos - spotlightPos)

	beamWidth := 0.15
	beamBrightness := 0.0

	if dist < beamWidth {
		// Gaussian-ish falloff for the beam
		normalizedDist := dist / beamWidth
		beamBrightness = math.Pow(1.0-normalizedDist, 2) * 1.0 // Peak brightness 1.0
	}

	// Combine: Base pulse (dim) + Beam (bright)
	totalBrightness := 0.2 + pulseBrightness + beamBrightness
	if totalBrightness > 1.0 {
		totalBrightness = 1.0
	}

	// Map brightness to blood red color shades
	if totalBrightness < 0.3 {
		return "52"  // Darkest red
	} else if totalBrightness < 0.5 {
		return "88"  // Dark red
	} else if totalBrightness < 0.7 {
		return "124" // Medium red
	} else if totalBrightness < 0.9 {
		return "160" // Bright red
	} else {
		return "196" // Brilliant red (The searchlight core)
	}
}

// colorizeGorkbotLine applies different colors to each letter in the ASCII art line
func colorizeGorkbotLine(line string, letterColors []string) string {
	// Rune boundaries for "GORKBOT" — 7 letters × 9 runes each = 63 runes/line
	// G: 0-8, O: 9-17, R: 18-26, K: 27-35, B: 36-44, O: 45-53, T: 54-62
	letterBoundaries := []int{0, 9, 18, 27, 36, 45, 54, 63}

	var result strings.Builder
	runes := []rune(line)

	for i, char := range runes {
		// Determine which letter this character belongs to
		letterIndex := len(letterBoundaries) - 2 // default to last letter
		for j := 0; j < len(letterBoundaries)-1; j++ {
			if i >= letterBoundaries[j] && i < letterBoundaries[j+1] {
				letterIndex = j
				break
			}
		}

		// Apply color to the character
		if letterIndex < len(letterColors) {
			coloredChar := lipgloss.NewStyle().
				Foreground(lipgloss.Color(letterColors[letterIndex])).
				Render(string(char))
			result.WriteString(coloredChar)
		} else {
			result.WriteRune(char)
		}
	}

	return result.String()
}

// RenderGorkbotHeaderWithLogo is like RenderGorkbotHeader but places a pre-rendered
// block-art logo to the left of the GORKBOT ASCII art on each of the 5 banner lines.
// logoLines contains the ANSI-colored lines from blockmosaic; logoWidth is their visible width.
// If logoLines is empty the function falls back to the plain header.
func RenderGorkbotHeaderWithLogo(version string, width int, glistenPos, spotlightPos float64, logoLines []string, logoWidth int) string {
	if len(logoLines) == 0 {
		return RenderGorkbotHeader(version, width, glistenPos, spotlightPos)
	}

	var output strings.Builder

	letterColors := make([]string, 7)
	for i := 0; i < 7; i++ {
		letterColors[i] = getGlistenColor(i, 7, glistenPos, spotlightPos)
	}

	const gap = "  " // Two-space gap between mascot and ASCII art

	nArt := len(gorkbotASCII)
	nLogo := len(logoLines)
	totalLines := nArt
	if nLogo > nArt {
		totalLines = nLogo
	}

	// Pre-compute art line visible width for padding logo-only rows (all art lines are equal).
	artLineWidth := 0
	if nArt > 0 {
		artLineWidth = lipgloss.Width(colorizeGorkbotLine(gorkbotASCII[0], letterColors))
	}

	for i := 0; i < totalLines; i++ {
		logoLine := ""
		if i < nLogo {
			logoLine = logoLines[i]
		}
		// Pad to exact logoWidth so columns stay aligned.
		vis := lipgloss.Width(logoLine)
		if vis < logoWidth {
			logoLine += strings.Repeat(" ", logoWidth-vis)
		}

		var combined string
		if i < nArt {
			coloredArt := colorizeGorkbotLine(gorkbotASCII[i], letterColors)
			combined = logoLine + gap + coloredArt
		} else {
			// Extra logo lines below the ASCII art — pad right to keep centering consistent.
			combined = logoLine + gap + strings.Repeat(" ", artLineWidth)
		}
		centered := lipgloss.PlaceHorizontal(width, lipgloss.Center, combined)
		output.WriteString(centered + "\n")
	}

	// Version subscript — "with SENSE | v1.5.3  ·  by Todd Eddings / Velarium AI"
	versionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true).
		Faint(true)
	centeredVersion := lipgloss.PlaceHorizontal(width, lipgloss.Center,
		versionStyle.Render("with SENSE | v"+version+"  ·  by Todd Eddings / Velarium AI"))
	output.WriteString(centeredVersion + "\n")

	return output.String()
}

// RenderCompactHeader creates a smaller header for constrained spaces
func RenderCompactHeader(version string, width int) string {
	title := "✨ GORKBOT with SENSE ✨"

	// Fixed Blood Red for compact header
	color := "196"

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(color)).
		Bold(true)

	versionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true)

	senseStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true)

	titleText := titleStyle.Render(title)
	versionText := versionStyle.Render(fmt.Sprintf("v%s", version))
	senseText := senseStyle.Render("by Todd Eddings / Velarium AI")
	combined := fmt.Sprintf("%s %s  %s", titleText, versionText, senseText)

	return lipgloss.PlaceHorizontal(width, lipgloss.Center, combined)
}
