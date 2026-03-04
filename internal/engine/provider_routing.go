package engine

import (
	"context"
	"fmt"

	"github.com/velariumai/gorkbot/internal/arc"
	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/discovery"
	"github.com/velariumai/gorkbot/pkg/providers"
)

// ProvMgr is the provider lifecycle manager (set from main.go).
// Declared here to extend Orchestrator without modifying orchestrator.go.
// NOTE: this is stored via SetProviderManager() to avoid struct change breakage.
var globalProvMgr *providers.Manager

// SetProviderManager stores the global provider manager for orchestrator access.
func SetProviderManager(pm *providers.Manager) {
	globalProvMgr = pm
}

// GetProviderManager returns the global provider manager.
func GetProviderManager() *providers.Manager {
	return globalProvMgr
}

// ─── Orchestrator extension methods ──────────────────────────────────────────

// SetPrimary hot-swaps the primary AI provider.
// Returns an error string if the provider/model cannot be resolved.
func (o *Orchestrator) SetPrimary(ctx context.Context, providerName, modelID string) error {
	pm := globalProvMgr
	if pm == nil {
		// Fallback: if no ProvMgr, try the existing base provider
		if o.Primary == nil {
			return fmt.Errorf("no provider manager and no current primary")
		}
		if modelID != "" {
			o.Primary = o.Primary.WithModel(modelID)
		}
		return nil
	}
	prov, err := pm.GetProviderForModel(providerName, modelID)
	if err != nil {
		return fmt.Errorf("SetPrimary: %w", err)
	}
	o.Primary = prov
	o.primaryModelName = prov.Name() + "/" + modelID
	if o.Registry != nil {
		o.Registry.SetAIProvider(prov)
	}
	if o.Logger != nil {
		o.Logger.Info("Primary provider switched", "provider", providerName, "model", modelID)
	}
	return nil
}

// SetSecondary hot-swaps the consultant (secondary) AI provider.
func (o *Orchestrator) SetSecondary(ctx context.Context, providerName, modelID string) error {
	pm := globalProvMgr
	if pm == nil {
		if o.Consultant == nil {
			return fmt.Errorf("no provider manager and no current consultant")
		}
		if modelID != "" {
			o.Consultant = o.Consultant.WithModel(modelID)
		}
		return nil
	}
	prov, err := pm.GetProviderForModel(providerName, modelID)
	if err != nil {
		return fmt.Errorf("SetSecondary: %w", err)
	}
	o.Consultant = prov
	// Keep the tool registry in sync so consultation tool always uses the
	// current secondary model (covers both UI switches and cascade failover).
	if o.Registry != nil {
		o.Registry.SetConsultantProvider(prov)
	}
	if o.Stabilizer != nil {
		o.Stabilizer = nil // re-init handled lazily
	}
	if o.Logger != nil {
		o.Logger.Info("Secondary provider switched", "provider", providerName, "model", modelID)
	}
	return nil
}

// ResolveConsultant returns the consultant provider to use for a given task.
// When autoSecondary is true (Consultant == nil), it uses ARC + discovery to
// select the best available secondary model. Otherwise it returns o.Consultant.
func (o *Orchestrator) ResolveConsultant(ctx context.Context, task string) interface{} {
	if o.Consultant != nil {
		return o.Consultant
	}
	res := o.intelligentSecondarySelect(ctx, task)
	if res == nil {
		return nil
	}
	return res
}

// intelligentSecondarySelect uses ARC routing + discovery to pick the best
// secondary model that is (a) different from the primary and (b) has a valid key.
// When ARC returns CostTierCheap, prefers mini/flash/haiku variants.
func (o *Orchestrator) intelligentSecondarySelect(ctx context.Context, task string) ai.AIProvider {
	if o.Discovery == nil || globalProvMgr == nil {
		return nil
	}

	// Classify the task with ARC if available
	cap := discovery.CapReasoning // default to reasoning for consultant queries
	preferCheap := false
	if o.Intelligence != nil {
		rd := o.Intelligence.Router.Route(task)
		switch rd.Budget.CostTier {
		case arc.CostTierCheap:
			cap = discovery.CapSpeed // cheap tasks → fast/cheap models
			preferCheap = true
		case arc.CostTierPremium:
			cap = discovery.CapReasoning
		default:
			cap = discovery.CapGeneral
		}
	}

	best := o.Discovery.BestForCap(cap, "")
	if best == nil {
		best = o.Discovery.BestForCap(discovery.CapGeneral, "")
	}
	if best == nil {
		return nil
	}

	// For cheap tier, prefer a cheap model if available
	if preferCheap && !providers.IsCheapModel(best.ID) {
		allModels := o.Discovery.Models()
		for i, m := range allModels {
			if providers.IsCheapModel(m.ID) && m.ID != best.ID {
				best = &allModels[i]
				break
			}
		}
	}

	// Avoid using the same model as primary
	primaryID := ""
	if o.Primary != nil {
		primaryID = o.Primary.GetMetadata().ID
	}
	if best.ID == primaryID {
		return nil
	}

	prov, err := globalProvMgr.GetProviderForModel(best.Provider, best.ID)
	if err != nil {
		if o.Logger != nil {
			o.Logger.Warn("intelligentSecondarySelect: provider unavailable",
				"provider", best.Provider, "model", best.ID, "error", err)
		}
		return nil
	}
	return prov
}

// SetProviderKey stores a new API key for the given provider and returns a status string.
func (o *Orchestrator) SetProviderKey(ctx context.Context, providerName, key string) string {
	pm := globalProvMgr
	if pm == nil {
		return fmt.Sprintf("Provider manager not initialized. Set %s manually in .env", providers.ProviderName(providerName))
	}
	if err := pm.SetKey(ctx, providerName, key, true); err != nil {
		return fmt.Sprintf("Key validation failed for %s: %v", providers.ProviderName(providerName), err)
	}
	// Re-poll discovery with the new key
	if o.Discovery != nil {
		o.Discovery.Start(ctx)
	}
	return fmt.Sprintf("%s API key saved and validated.", providers.ProviderName(providerName))
}

// GetProviderStatus returns a formatted status summary of all providers.
func (o *Orchestrator) GetProviderStatus() string {
	pm := globalProvMgr
	if pm == nil {
		return "Provider manager not initialized."
	}
	return pm.KeyStore().FormatStatus()
}
