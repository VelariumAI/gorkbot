package tui

// model_extensions.go adds new methods to Model without touching the original
// model.go (which contains the touch-scroll logic that must not be disturbed).
// Go allows a type's methods to be spread across multiple files in the same package.

import (
	"github.com/velariumai/gorkbot/pkg/commands"
	"github.com/velariumai/gorkbot/pkg/discovery"
)

// GetCommandRegistry returns the command registry so main.go can wire up
// orchestrator adapters, skill loaders, and rule engines after model creation.
func (m *Model) GetCommandRegistry() *commands.Registry {
	return m.commands
}

// SetExecutionMode updates the mode badge displayed in the status bar.
func (m *Model) SetExecutionMode(modeName string) {
	m.statusBar.SetMode(modeName)
}

// UpdateContextStats pushes context window and cost info to the status bar.
func (m *Model) UpdateContextStats(usedPct float64, costUSD float64) {
	m.statusBar.UpdateContext(usedPct, costUSD)
}

// SetDiscoveryManager wires a discovery.Manager into the TUI.
// It subscribes to model-list updates, converts them to the internal
// discoveryModel type, and forwards them to m.discoverySub so the Update()
// loop can refresh the Cloud Brains tab without touching model.go directly.
func (m *Model) SetDiscoveryManager(dm *discovery.Manager) {
	if dm == nil {
		return
	}
	raw := dm.Subscribe()
	ch := make(chan []discoveryModel, 1)
	m.discoverySub = ch

	// Seed immediately with whatever is already cached.
	if seed := dm.Models(); len(seed) > 0 {
		m.discoveredModels = convertModels(seed)
	}

	// Bridge goroutine: convert discovery.DiscoveredModel → discoveryModel.
	// This goroutine is cheap (only fires on 30-min poll boundaries) and
	// does not hold any TUI state — it just forwards converted snapshots.
	go func() {
		for models := range raw {
			converted := convertModels(models)
			select {
			case ch <- converted:
			default:
			}
		}
	}()
}

func convertModels(src []discovery.DiscoveredModel) []discoveryModel {
	out := make([]discoveryModel, len(src))
	for i, m := range src {
		out[i] = discoveryModel{
			ID:       m.ID,
			Name:     m.Name,
			Provider: m.Provider,
			BestCap:  m.BestCap().String(),
		}
	}
	return out
}
