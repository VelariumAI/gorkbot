package providers

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/velariumai/gorkbot/internal/config"
)

// ProviderFactory creates and caches AI provider instances
type ProviderFactory struct {
	config   *config.GorkbotConfig
	cache    map[string]AIProvider
	mu       sync.RWMutex
	logger   *slog.Logger
	creators map[string]CreatorFunc
}

// CreatorFunc is a function that creates a provider instance
type CreatorFunc func(cfg *config.ProviderConfig, logger *slog.Logger) (AIProvider, error)

// NewProviderFactory creates a new provider factory
func NewProviderFactory(cfg *config.GorkbotConfig, logger *slog.Logger) *ProviderFactory {
	if logger == nil {
		logger = slog.Default()
	}

	pf := &ProviderFactory{
		config:   cfg,
		cache:    make(map[string]AIProvider),
		logger:   logger,
		creators: make(map[string]CreatorFunc),
	}

	// Register default provider creators
	pf.registerDefaultCreators()

	return pf
}

// registerDefaultCreators registers creator functions for known providers
func (pf *ProviderFactory) registerDefaultCreators() {
	// Register built-in provider constructors.
	pf.RegisterCreator("anthropic", newAnthropicProvider)
	pf.RegisterCreator("openai", newOpenAIProvider)
	pf.RegisterCreator("google", newGoogleProvider)
	pf.RegisterCreator("bedrock", newBedrockProvider)
	pf.RegisterCreator("azure", newAzureProvider)
	pf.RegisterCreator("ollama", newOllamaProvider)
}

// RegisterCreator registers a custom provider creator
func (pf *ProviderFactory) RegisterCreator(name string, creator CreatorFunc) {
	pf.mu.Lock()
	defer pf.mu.Unlock()
	pf.creators[name] = creator
}

// CreateProvider creates or returns cached provider instance
func (pf *ProviderFactory) CreateProvider(name string) (AIProvider, error) {
	// Check cache first
	pf.mu.RLock()
	if provider, ok := pf.cache[name]; ok {
		pf.mu.RUnlock()
		return provider, nil
	}
	pf.mu.RUnlock()

	// Get provider configuration
	providerCfg, ok := pf.config.Providers[name]
	if !ok {
		return nil, fmt.Errorf("provider '%s' not configured", name)
	}

	// Get creator function
	pf.mu.RLock()
	creator, ok := pf.creators[name]
	pf.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("provider '%s' has no registered creator", name)
	}

	// Create provider
	provider, err := creator(&providerCfg, pf.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider '%s': %w", name, err)
	}

	// Cache it
	pf.mu.Lock()
	pf.cache[name] = provider
	pf.mu.Unlock()

	pf.logger.Debug("created provider instance", slog.String("provider", name))

	return provider, nil
}

// SelectProvider selects the best provider for a capability based on routing
func (pf *ProviderFactory) SelectProvider(ctx context.Context, capability string) (AIProvider, error) {
	// Get provider name from config for this capability
	var providerName string
	switch capability {
	case "thinking":
		providerName = pf.config.Capabilities.ThinkingProvider
	case "coding":
		providerName = pf.config.Capabilities.CodingProvider
	case "vision":
		providerName = pf.config.Capabilities.VisionProvider
	case "memory":
		providerName = pf.config.Capabilities.MemoryProvider
	case "specialist":
		providerName = pf.config.Capabilities.SpecialistProvider
	default:
		providerName = pf.config.Capabilities.DefaultProvider
	}

	if providerName == "" {
		providerName = pf.config.Capabilities.DefaultProvider
	}

	if providerName == "" {
		return nil, fmt.Errorf("no provider available for capability: %s", capability)
	}

	// Try to create/get provider
	provider, err := pf.CreateProvider(providerName)
	if err != nil {
		// Try fallback chain
		fallbacks := pf.GetFallbacks(providerName)
		for _, fallback := range fallbacks {
			provider, err = pf.CreateProvider(fallback)
			if err == nil {
				pf.logger.Warn("using fallback provider",
					slog.String("primary", providerName),
					slog.String("fallback", fallback))
				return provider, nil
			}
		}
		return nil, fmt.Errorf("no provider available for capability '%s': %w", capability, err)
	}

	return provider, nil
}

// GetProvider returns a provider by name (from cache if available)
func (pf *ProviderFactory) GetProvider(name string) (AIProvider, error) {
	return pf.CreateProvider(name)
}

// GetFallbacks returns the fallback provider chain for a given provider
func (pf *ProviderFactory) GetFallbacks(provider string) []string {
	key := provider + "_fallback"
	return pf.config.Routing.Fallback[key]
}

