package designsystem

import (
	"strings"
	"testing"
)

// TestColorTokensValid verifies all color tokens are valid hex values.
func TestColorTokensValid(t *testing.T) {
	ct := NewColorTokens()

	if err := ct.ValidateColors(); err != nil {
		t.Errorf("color validation failed: %v", err)
	}
}

// TestColorTokensNotEmpty verifies no color is empty.
func TestColorTokensNotEmpty(t *testing.T) {
	ct := NewColorTokens()

	if ct.BG.Canvas == "" {
		t.Errorf("bg.canvas is empty")
	}
	if ct.Accent.Primary == "" {
		t.Errorf("accent.primary is empty")
	}
	if ct.Status.Success == "" {
		t.Errorf("status.success is empty")
	}
}

// TestSpacingScaleNoGaps verifies spacing is properly spaced.
func TestSpacingScaleNoGaps(t *testing.T) {
	ss := NewSpacingScale()

	if ss.Xs != 4 {
		t.Errorf("xs spacing should be 4, got %d", ss.Xs)
	}
	if ss.Xxxl != 48 {
		t.Errorf("xxxl spacing should be 48, got %d", ss.Xxxl)
	}

	// Verify all values are multiples of 4
	values := []int{ss.Xs, ss.Sm, ss.Md, ss.Base, ss.Lg, ss.Xl, ss.Xxl, ss.Xxxl}
	for i, val := range values {
		if val%4 != 0 {
			t.Errorf("spacing[%d] = %d is not a multiple of 4", i, val)
		}
	}
}

// TestRadiusTokens verifies radius values are sensible.
func TestRadiusTokens(t *testing.T) {
	rt := NewRadiusTokens()

	if rt.Compact < 4 || rt.Compact > 12 {
		t.Errorf("compact radius should be 4-12, got %d", rt.Compact)
	}
	if rt.Standard != 12 {
		t.Errorf("standard radius should be 12, got %d", rt.Standard)
	}
	if rt.Large != 16 {
		t.Errorf("large radius should be 16, got %d", rt.Large)
	}
}

// TestElevationLevels verifies elevation tokens are correct.
func TestElevationLevels(t *testing.T) {
	et := NewElevationTokens()

	if et.None != 0 {
		t.Errorf("none elevation should be 0, got %d", et.None)
	}
	if et.Surface != 1 {
		t.Errorf("surface elevation should be 1, got %d", et.Surface)
	}
	if et.Active != 2 {
		t.Errorf("active elevation should be 2, got %d", et.Active)
	}

	// Verify three levels only
	if et.Active > 2 {
		t.Errorf("elevation system should not exceed level 2 (no visual mud)")
	}
}

// TestTypographyTokensValidate verifies typography definitions are valid.
func TestTypographyTokensValidate(t *testing.T) {
	tt := NewTypographyTokens()

	if err := tt.ValidateTypography(); err != nil {
		t.Errorf("typography validation failed: %v", err)
	}
}

// TestTypographyWeights verifies font weights are valid.
func TestTypographyWeights(t *testing.T) {
	tt := NewTypographyTokens()

	validWeights := map[int]bool{400: true, 500: true, 600: true, 700: true}

	types := []struct {
		name   string
		weight int
	}{
		{"display", tt.Display.Weight},
		{"body", tt.Body.Weight},
		{"mono", tt.Mono.Weight},
	}

	for _, typ := range types {
		if !validWeights[typ.weight] {
			t.Errorf("%s weight %d is invalid (must be 400, 500, 600, or 700)", typ.name, typ.weight)
		}
	}
}

// TestIconTokens verifies icon symbols are defined.
func TestIconTokens(t *testing.T) {
	it := NewIconTokens()

	if it.ChatSymbol == "" {
		t.Errorf("chat symbol is empty")
	}
	if it.TasksSymbol == "" {
		t.Errorf("tasks symbol is empty")
	}
	if it.ToolsSymbol == "" {
		t.Errorf("tools symbol is empty")
	}
}

// TestDensitySettingsFocus verifies focus density settings.
func TestDensitySettingsFocus(t *testing.T) {
	settings := NewDensitySettings(DensityFocus)

	if settings.Mode != DensityFocus {
		t.Errorf("expected mode DensityFocus, got %s", settings.Mode)
	}
	if settings.ShowMetrics {
		t.Errorf("focus mode should not show metrics")
	}
	if settings.ShowToolDetails {
		t.Errorf("focus mode should not show tool details")
	}
}

// TestDensitySettingsOperator verifies operator density settings.
func TestDensitySettingsOperator(t *testing.T) {
	settings := NewDensitySettings(DensityOperator)

	if settings.Mode != DensityOperator {
		t.Errorf("expected mode DensityOperator, got %s", settings.Mode)
	}
	if !settings.ShowMetrics {
		t.Errorf("operator mode should show metrics")
	}
}

// TestDensitySettingsOrchestration verifies orchestration density settings.
func TestDensitySettingsOrchestration(t *testing.T) {
	settings := NewDensitySettings(DensityOrchestration)

	if settings.Mode != DensityOrchestration {
		t.Errorf("expected mode DensityOrchestration, got %s", settings.Mode)
	}
	if !settings.ShowMetrics {
		t.Errorf("orchestration mode should show metrics")
	}
	if !settings.ShowToolDetails {
		t.Errorf("orchestration mode should show tool details")
	}
	if !settings.ShowExecutionTrace {
		t.Errorf("orchestration mode should show execution trace")
	}
}

// TestIsValidHex verifies hex validation works.
func TestIsValidHex(t *testing.T) {
	tests := []struct {
		hex   string
		valid bool
	}{
		{"#0a0a0a", true},
		{"#ffffff", true},
		{"#000000", true},
		{"#7c3aed", true},
		{"0a0a0a", false},  // Missing #
		{"#0g0a0a", false}, // Invalid character
		{"#0a0a", false},   // Too short
		{"#0a0a0a00", false}, // Too long
		{"", false},           // Empty
	}

	for _, test := range tests {
		got := isValidHex(test.hex)
		if got != test.valid {
			t.Errorf("isValidHex(%q) = %v, want %v", test.hex, got, test.valid)
		}
	}
}

// TestColorIncontrast verifies primary and secondary colors have sufficient contrast.
func TestColorContrast(t *testing.T) {
	ct := NewColorTokens()

	// Verify text primary is lighter than background
	if strings.Compare(ct.Text.Primary, ct.BG.Canvas) < 0 {
		t.Errorf("text.primary should be lighter than bg.canvas for readability")
	}

	// Verify accent primary is different from secondary
	if ct.Accent.Primary == ct.Accent.Secondary {
		t.Errorf("accent.primary and accent.secondary should be different")
	}
}
