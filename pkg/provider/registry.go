package provider

import (
	"fmt"
	"strings"
	"sync"

	"github.com/velariumai/gorkbot/pkg/ai"
)

// ErrUnknownProvider indicates that a provider key could not be resolved.
var ErrUnknownProvider = fmt.Errorf("unknown provider")

// AIFactoryParams holds the configuration passed to AI provider factories.
type AIFactoryParams struct {
	APIKey           string
	Model            string
	BaseURL          string
	MaxTokens        int
	Temperature      float64
	SupportsThinking bool
	VerboseThoughts  bool
	CustomFields     map[string]interface{}
}

// AIProviderFactory creates an AIProvider from the given parameters.
type AIProviderFactory func(params AIFactoryParams) (ai.AIProvider, error)

// SandboxProviderFactory creates a SandboxProvider from custom parameters.
type SandboxProviderFactory func(params map[string]interface{}) (SandboxProvider, error)

// GuardrailsProviderFactory creates a GuardrailsProvider from custom parameters.
type GuardrailsProviderFactory func(params map[string]interface{}) (GuardrailsProvider, error)

// FactoryRegistry holds the registered factory functions for all provider types.
type FactoryRegistry struct {
	mu          sync.RWMutex
	aiFactories map[string]AIProviderFactory
	sbFactories map[string]SandboxProviderFactory
	grFactories map[string]GuardrailsProviderFactory
}

// NewFactoryRegistry creates an empty registry.
func NewFactoryRegistry() *FactoryRegistry {
	return &FactoryRegistry{
		aiFactories: make(map[string]AIProviderFactory),
		sbFactories: make(map[string]SandboxProviderFactory),
		grFactories: make(map[string]GuardrailsProviderFactory),
	}
}

// RegisterAI registers an AI provider factory. Panics if the key is already registered.
func (r *FactoryRegistry) RegisterAI(key string, factory AIProviderFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.aiFactories[key]; exists {
		panic(fmt.Sprintf("AI provider factory already registered: %s", key))
	}
	r.aiFactories[key] = factory
}

// RegisterSandbox registers a sandbox provider factory. Panics if the key is already registered.
func (r *FactoryRegistry) RegisterSandbox(key string, factory SandboxProviderFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.sbFactories[key]; exists {
		panic(fmt.Sprintf("Sandbox provider factory already registered: %s", key))
	}
	r.sbFactories[key] = factory
}

// RegisterGuardrails registers a guardrails provider factory. Panics if the key is already registered.
func (r *FactoryRegistry) RegisterGuardrails(key string, factory GuardrailsProviderFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.grFactories[key]; exists {
		panic(fmt.Sprintf("Guardrails provider factory already registered: %s", key))
	}
	r.grFactories[key] = factory
}

// ResolveAI instantiates an AI provider using the registered factory for the given key.
// Returns ErrUnknownProvider if the key is not registered.
func (r *FactoryRegistry) ResolveAI(key string, params AIFactoryParams) (ai.AIProvider, error) {
	r.mu.RLock()
	factory, ok := r.aiFactories[key]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownProvider, key)
	}
	return factory(params)
}

// ResolveSandbox instantiates a sandbox provider using the registered factory for the given key.
// Returns ErrUnknownProvider if the key is not registered.
func (r *FactoryRegistry) ResolveSandbox(key string, params map[string]interface{}) (SandboxProvider, error) {
	r.mu.RLock()
	factory, ok := r.sbFactories[key]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownProvider, key)
	}
	return factory(params)
}

// ResolveGuardrails instantiates a guardrails provider using the registered factory for the given key.
// Returns ErrUnknownProvider if the key is not registered.
func (r *FactoryRegistry) ResolveGuardrails(key string, params map[string]interface{}) (GuardrailsProvider, error) {
	r.mu.RLock()
	factory, ok := r.grFactories[key]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownProvider, key)
	}
	return factory(params)
}

// ListAIKeys returns all registered AI provider keys.
func (r *FactoryRegistry) ListAIKeys() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	keys := make([]string, 0, len(r.aiFactories))
	for k := range r.aiFactories {
		keys = append(keys, k)
	}
	return keys
}

// ParseUseKey parses a Use string in the format "pkg.namespace:ProviderName" and returns (namespace, provider).
// Invalid formats return empty strings and an error.
func ParseUseKey(useStr string) (namespace, provider string, err error) {
	parts := strings.SplitN(useStr, ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid Use key format: %q (expected 'pkg.namespace:ProviderName')", useStr)
	}
	namespace = strings.TrimSpace(parts[0])
	provider = strings.TrimSpace(parts[1])
	if namespace == "" || provider == "" {
		return "", "", fmt.Errorf("invalid Use key format: %q (namespace and provider must be non-empty)", useStr)
	}
	return namespace, provider, nil
}

// ─── Global DefaultRegistry singleton ──────────────────────────────────────

// DefaultRegistry is the global provider factory registry, initialized at startup.
var DefaultRegistry = NewFactoryRegistry()
