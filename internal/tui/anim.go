package tui

import (
	"math"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// AnimFPS defines the animation frame rate in milliseconds (20 FPS = 50ms per frame).
const AnimFPS = 50

// AnimTickMsg is sent by the animation coordinator every AnimFPS ms.
type AnimTickMsg struct {
	At time.Time
}

// AnimTick returns a tea.Cmd that fires AnimTickMsg after AnimFPS ms.
// Use this in your Update() loop to drive smooth animations.
func AnimTick() tea.Cmd {
	return tea.Tick(time.Duration(AnimFPS)*time.Millisecond, func(t time.Time) tea.Msg {
		return AnimTickMsg{At: t}
	})
}

// EaseInOut applies a smooth ease-in-out cubic curve to t ∈ [0,1].
// Returns a value in [0,1] with smooth acceleration and deceleration.
func EaseInOut(t float64) float64 {
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}

	if t < 0.5 {
		// First half: accelerate (cubic ease-in)
		return 4 * t * t * t
	}
	// Second half: decelerate (cubic ease-out)
	return 1 - math.Pow(-2*t+2, 3)/2
}

// Lerp linearly interpolates between a and b by t ∈ [0,1].
func Lerp(a, b, t float64) float64 {
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return a + (b-a)*t
}

// PulseValue returns a value oscillating between lo and hi using a sine wave curve.
// frame: current frame number (0+)
// totalFrames: total frames in the pulse cycle
// lo, hi: minimum and maximum values
// Returns a value between lo and hi.
func PulseValue(frame int, totalFrames int, lo, hi float64) float64 {
	if totalFrames <= 0 {
		return lo
	}

	// Normalize frame to [0, 1]
	t := float64(frame % totalFrames) / float64(totalFrames)

	// Sine wave oscillation: sin(2π*t) ranges from 0 to 1 to 0
	// Scale to [0,1] range
	sineValue := (math.Sin(2*math.Pi*t) + 1) / 2

	// Interpolate between lo and hi
	return Lerp(lo, hi, sineValue)
}

// FrameToPercent converts a frame counter to a percentage value [0,1].
// Useful for progress animations.
func FrameToPercent(frame int, totalFrames int) float64 {
	if totalFrames <= 0 {
		return 0
	}
	pct := float64(frame) / float64(totalFrames)
	if pct > 1 {
		pct = 1
	}
	return pct
}

// CubicBezier evaluates a cubic Bézier curve at t ∈ [0,1].
// Control points: p0 (start), p1, p2, p3 (end).
// All should be in [0,1] for typical animation curves.
func CubicBezier(p0, p1, p2, p3, t float64) float64 {
	// Bézier formula: B(t) = (1-t)³P0 + 3(1-t)²tP1 + 3(1-t)t²P2 + t³P3
	one_t := 1 - t
	a := one_t * one_t * one_t
	b := 3 * one_t * one_t * t
	c := 3 * one_t * t * t
	d := t * t * t

	return a*p0 + b*p1 + c*p2 + d*p3
}
