package tui

import (
	"math"
	"testing"
)

func TestEaseInOut_Bounds(t *testing.T) {
	// Test boundary values
	if EaseInOut(0) != 0 {
		t.Errorf("EaseInOut(0) should be 0, got %f", EaseInOut(0))
	}

	if EaseInOut(1) != 1 {
		t.Errorf("EaseInOut(1) should be 1, got %f", EaseInOut(1))
	}

	// Test out-of-bounds clamping
	if EaseInOut(-0.5) != 0 {
		t.Error("EaseInOut should clamp negative values to 0")
	}

	if EaseInOut(1.5) != 1 {
		t.Error("EaseInOut should clamp values > 1 to 1")
	}
}

func TestEaseInOut_MonotonicGrowth(t *testing.T) {
	// Test that ease-in-out is monotonic increasing
	prev := EaseInOut(0.0)
	for i := 1; i <= 100; i++ {
		current := EaseInOut(float64(i) / 100)
		if current < prev {
			t.Errorf("EaseInOut not monotonic at %.2f", float64(i)/100)
		}
		prev = current
	}
}

func TestEaseInOut_MiddlePoint(t *testing.T) {
	// At t=0.5, ease-in-out should return 0.5
	v := EaseInOut(0.5)
	if math.Abs(v-0.5) > 0.01 {
		t.Errorf("EaseInOut(0.5) should be ~0.5, got %f", v)
	}
}

func TestLerp_Linear(t *testing.T) {
	result := Lerp(0, 10, 0.5)
	if result != 5 {
		t.Errorf("Lerp(0, 10, 0.5) should be 5, got %f", result)
	}

	result = Lerp(10, 20, 0.5)
	if result != 15 {
		t.Errorf("Lerp(10, 20, 0.5) should be 15, got %f", result)
	}
}

func TestLerp_BoundaryValues(t *testing.T) {
	result := Lerp(5, 10, 0)
	if result != 5 {
		t.Errorf("Lerp with t=0 should return a, got %f", result)
	}

	result = Lerp(5, 10, 1)
	if result != 10 {
		t.Errorf("Lerp with t=1 should return b, got %f", result)
	}
}

func TestLerp_Clamping(t *testing.T) {
	// Values < 0 should clamp to t=0
	result := Lerp(0, 100, -1)
	if result != 0 {
		t.Error("Lerp should clamp negative t to 0")
	}

	// Values > 1 should clamp to t=1
	result = Lerp(0, 100, 2)
	if result != 100 {
		t.Error("Lerp should clamp t>1 to 1")
	}
}

func TestPulseValue_OscillatesInRange(t *testing.T) {
	totalFrames := 100
	lo, hi := 2.0, 8.0

	for frame := 0; frame < totalFrames*2; frame++ {
		value := PulseValue(frame, totalFrames, lo, hi)

		if value < lo || value > hi {
			t.Errorf("PulseValue out of range: got %f, want [%f, %f]", value, lo, hi)
		}
	}
}

func TestPulseValue_QuarterPoints(t *testing.T) {
	totalFrames := 100
	lo, hi := 0.0, 10.0

	// At frame 0, sin(0) = 0, so value should be at lo
	v := PulseValue(0, totalFrames, lo, hi)
	if v < lo || v > hi {
		t.Errorf("PulseValue(0) should be in range [%f, %f], got %f", lo, hi, v)
	}

	// At frame 25 (quarter), sin(π/2) = 1, so should be at hi
	v = PulseValue(25, totalFrames, lo, hi)
	if v < hi-0.5 {
		t.Errorf("PulseValue(quarter) should be near peak, got %f", v)
	}

	// At frame 50 (half), sin(π) = 0, so should be back at mid
	v = PulseValue(50, totalFrames, lo, hi)
	if v < lo || v > hi {
		t.Errorf("PulseValue(half) should be in range, got %f", v)
	}

	// At frame 75 (three quarters), sin(3π/2) = -1, so should be at lo
	v = PulseValue(75, totalFrames, lo, hi)
	if v < lo || v > hi {
		t.Errorf("PulseValue(3/4) should be in range, got %f", v)
	}
}

func TestFrameToPercent_Linear(t *testing.T) {
	result := FrameToPercent(0, 100)
	if result != 0 {
		t.Errorf("FrameToPercent(0, 100) should be 0, got %f", result)
	}

	result = FrameToPercent(50, 100)
	if result != 0.5 {
		t.Errorf("FrameToPercent(50, 100) should be 0.5, got %f", result)
	}

	result = FrameToPercent(100, 100)
	if result != 1 {
		t.Errorf("FrameToPercent(100, 100) should be 1, got %f", result)
	}
}

func TestFrameToPercent_Clamping(t *testing.T) {
	// Beyond totalFrames should clamp to 1
	result := FrameToPercent(150, 100)
	if result != 1 {
		t.Error("FrameToPercent should clamp > totalFrames to 1")
	}
}

func TestCubicBezier_Endpoints(t *testing.T) {
	// At t=0, should return p0
	result := CubicBezier(0, 0.3, 0.7, 1, 0)
	if math.Abs(result-0) > 0.001 {
		t.Errorf("CubicBezier at t=0 should return p0, got %f", result)
	}

	// At t=1, should return p3
	result = CubicBezier(0, 0.3, 0.7, 1, 1)
	if math.Abs(result-1) > 0.001 {
		t.Errorf("CubicBezier at t=1 should return p3, got %f", result)
	}
}

func TestCubicBezier_Midpoint(t *testing.T) {
	// Linear curve: p0=0, p1=0.33, p2=0.67, p3=1
	result := CubicBezier(0, 0.33, 0.67, 1, 0.5)

	// Should be close to 0.5 for a nearly-linear curve
	if result < 0.4 || result > 0.6 {
		t.Errorf("CubicBezier linear curve at t=0.5 should be ~0.5, got %f", result)
	}
}
