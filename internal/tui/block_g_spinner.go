package tui

import (
	"time"

	"github.com/charmbracelet/bubbles/spinner"
)

// BlockGSpinner creates a custom spinner that builds a block 'G'.
// Animation: Snake-like construction path.
// Shape:
// ████
// █  █
// █ ██
func BlockGSpinner() spinner.Spinner {
	return spinner.Spinner{
		Frames: []string{
			// 1. Empty
			"    \n    \n    ",

			// Top Bar (Left to Right)
			"█   \n    \n    ",
			"██  \n    \n    ",
			"███ \n    \n    ",
			"████\n    \n    ",

			// Left Wall (Down)
			"████\n█   \n    ",
			"████\n█   \n█   ",

			// Bottom Bar (Left to Right, skipping middle)
			// Target bottom: █ ██

			"████\n█   \n█  █", // Add pos 2 (skipping 1)
			"████\n█   \n█ ██", // Add pos 3

			// Hook (Up)
			// Target mid: █  █
			"████\n█  █\n█ ██", // Complete G

			// Hold
			"████\n█  █\n█ ██",
			"████\n█  █\n█ ██",
			"████\n█  █\n█ ██",

			// Blink/Pulse Effect
			"    \n    \n    ", // Flash off
			"████\n█  █\n█ ██", // Flash on
		},
		FPS: 80 * time.Millisecond,
	}
}

// ConsultantSpinner returns a pulsing red light spinner for the bottom corner
func ConsultantSpinner() spinner.Spinner {
	return spinner.Spinner{
		Frames: []string{
			"○", "◔", "◑", "◕", "●", "◕", "◑", "◔",
		},
		FPS: 80 * time.Millisecond,
	}
}
