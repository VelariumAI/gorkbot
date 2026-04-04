package engine

import (
	"context"
	"fmt"

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
// Delegates to the provider coordinator.
func (o *Orchestrator) SetPrimary(ctx context.Context, providerName, modelID string) error {
	if o.ProviderCoord == nil {
		return fmt.Errorf("provider coordinator not available")
	}
	return o.ProviderCoord.SetPrimary(ctx, providerName, modelID)
}

// SetSecondary hot-swaps the consultant (secondary) AI provider.
// Delegates to the provider coordinator.
func (o *Orchestrator) SetSecondary(ctx context.Context, providerName, modelID string) error {
	if o.ProviderCoord == nil {
		return fmt.Errorf("provider coordinator not available")
	}
	return o.ProviderCoord.SetSecondary(ctx, providerName, modelID)
}

// ResolveConsultant returns the consultant provider to use for a given task.
// Delegates to the provider coordinator.
func (o *Orchestrator) ResolveConsultant(ctx context.Context, task string) interface{} {
	if o.ProviderCoord == nil {
		return nil
	}
	return o.ProviderCoord.SelectConsultant(ctx, task)
}

// SetProviderKey stores a new API key for the given provider.
// Delegates to the provider coordinator.
func (o *Orchestrator) SetProviderKey(ctx context.Context, providerName, key string) string {
	if o.ProviderCoord == nil {
		return "Provider coordinator not initialized."
	}
	if err := o.ProviderCoord.SetProviderKey(ctx, providerName, key); err != nil {
		return fmt.Sprintf("Failed to set provider key: %v", err)
	}
	return fmt.Sprintf("%s API key saved and validated.", providers.ProviderName(providerName))
}

// GetProviderStatus returns a formatted status summary of all providers.
// Delegates to the provider coordinator.
func (o *Orchestrator) GetProviderStatus() string {
	if o.ProviderCoord == nil {
		return "Provider coordinator not initialized."
	}
	return o.ProviderCoord.GetProviderStatus()
}
