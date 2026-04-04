package providers

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/embeddings"
	"github.com/velariumai/gorkbot/pkg/registry"
)

// Manager creates and caches one base AIProvider instance per provider.
// Hot-swapping the active model is achieved via base.WithModel(id).
type Manager struct {
	keys   *KeyStore
	bases  map[string]ai.AIProvider
	mu     sync.RWMutex
	logger *slog.Logger
	cache  *SemanticCache

	verboseThoughts bool // forwarded to Gemini

	// Model confidence tracking via EWMA (α=0.1).
	// Key: modelID, value: failure rate 0.0 (perfect) → 1.0 (always fails).
	failureRates map[string]float64
	failureMu    sync.RWMutex

	// Session-level disable: cleared on restart, not persisted.
	sessionDisabled map[string]bool
	sdMu            sync.RWMutex
}

// NewManager creates a Manager from the given KeyStore and initialises any
// provider whose key is already present.
func NewManager(keys *KeyStore, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}

	// Initialize semantic cache backed by Ollama Nomic embedder.
	// configDir is derived from the KeyStore so no extra parameter is needed.
	embedder := embeddings.NewOllamaEmbedder("", "")
	cache, err := NewSemanticCache(embedder, keys.Dir())
	if err != nil {
		logger.Warn("providers.Manager: failed to init semantic cache", "error", err)
	}

	m := &Manager{
		keys:            keys,
		bases:           make(map[string]ai.AIProvider),
		logger:          logger,
		cache:           cache,
		failureRates:    make(map[string]float64),
		sessionDisabled: make(map[string]bool),
	}
	for _, p := range AllProviders() {
		m.initProvider(p)
	}
	return m
}

// SetVerboseThoughts sets the verbose-thoughts flag (Gemini-specific).
func (m *Manager) SetVerboseThoughts(v bool) {
	m.mu.Lock()
	m.verboseThoughts = v
	m.mu.Unlock()
	// Re-init Gemini if already present to apply new flag
	m.InitProvider(ProviderGoogle)
}

// InitProvider (re)creates the base instance for the given provider using the
// current key from the KeyStore. A missing or empty key is a no-op.
func (m *Manager) InitProvider(provider string) {
	key, _ := m.keys.Get(provider)
	if key == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	base := m.buildBase(provider, key)
	if base != nil {
		m.bases[provider] = NewWrappedProvider(base, m.cache)
	}
}

// initProvider is the lock-free inner version called during construction.
func (m *Manager) initProvider(provider string) {
	key, _ := m.keys.Get(provider)
	if key == "" {
		return
	}
	base := m.buildBase(provider, key)
	if base != nil {
		m.bases[provider] = NewWrappedProvider(base, m.cache)
	}
}

// buildBase constructs the correct AIProvider for the given provider/key pair.
func (m *Manager) buildBase(provider, key string) ai.AIProvider {
	switch provider {
	case ProviderXAI:
		return ai.NewGrokProvider(key, "")
	case ProviderGoogle:
		return ai.NewGeminiProvider(key, "", m.verboseThoughts)
	case ProviderAnthropic:
		return ai.NewAnthropicProvider(key, "")
	case ProviderOpenAI:
		return ai.NewOpenAIProvider(key, "")
	case ProviderMiniMax:
		return ai.NewMiniMaxProvider(key, "")
	case ProviderOpenRouter:
		return ai.NewOpenRouterProvider(key, "")
	case ProviderMoonshot:
		return ai.NewMoonshotProvider(key, "")
	default:
		m.logger.Warn("providers.Manager: unknown provider", "provider", provider)
		return nil
	}
}

// DisableForSession marks a provider as session-disabled (not persisted).
func (m *Manager) DisableForSession(id string) {
	m.sdMu.Lock()
	m.sessionDisabled[id] = true
	m.sdMu.Unlock()
}

// EnableForSession clears a provider's session-disabled state.
func (m *Manager) EnableForSession(id string) {
	m.sdMu.Lock()
	delete(m.sessionDisabled, id)
	m.sdMu.Unlock()
}

// IsSessionDisabled returns true if the provider is disabled for this session.
func (m *Manager) IsSessionDisabled(id string) bool {
	m.sdMu.RLock()
	v := m.sessionDisabled[id]
	m.sdMu.RUnlock()
	return v
}

// GetBase returns the base instance for the provider, or an error if unavailable.
// Returns an error if the provider is session-disabled.
func (m *Manager) GetBase(provider string) (ai.AIProvider, error) {
	if m.IsSessionDisabled(provider) {
		return nil, fmt.Errorf("provider %q is disabled for this session", provider)
	}
	m.mu.RLock()
	base, ok := m.bases[provider]
	m.mu.RUnlock()
	if !ok || base == nil {
		return nil, fmt.Errorf("provider %q unavailable (no API key?)", provider)
	}
	return base, nil
}

// GetProviderForModel returns a provider instance configured for the given model.
func (m *Manager) GetProviderForModel(provider, modelID string) (ai.AIProvider, error) {
	base, err := m.GetBase(provider)
	if err != nil {
		return nil, err
	}
	if modelID == "" {
		return base, nil
	}
	return base.WithModel(modelID), nil
}

