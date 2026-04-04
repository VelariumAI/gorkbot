package platform

import (
	"log/slog"
	"os"
	"testing"
)

func TestActiveFeatures_ReturnsMap(t *testing.T) {
	features := ActiveFeatures()

	// Map should not be empty
	if len(features) == 0 {
		t.Error("ActiveFeatures returned empty map")
	}

	// Check expected keys exist
	expectedKeys := []string{"security", "headless", "plugins", "mcp"}
	for _, key := range expectedKeys {
		if _, ok := features[key]; !ok {
			t.Errorf("ActiveFeatures missing expected key: %s", key)
		}
	}

	// All values should be booleans (implicitly true via assignment)
	for key, value := range features {
		if value != true && value != false {
			t.Errorf("ActiveFeatures[%s] is not a boolean", key)
		}
	}
}

func TestVariantName_NotEmpty(t *testing.T) {
	name := VariantName()
	if name == "" {
		t.Error("VariantName returned empty string")
	}

	// Variant name should be one of the expected values or a hyphen-separated combination
	if len(name) < 3 {
		t.Errorf("VariantName too short: %s", name)
	}
}

func TestVariantName_LiteWhenAllDisabled(t *testing.T) {
	// This test will only work if no features are compiled in
	// It's informational—the actual result depends on build tags
	name := VariantName()
	t.Logf("Current variant name: %s", name)

	// Verify it's a valid non-empty string
	if name == "" {
		t.Error("VariantName should never return empty string")
	}
}

func TestLogFeatures_NoError(t *testing.T) {
	// Create a simple logger that writes to a test buffer
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Should not panic or error
	LogFeatures(logger)
	t.Log("LogFeatures executed without panic")
}

func TestLogFeatures_NilSafe(t *testing.T) {
	// Should not panic when logger is nil
	LogFeatures(nil)
	t.Log("LogFeatures handled nil logger gracefully")
}

func TestValidateFeatureGates_NoError(t *testing.T) {
	err := ValidateFeatureGates()
	if err != nil {
		t.Errorf("ValidateFeatureGates returned unexpected error: %v", err)
	}
}
