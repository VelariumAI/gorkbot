package designsystem

import (
	"log/slog"
	"sync"
)

// Registry manages design tokens and provides unified access.
// Ensures consistency between TUI and WebUI token usage.
type Registry struct {
	colors       ColorTokens
	spacing      SpacingScale
	radius       RadiusTokens
	elevation    ElevationTokens
	typography   TypographyTokens
	icons        IconTokens
	density      DensityMode
	densityMu    sync.RWMutex
	logger       *slog.Logger
}

var (
	// Global registry instance
	globalRegistry *Registry
	registryMu     sync.Mutex
)

// Init initializes the global token registry with defaults.
// Called once at application startup.
func Init(logger *slog.Logger) error {
	registryMu.Lock()
	defer registryMu.Unlock()

	if logger == nil {
		logger = slog.Default()
	}

	reg := &Registry{
		colors:     NewColorTokens(),
		spacing:    NewSpacingScale(),
		radius:     NewRadiusTokens(),
		elevation:  NewElevationTokens(),
		typography: NewTypographyTokens(),
		icons:      NewIconTokens(),
		density:    DensityOperator, // Default density
		logger:     logger,
	}

	// Validate token integrity
	if err := reg.colors.ValidateColors(); err != nil {
		return err
	}
	if err := reg.typography.ValidateTypography(); err != nil {
		return err
	}

	globalRegistry = reg
	logger.Debug("design system initialized", "density", string(DensityOperator))

	return nil
}

// Get returns the global registry instance.
// Panics if Init() was not called.
func Get() *Registry {
	registryMu.Lock()
	reg := globalRegistry
	registryMu.Unlock()

	if reg == nil {
		panic("design system registry not initialized; call designsystem.Init() first")
	}

	return reg
}

// GetColors returns the current color token set.
func (r *Registry) GetColors() ColorTokens {
	return r.colors
}

// GetSpacing returns the current spacing scale.
func (r *Registry) GetSpacing() SpacingScale {
	return r.spacing
}

// GetRadius returns the current radius token set.
func (r *Registry) GetRadius() RadiusTokens {
	return r.radius
}

// GetElevation returns the current elevation token set.
func (r *Registry) GetElevation() ElevationTokens {
	return r.elevation
}

// GetTypography returns the current typography token set.
func (r *Registry) GetTypography() TypographyTokens {
	return r.typography
}

// GetIcons returns the current icon token set.
func (r *Registry) GetIcons() IconTokens {
	return r.icons
}

// GetDensity returns the current density mode.
func (r *Registry) GetDensity() DensityMode {
	r.densityMu.RLock()
	defer r.densityMu.RUnlock()
	return r.density
}

// SetDensity sets the current density mode.
func (r *Registry) SetDensity(mode DensityMode) {
	r.densityMu.Lock()
	defer r.densityMu.Unlock()

	if mode == DensityFocus || mode == DensityOperator || mode == DensityOrchestration {
		r.density = mode
		r.logger.Debug("density mode changed", "mode", string(mode))
	} else {
		r.logger.Warn("invalid density mode", "requested", string(mode))
	}
}

// GetDensitySettings returns the settings for the current density mode.
func (r *Registry) GetDensitySettings() DensitySettings {
	return NewDensitySettings(r.GetDensity())
}

// SetColors replaces the color token set.
func (r *Registry) SetColors(colors ColorTokens) error {
	if err := colors.ValidateColors(); err != nil {
		return err
	}
	r.colors = colors
	r.logger.Debug("color tokens updated")
	return nil
}

// SetTypography replaces the typography token set.
func (r *Registry) SetTypography(typography TypographyTokens) error {
	if err := typography.ValidateTypography(); err != nil {
		return err
	}
	r.typography = typography
	r.logger.Debug("typography tokens updated")
	return nil
}

// Summary returns a human-readable summary of the current tokens.
func (r *Registry) Summary() string {
	density := r.GetDensity()
	return "Design System (Tokens)\n" +
		"  Colors: " + string(density) + " mode\n" +
		"  Spacing: " + "8 values (4-48px)\n" +
		"  Radius: compact(8) standard(12) large(16)\n" +
		"  Elevation: 3 levels (0-2)\n" +
		"  Typography: 7 WebUI + 4 TUI roles\n" +
		"  Icons: 8 semantic roles"
}
