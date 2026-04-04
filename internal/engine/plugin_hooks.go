package engine

import (
	"fmt"
	"log/slog"

	"github.com/velariumai/gorkbot/internal/plugins"
)

// InitPlugins initializes the plugin system and loads all plugins from the configured directory.
// Called from Orchestrator.InitEnhancements() during startup.
// Non-fatal: if plugins fail to load, logs errors but allows startup to continue.
func (o *Orchestrator) InitPlugins() error {
	if o.Logger == nil {
		o.Logger = slog.Default()
	}

	if o.Registry == nil {
		return fmt.Errorf("orchestrator registry not initialized")
	}

	// Determine plugin directory from config or use default
	pluginDir := ""
	if o.ConfigLoader != nil {
		// ConfigLoader may provide a custom plugin dir; fallback to default if not
		pluginDir = ""
	}

	// Create module loader
	loader := plugins.NewModuleLoader(o.Logger, pluginDir)

	// Wire registry for plugin tool registration
	loader.SetToolRegistry(o.Registry)

	// Load all plugins from directory
	loadErrors := loader.LoadAll()

	// Log any errors non-fatally
	for _, err := range loadErrors {
		o.Logger.Error("plugin load error", slog.String("error", err.Error()))
	}

	// Store loader reference for later access if needed
	// (Could be stored in Orchestrator if dynamic plugin management is needed)

	o.Logger.Info("plugin system initialized", slog.Int("errors", len(loadErrors)))
	return nil
}
