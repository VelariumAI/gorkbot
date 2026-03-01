package tui

import (
	"time"

	"github.com/charmbracelet/bubbles/spinner"
)

// ArcaneSpinner creates a custom arcane-themed spinner with a narrative arc
// The animation tells a story: Void → Convergence → Manifestation → Pulse
// Uses Mathematical Bold Capital G (𝐆) as the central arcane symbol
func ArcaneSpinner() spinner.Spinner {
	const centerChar = "𝐆"

	return spinner.Spinner{
		Frames: []string{
			// Phase 1: The Void (Empty/Sparse) - The quiet before the magic
			"           ",        // 1. Emptiness
			" .       . ",        // 2. First sparks
			"   .   .   ",        // 3. Energy gathering

			// Phase 2: Convergence (Building Up) - Power coalescing
			"    o o    ",        // 4. Particles forming
			"     o     ",        // 5. Focusing inward

			// Phase 3: Manifestation (The G appears) - The symbol emerges
			"     " + centerChar + "     ", // 6. The arcane symbol materializes

			// Phase 4: The Pulse (Magical Energy Loop) - Living, breathing magic
			"   * " + centerChar + " *  ",  // 7. Energy expands
			"  ✨ " + centerChar + " ✨  ", // 8. Peak magical power
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

// ClassicSpinner returns the default Grokster spinner (dot style)
// Available as fallback or for minimal visual distraction
func ClassicSpinner() spinner.Spinner {
	return spinner.Dot
}
