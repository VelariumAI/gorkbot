// Package main provides provider selection logic for agent-agnostic provider initialization.
package main

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/registry"
)

// ProviderInfo describes a registered AI provider.
type ProviderInfo struct {
	Name     string
	Provider ai.AIProvider
	HasKey   bool
}

// SelectProviders chooses primary and consultant providers based on:
// 1. User explicit override (flags or env vars)
// 2. First/second available provider with valid API key
// 3. Nil if no provider available
//
// This implements provider agnosticism - no provider is preferred over another.
func SelectProviders(
	reg *registry.ModelRegistry,
	primaryOverride string,
	consultantOverride string,
	logger *slog.Logger,
) (primary ai.AIProvider, consultant ai.AIProvider, err error) {
	if reg == nil {
		return nil, nil, fmt.Errorf("model registry is nil")
	}

	// Collect all registered providers
	allProviders := reg.ListActiveModels()
	if len(allProviders) == 0 {
		return nil, nil, fmt.Errorf("no AI providers registered - check API keys")
	}

	// Map provider names to instances for lookup
	providerMap := make(map[string]ai.AIProvider)
	var availableNames []string

	for _, modelInfo := range allProviders {
		providerName := strings.ToLower(string(modelInfo.Provider))
		if providerMap[providerName] == nil {
			// Get the actual provider instance from registry
			if p, ok := reg.GetProvider(modelInfo.Provider); ok {
				if provider, ok := p.(ai.AIProvider); ok {
					providerMap[providerName] = provider
					availableNames = append(availableNames, providerName)
				}
			}
		}
	}

	logger.Info("Available AI providers", "providers", availableNames)

	// 1. Check for explicit overrides
	if primaryOverride != "" {
		primaryLower := strings.ToLower(primaryOverride)
		if p, ok := providerMap[primaryLower]; ok {
			primary = p
			logger.Info("Using explicit primary provider", "provider", primaryOverride)
		} else {
			return nil, nil, fmt.Errorf("specified primary provider not found: %s (available: %s)", primaryOverride, strings.Join(availableNames, ", "))
		}
	}

	if consultantOverride != "" {
		consultantLower := strings.ToLower(consultantOverride)
		if p, ok := providerMap[consultantLower]; ok {
			// Ensure consultant is different from primary
			if primary != nil && p == primary {
				return nil, nil, fmt.Errorf("consultant provider must be different from primary")
			}
			consultant = p
			logger.Info("Using explicit consultant provider", "provider", consultantOverride)
		} else {
			return nil, nil, fmt.Errorf("specified consultant provider not found: %s (available: %s)", consultantOverride, strings.Join(availableNames, ", "))
		}
	}

	// 2. Auto-select primary if not overridden
	if primary == nil && len(availableNames) > 0 {
		primaryName := availableNames[0]
		primary = providerMap[primaryName]
		logger.Info("Selected first available provider as primary", "provider", primaryName)
	}

	// 3. Auto-select consultant if not overridden (must be different from primary)
	if consultant == nil && len(availableNames) > 1 && primary != nil {
		// Find first provider that's not the primary
		for _, name := range availableNames {
			p := providerMap[name]
			if p != primary {
				consultant = p
				logger.Info("Selected second available provider as consultant", "provider", name)
				break
			}
		}
	}

	// 4. If only one provider available and consultant not specified, that's ok
	if consultant == nil && consultantOverride != "" {
		return nil, nil, fmt.Errorf("consultant provider specified but not found")
	}

	if primary == nil {
		return nil, nil, fmt.Errorf("no primary provider available")
	}

	return primary, consultant, nil
}

// ValidateProviderConfig ensures the selected providers are valid.
func ValidateProviderConfig(primary, consultant ai.AIProvider, logger *slog.Logger) error {
	if primary == nil {
		return fmt.Errorf("primary provider is required")
	}

	primaryName := primary.GetMetadata().Name
	if primaryName == "" {
		return fmt.Errorf("primary provider has invalid metadata")
	}

	logger.Info("Provider configuration valid",
		"primary", primaryName,
		"has_consultant", consultant != nil,
	)

	if consultant != nil {
		consultantName := consultant.GetMetadata().Name
		if consultantName == "" {
			return fmt.Errorf("consultant provider has invalid metadata")
		}
		logger.Info("Consultant provider", "name", consultantName)
	}

	return nil
}

// GetProviderNames returns the names of the primary and consultant providers.
func GetProviderNames(primary, consultant ai.AIProvider) (primaryName, consultantName string) {
	if primary != nil {
		primaryName = primary.GetMetadata().Name
	}
	if consultant != nil {
		consultantName = consultant.GetMetadata().Name
	}
	return primaryName, consultantName
}
