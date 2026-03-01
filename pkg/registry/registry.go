package registry

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ModelRegistry manages the lifecycle and availability of AI models
type ModelRegistry struct {
	models    map[ModelID]ModelDefinition
	providers map[ProviderID]ModelProvider
	mu        sync.RWMutex
	logger    *slog.Logger
}

// NewModelRegistry creates a new registry instance
func NewModelRegistry(logger *slog.Logger) *ModelRegistry {
	return &ModelRegistry{
		models:    make(map[ModelID]ModelDefinition),
		providers: make(map[ProviderID]ModelProvider),
		logger:    logger,
	}
}

// RegisterProvider adds a provider to the registry and performs an initial fetch
func (r *ModelRegistry) RegisterProvider(ctx context.Context, provider ModelProvider) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := provider.ID()
	if _, exists := r.providers[id]; exists {
		return fmt.Errorf("provider %s already registered", id)
	}

	r.providers[id] = provider
	r.logger.Info("Registered provider", "id", id)

	// Initial fetch
	return r.refreshProviderLocked(ctx, provider)
}

// RefreshAll triggers an update from all registered providers in parallel
func (r *ModelRegistry) RefreshAll(ctx context.Context) {
	r.mu.Lock()
	providers := make([]ModelProvider, 0, len(r.providers))
	for _, provider := range r.providers {
		providers = append(providers, provider)
	}
	r.mu.Unlock()

	// Refresh all providers in parallel using goroutines
	var wg sync.WaitGroup
	for _, provider := range providers {
		wg.Add(1)
		go func(p ModelProvider) {
			defer wg.Done()
			r.mu.Lock()
			err := r.refreshProviderLocked(ctx, p)
			r.mu.Unlock()
			if err != nil {
				r.logger.Error("Failed to refresh provider", "id", p.ID(), "error", err)
			}
		}(provider)
	}
	wg.Wait()
}

// StartRefreshLoop starts a background worker to refresh models periodically
func (r *ModelRegistry) StartRefreshLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				r.RefreshAll(ctx)
			}
		}
	}()
}

// refreshProviderLocked fetches and updates models for a single provider
// Must be called with lock held
func (r *ModelRegistry) refreshProviderLocked(ctx context.Context, provider ModelProvider) error {
	fetchedModels, err := provider.FetchModels(ctx)
	if err != nil {
		return err
	}

	// Track seen IDs to identify deprecations
	seenIDs := make(map[ModelID]bool)

	for _, model := range fetchedModels {
		model.Status = StatusActive // Ensure active if freshly fetched
		model.LastUpdated = time.Now()
		r.models[model.ID] = model
		seenIDs[model.ID] = true
	}

	// Mark unseen models from this provider as deprecated/offline
	// Note: In a real system, we might want a grace period.
	providerID := provider.ID()
	for id, model := range r.models {
		if model.Provider == providerID && !seenIDs[id] {
			if model.Status == StatusActive {
				r.logger.Warn("Model disappeared from provider", "id", id)
				model.Status = StatusDeprecated
				r.models[id] = model
			}
		}
	}

	return nil
}

// GetModel retrieves a model definition by ID
func (r *ModelRegistry) GetModel(id ModelID) (ModelDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	model, ok := r.models[id]
	return model, ok
}

// ListActiveModels returns all currently available models
func (r *ModelRegistry) ListActiveModels() []ModelDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var active []ModelDefinition
	for _, m := range r.models {
		if m.Status == StatusActive {
			active = append(active, m)
		}
	}
	return active
}

// GetProvider retrieves a registered provider by its ID
func (r *ModelRegistry) GetProvider(id ProviderID) (ModelProvider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	provider, ok := r.providers[id]
	return provider, ok
}
