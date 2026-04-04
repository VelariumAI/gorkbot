package provider

import (
	"fmt"

	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/config"
)

// RegisterAll registers all built-in providers: AI, Sandbox, and Guardrails.
func RegisterAll() {
	RegisterAIProviders(DefaultRegistry)
	RegisterSandboxProviders(DefaultRegistry)
	RegisterGuardrailsProviders(DefaultRegistry)
}

// RegisterSandboxProviders registers the built-in sandbox providers.
func RegisterSandboxProviders(r *FactoryRegistry) {
	r.RegisterSandbox("pkg.sandbox.local:LocalSandboxProvider", newLocalSandboxProvider)
}

// RegisterGuardrailsProviders registers the built-in guardrails providers.
func RegisterGuardrailsProviders(r *FactoryRegistry) {
	r.RegisterGuardrails("pkg.guardrails:AllowlistProvider", newAllowlistProvider)
}

// ResolveAIFromConfig instantiates an AI provider from ModelConfig using DefaultRegistry.
// Falls back to legacy behavior if cfg.Use is empty or cannot be resolved.
// verboseThoughts is forwarded to Gemini-compatible providers.
func ResolveAIFromConfig(cfg *config.ModelConfig, verboseThoughts bool) (ai.AIProvider, error) {
	return ResolveAIFromConfigWithRegistry(DefaultRegistry, cfg, verboseThoughts)
}

// ResolveAIFromConfigWithRegistry is the internal version that accepts a custom registry.
func ResolveAIFromConfigWithRegistry(reg *FactoryRegistry, cfg *config.ModelConfig, verboseThoughts bool) (ai.AIProvider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("ModelConfig is nil")
	}

	// If no Use key is specified, return error (caller should use legacy provider selection)
	if cfg.Use == "" {
		return nil, fmt.Errorf("ModelConfig.Use is empty")
	}

	params := AIFactoryParams{
		APIKey:          cfg.APIKey,
		Model:           cfg.Model,
		BaseURL:         cfg.BaseURL,
		MaxTokens:       cfg.MaxTokens,
		Temperature:     cfg.Temperature,
		VerboseThoughts: verboseThoughts,
		CustomFields:    cfg.CustomFields,
	}

	return reg.ResolveAI(cfg.Use, params)
}

// ResolveSandboxFromConfig instantiates a sandbox provider from SandboxConfig using DefaultRegistry.
// Returns (nil, nil) when cfg.Enabled == false.
// Returns an error if cfg.Use is empty or the provider cannot be resolved.
func ResolveSandboxFromConfig(cfg *config.SandboxConfig) (SandboxProvider, error) {
	return ResolveSandboxFromConfigWithRegistry(DefaultRegistry, cfg)
}

// ResolveSandboxFromConfigWithRegistry is the internal version that accepts a custom registry.
func ResolveSandboxFromConfigWithRegistry(reg *FactoryRegistry, cfg *config.SandboxConfig) (SandboxProvider, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}

	if cfg.Use == "" {
		return nil, fmt.Errorf("SandboxConfig.Use is empty")
	}

	params := cfg.CustomFields
	if params == nil {
		params = make(map[string]interface{})
	}

	return reg.ResolveSandbox(cfg.Use, params)
}

// ResolveGuardrailsFromConfig instantiates a guardrails provider from GuardrailsConfig using DefaultRegistry.
// Returns (nil, nil) when cfg.Enabled == false.
// Returns an error if cfg.Use is empty or the provider cannot be resolved.
func ResolveGuardrailsFromConfig(cfg *config.GuardrailsConfig) (GuardrailsProvider, error) {
	return ResolveGuardrailsFromConfigWithRegistry(DefaultRegistry, cfg)
}

// ResolveGuardrailsFromConfigWithRegistry is the internal version that accepts a custom registry.
func ResolveGuardrailsFromConfigWithRegistry(reg *FactoryRegistry, cfg *config.GuardrailsConfig) (GuardrailsProvider, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}

	if cfg.Use == "" {
		return nil, fmt.Errorf("GuardrailsConfig.Use is empty")
	}

	params := cfg.CustomFields
	if params == nil {
		params = make(map[string]interface{})
	}

	return reg.ResolveGuardrails(cfg.Use, params)
}
