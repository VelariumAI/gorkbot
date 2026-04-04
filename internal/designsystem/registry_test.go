package designsystem

import (
	"log/slog"
	"testing"
)

// TestInit initializes the registry and verifies it works.
func TestInit(t *testing.T) {
	logger := slog.Default()

	err := Init(logger)
	if err != nil {
		t.Errorf("Init() failed: %v", err)
	}

	// Verify global registry is available
	reg := Get()
	if reg == nil {
		t.Errorf("Get() returned nil after Init()")
	}
}

// TestGetColors returns current color tokens.
func TestGetColors(t *testing.T) {
	logger := slog.Default()
	Init(logger)

	reg := Get()
	colors := reg.GetColors()

	if colors.BG.Canvas == "" {
		t.Errorf("colors.BG.Canvas is empty")
	}
}

// TestGetSpacing returns current spacing scale.
func TestGetSpacing(t *testing.T) {
	logger := slog.Default()
	Init(logger)

	reg := Get()
	spacing := reg.GetSpacing()

	if spacing.Xs != 4 {
		t.Errorf("spacing.Xs should be 4, got %d", spacing.Xs)
	}
}

// TestGetTypography returns current typography tokens.
func TestGetTypography(t *testing.T) {
	logger := slog.Default()
	Init(logger)

	reg := Get()
	typography := reg.GetTypography()

	if typography.Display.Size == 0 {
		t.Errorf("typography.Display.Size should be non-zero")
	}
}

// TestSetDensity changes and retrieves density mode.
func TestSetDensity(t *testing.T) {
	logger := slog.Default()
	Init(logger)

	reg := Get()

	// Default should be Operator
	if reg.GetDensity() != DensityOperator {
		t.Errorf("default density should be Operator, got %s", reg.GetDensity())
	}

	// Change to Focus
	reg.SetDensity(DensityFocus)
	if reg.GetDensity() != DensityFocus {
		t.Errorf("density should be Focus after SetDensity, got %s", reg.GetDensity())
	}

	// Change to Orchestration
	reg.SetDensity(DensityOrchestration)
	if reg.GetDensity() != DensityOrchestration {
		t.Errorf("density should be Orchestration after SetDensity, got %s", reg.GetDensity())
	}
}

// TestGetDensitySettings returns settings for current mode.
func TestGetDensitySettings(t *testing.T) {
	logger := slog.Default()
	Init(logger)

	reg := Get()

	reg.SetDensity(DensityFocus)
	settings := reg.GetDensitySettings()

	if settings.Mode != DensityFocus {
		t.Errorf("settings.Mode should be Focus, got %s", settings.Mode)
	}
	if settings.ShowMetrics {
		t.Errorf("focus mode should not show metrics")
	}
}

// TestSetColors replaces color tokens.
func TestSetColors(t *testing.T) {
	logger := slog.Default()
	Init(logger)

	reg := Get()
	originalColors := reg.GetColors()

	// Create new colors
	newColors := NewColorTokens()
	newColors.Accent.Primary = "#ff0000" // Red

	err := reg.SetColors(newColors)
	if err != nil {
		t.Errorf("SetColors() failed: %v", err)
	}

	updatedColors := reg.GetColors()
	if updatedColors.Accent.Primary != "#ff0000" {
		t.Errorf("accent.primary should be #ff0000, got %s", updatedColors.Accent.Primary)
	}

	// Restore original
	reg.SetColors(originalColors)
}

// TestSetColorsValidation verifies invalid colors are rejected.
func TestSetColorsValidation(t *testing.T) {
	logger := slog.Default()
	Init(logger)

	reg := Get()

	// Create invalid colors (bad hex)
	badColors := NewColorTokens()
	badColors.BG.Canvas = "invalid" // Not valid hex

	err := reg.SetColors(badColors)
	if err == nil {
		t.Errorf("SetColors() should reject invalid hex values")
	}
}

// TestSummary returns human-readable summary.
func TestSummary(t *testing.T) {
	logger := slog.Default()
	Init(logger)

	reg := Get()
	summary := reg.Summary()

	if summary == "" {
		t.Errorf("Summary() returned empty string")
	}

	if !contains(summary, "Design System") {
		t.Errorf("summary should mention Design System")
	}
}

// helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && len(s) >= len(substr)
}