// ListProviders returns all configured provider names
func (pf *ProviderFactory) ListProviders() []string {
	var names []string
	for name := range pf.config.Providers {
		names = append(names, name)
	}
	return names
}

// HealthCheckAll checks health of all cached providers
func (pf *ProviderFactory) HealthCheckAll(ctx context.Context) map[string]error {
	pf.mu.RLock()
	defer pf.mu.RUnlock()

	results := make(map[string]error)
	for name, provider := range pf.cache {
		results[name] = provider.HealthCheck(ctx)
	}
	return results
}

// ClearCache clears the provider cache
func (pf *ProviderFactory) ClearCache() {
	pf.mu.Lock()
	defer pf.mu.Unlock()

	for name, provider := range pf.cache {
		if err := provider.Close(); err != nil {
			pf.logger.Error("error closing provider",
				slog.String("provider", name),
				slog.String("error", err.Error()))
		}
	}

	pf.cache = make(map[string]AIProvider)
}

// Close closes all cached providers
func (pf *ProviderFactory) Close() error {
	pf.ClearCache()
	return nil
}

// Provider creator function implementations (real providers)

func newAnthropicProvider(cfg *config.ProviderConfig, logger *slog.Logger) (AIProvider, error) {
	return NewAnthropicProvider(cfg, logger)
}

func newOpenAIProvider(cfg *config.ProviderConfig, logger *slog.Logger) (AIProvider, error) {
	return NewOpenAIProvider(cfg, logger)
}

func newGoogleProvider(cfg *config.ProviderConfig, logger *slog.Logger) (AIProvider, error) {
	return NewGoogleProvider(cfg, logger)
}

func newBedrockProvider(cfg *config.ProviderConfig, logger *slog.Logger) (AIProvider, error) {
	return NewBedrockProvider(cfg, logger)
}

func newAzureProvider(cfg *config.ProviderConfig, logger *slog.Logger) (AIProvider, error) {
	return NewAzureProvider(cfg, logger)
}

func newOllamaProvider(cfg *config.ProviderConfig, logger *slog.Logger) (AIProvider, error) {
	return NewOllamaProvider(cfg, logger)
}

// Config returns provider configuration for validation/inspection
func (pf *ProviderFactory) Config() *config.GorkbotConfig {
	return pf.config
}

// SyncCache reloads providers from updated configuration (for hot-reload)
func (pf *ProviderFactory) SyncCache(newCfg *config.GorkbotConfig) error {
	pf.mu.Lock()
	defer pf.mu.Unlock()

	// Check what changed
	// For now, simple approach: clear cache if config differs
	oldNames := make(map[string]bool)
	newNames := make(map[string]bool)

	for name := range pf.config.Providers {
		oldNames[name] = true
	}

	for name := range newCfg.Providers {
		newNames[name] = true
	}

	// If any providers were removed or changed, clear affected cache entries
	for name := range oldNames {
		if _, ok := newNames[name]; !ok {
			// Provider was removed
			if provider, ok := pf.cache[name]; ok {
				provider.Close()
				delete(pf.cache, name)
			}
		}
	}

	// Update config
	pf.config = newCfg

	pf.logger.Debug("synced provider factory cache with new configuration")
	return nil
}

// GetCacheStats returns statistics about cached providers
func (pf *ProviderFactory) GetCacheStats() map[string]interface{} {
	pf.mu.RLock()
	defer pf.mu.RUnlock()

	return map[string]interface{}{
		"cached_providers": len(pf.cache),
		"total_providers":  len(pf.config.Providers),
		"cache_keys":       getMapKeys(pf.cache),
	}
}

func getMapKeys(m map[string]AIProvider) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ReloadProviders reloads all providers from environment/config
func (pf *ProviderFactory) ReloadProviders() error {
	// Re-validate all configured providers
	for name, providerCfg := range pf.config.Providers {
		if err := config.ValidateProviderConfig(name, &providerCfg); err != nil {
			pf.logger.Error("provider validation failed",
				slog.String("provider", name),
				slog.String("error", err.Error()))
			return err
		}

		// Check API keys are available (from env)
		if providerCfg.APIKey != "" {
			// Key is in config (already set)
			continue
		}

		// Try to get from environment
		envKey := fmt.Sprintf("GORKBOT_%s_API_KEY", name)
		if key := os.Getenv(envKey); key != "" {
			providerCfg.APIKey = key
		}
	}

	pf.logger.Debug("reloaded provider configurations")
	return nil
}
