package platform

import (
	"log/slog"
)

// ActiveFeatures returns a human-readable feature summary map.
// Each key corresponds to a build tag feature that may or may not be enabled.
func ActiveFeatures() map[string]bool {
	return map[string]bool{
		"security":  FeatureSecurity,
		"headless":  FeatureHeadless,
		"plugins":   FeaturePlugins,
		"mcp":       FeatureMCP,
	}
}

// VariantName returns a human-readable build variant label based on active features.
// Examples: "lite", "security-pack", "headless", "full"
func VariantName() string {
	features := ActiveFeatures()

	// Count enabled features
	enabledCount := 0
	for _, enabled := range features {
		if enabled {
			enabledCount++
		}
	}

	// No optional features enabled
	if enabledCount == 0 {
		return "lite"
	}

	// All optional features enabled
	if enabledCount == 4 {
		return "full"
	}

	// Security only
	if features["security"] && !features["headless"] && !features["plugins"] && !features["mcp"] {
		return "security-pack"
	}

	// Headless only
	if features["headless"] && !features["security"] && !features["plugins"] && !features["mcp"] {
		return "headless"
	}

	// Plugins only
	if features["plugins"] && !features["security"] && !features["headless"] && !features["mcp"] {
		return "plugins-only"
	}

	// Custom combination
	var names []string
	if features["security"] {
		names = append(names, "sec")
	}
	if features["headless"] {
		names = append(names, "headless")
	}
	if features["plugins"] {
		names = append(names, "plugins")
	}
	if features["mcp"] {
		names = append(names, "mcp")
	}

	if len(names) == 0 {
		return "lite"
	}

	// Join with hyphens
	result := names[0]
	for i := 1; i < len(names); i++ {
		result += "-" + names[i]
	}
	return result
}

// LogFeatures logs the active feature set via slog at the info level.
func LogFeatures(logger *slog.Logger) {
	if logger == nil {
		return
	}

	features := ActiveFeatures()
	attrs := []slog.Attr{}

	for featureName, enabled := range features {
		status := "disabled"
		if enabled {
			status = "enabled"
		}
		attrs = append(attrs, slog.String(featureName, status))
	}

	attrs = append(attrs, slog.String("variant", VariantName()))

	logger.LogAttrs(nil, slog.LevelInfo, "features", attrs...)
}

// ValidateFeatureGates returns an error if incompatible feature combinations
// are detected. For now, all combinations are valid.
func ValidateFeatureGates() error {
	// Reserved for future validation logic.
	// Examples:
	// - Cannot use security tools in lite build (if we add that constraint)
	// - Cannot use plugins without headless (if we add that constraint)
	return nil
}
