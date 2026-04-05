package plugins

import "github.com/velariumai/gorkbot/pkg/tools"

// GorkPlugin is the interface every plugin .so must export as symbol "GorkPlugin".
// Plugin authors implement this interface to provide tools to Gorkbot.
type GorkPlugin interface {
	// Name returns the plugin identifier.
	Name() string

	// Version returns the semantic version (e.g., "1.0.0").
	Version() string

	// Init initializes the plugin and registers its tools via the provided registrar.
	// Called during plugin load with a ToolRegistrar to register tools.
	Init(reg ToolRegistrar) error

	// Shutdown performs cleanup (close connections, flush state, etc).
	// Called during plugin unload or orchestrator shutdown.
	Shutdown() error
}

// ToolRegistrar is the minimal interface plugins use to register tools.
type ToolRegistrar interface {
	// Register adds a tool to the registry.
	Register(tool tools.Tool) error
}

// toolRegistrar is a concrete implementation of ToolRegistrar.
type toolRegistrar struct {
	registry *tools.Registry
}

// NewToolRegistrar creates a ToolRegistrar for plugin authors.
func NewToolRegistrar(registry *tools.Registry) ToolRegistrar {
	return &toolRegistrar{
		registry: registry,
	}
}

// Register delegates to the real registry.
func (tr *toolRegistrar) Register(tool tools.Tool) error {
	if tr.registry == nil {
		return ErrNilRegistry
	}
	return tr.registry.Register(tool)
}