// SetKey stores a new key, re-initialises the provider, and optionally validates it.
// Pass validate=true to Ping the provider and update the key status.
func (m *Manager) SetKey(ctx context.Context, provider, key string, validate bool) error {
	if err := m.keys.Set(provider, key); err != nil {
		return err
	}
	m.InitProvider(provider)
	if !validate {
		return nil
	}
	base, err := m.GetBase(provider)
	if err != nil {
		return err
	}
	return m.keys.Validate(ctx, provider, base, m.logger)
}

// ListAvailableModels polls all providers with valid/unverified keys and returns a
// map of provider → model definitions.  Polling is best-effort; failures are logged.
func (m *Manager) ListAvailableModels(ctx context.Context) map[string][]registry.ModelDefinition {
	result := make(map[string][]registry.ModelDefinition)
	m.mu.RLock()
	bases := make(map[string]ai.AIProvider, len(m.bases))
	for k, v := range m.bases {
		bases[k] = v
	}
	m.mu.RUnlock()

	for provider, base := range bases {
		models, err := base.FetchModels(ctx)
		if err != nil {
			m.logger.Warn("providers.Manager: FetchModels failed", "provider", provider, "error", err)
			continue
		}
		result[provider] = models
	}
	return result
}

// PollProvider refreshes the model list for a single provider on demand.
func (m *Manager) PollProvider(ctx context.Context, provider string) ([]registry.ModelDefinition, error) {
	base, err := m.GetBase(provider)
	if err != nil {
		return nil, err
	}
	return base.FetchModels(ctx)
}

// KeyStore returns the underlying KeyStore for direct access.
func (m *Manager) KeyStore() *KeyStore { return m.keys }

// RecordOutcome updates the EWMA failure rate for the given model.
// failed=true means the generation returned an error or empty result.
// α=0.1 gives a ~10-sample rolling window.
func (m *Manager) RecordOutcome(modelID string, failed bool) {
	const alpha = 0.1
	m.failureMu.Lock()
	defer m.failureMu.Unlock()
	cur, exists := m.failureRates[modelID]
	if !exists {
		cur = 0
	}
	var obs float64
	if failed {
		obs = 1.0
	}
	m.failureRates[modelID] = alpha*obs + (1-alpha)*cur
}

// FailureRate returns the current EWMA failure rate for modelID (0.0–1.0).
// Returns 0.0 for unknown models (assume healthy until observed otherwise).
func (m *Manager) FailureRate(modelID string) float64 {
	m.failureMu.RLock()
	defer m.failureMu.RUnlock()
	return m.failureRates[modelID]
}

// ConfidenceReport returns a formatted summary of model failure rates.
func (m *Manager) ConfidenceReport() string {
	m.failureMu.RLock()
	defer m.failureMu.RUnlock()
	if len(m.failureRates) == 0 {
		return "No model confidence data yet."
	}
	var sb strings.Builder
	sb.WriteString("Model Reliability (EWMA failure rate):\n")
	for id, rate := range m.failureRates {
		bar := "✓"
		if rate > 0.5 {
			bar = "✗"
		} else if rate > 0.2 {
			bar = "~"
		}
		sb.WriteString(fmt.Sprintf("  %s  %s  %.1f%%\n", bar, id, rate*100))
	}
	return sb.String()
}

// ─── Global manager singleton (set from main.go / engine) ────────────────────

var globalManager *Manager

// SetGlobalProviderManager stores the process-wide provider manager.
func SetGlobalProviderManager(pm *Manager) {
	globalManager = pm
}

// GetGlobalProviderManager returns the process-wide provider manager (may be nil).
func GetGlobalProviderManager() *Manager {
	return globalManager
}

// ProviderName returns a display name for a provider ID.
func ProviderName(id string) string {
	switch id {
	case ProviderXAI:
		return "xAI"
	case ProviderGoogle:
		return "Google"
	case ProviderAnthropic:
		return "Anthropic"
	case ProviderOpenAI:
		return "OpenAI"
	case ProviderMiniMax:
		return "MiniMax"
	case ProviderOpenRouter:
		return "OpenRouter"
	case ProviderMoonshot:
		return "Moonshot"
	default:
		if id == "" {
			return id
		}
		return strings.ToUpper(id[:1]) + id[1:]
	}
}

// ProviderWebsite returns the API key console URL for a provider.
func ProviderWebsite(id string) string {
	switch id {
	case ProviderXAI:
		return "console.x.ai"
	case ProviderGoogle:
		return "aistudio.google.com"
	case ProviderAnthropic:
		return "console.anthropic.com/settings/keys"
	case ProviderOpenAI:
		return "platform.openai.com/api-keys"
	case ProviderMiniMax:
		return "platform.minimaxi.com/user-center/basic-information"
	case ProviderOpenRouter:
		return "openrouter.ai/settings/keys"
	case ProviderMoonshot:
		return "platform.moonshot.cn/console/api-keys"
	default:
		return ""
	}
}
