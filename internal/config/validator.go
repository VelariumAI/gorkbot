package config

import (
	"fmt"
	"strings"
)

// ValidateConfig validates the entire configuration
func ValidateConfig(cfg *GorkbotConfig) error {
	if cfg == nil {
		return fmt.Errorf("configuration is nil")
	}

	// Validate providers
	if err := validateProviders(cfg); err != nil {
		return err
	}

	// Validate capabilities
	if err := validateCapabilities(cfg); err != nil {
		return err
	}

	// Validate routing
	if err := validateRouting(cfg); err != nil {
		return err
	}

	// Validate specialist
	if err := validateSpecialist(cfg); err != nil {
		return err
	}

	// Validate optimization
	if err := validateOptimization(cfg); err != nil {
		return err
	}

	return nil
}

// validateProviders ensures all providers are properly configured
func validateProviders(cfg *GorkbotConfig) error {
	if len(cfg.Providers) == 0 {
		return fmt.Errorf("no providers configured in [providers]")
	}

	for name, provider := range cfg.Providers {
		// API key validation
		if provider.APIKey == "" && provider.BaseURL == "" {
			return fmt.Errorf("provider '%s': must have either api_key or base_url", name)
		}

		// Model validation
		if provider.Model == "" {
			return fmt.Errorf("provider '%s': model is required", name)
		}

		// Timeout validation
		if provider.Timeout <= 0 {
			return fmt.Errorf("provider '%s': timeout must be > 0", name)
		}

		// Max retries validation
		if provider.MaxRetries < 0 {
			return fmt.Errorf("provider '%s': max_retries cannot be negative", name)
		}

		// Thinking budget validation
		if provider.Thinking.Enabled && provider.Thinking.Budget <= 0 {
			return fmt.Errorf("provider '%s': thinking.budget must be > 0 when enabled", name)
		}

		// Cost validation
		if provider.CostPer1MInput < 0 || provider.CostPer1MOutput < 0 {
			return fmt.Errorf("provider '%s': cost values cannot be negative", name)
		}

		// Caching TTL validation
		if provider.Caching.Enabled && provider.Caching.TTL <= 0 {
			return fmt.Errorf("provider '%s': caching.ttl must be > 0 when enabled", name)
		}
	}

	return nil
}

// validateCapabilities ensures capability mappings are valid
func validateCapabilities(cfg *GorkbotConfig) error {
	// Check that default provider exists
	if cfg.Capabilities.DefaultProvider != "" {
		if _, ok := cfg.Providers[cfg.Capabilities.DefaultProvider]; !ok {
			return fmt.Errorf("default_provider '%s' not found in [providers]", cfg.Capabilities.DefaultProvider)
		}
	} else {
		return fmt.Errorf("default_provider must be specified in [capabilities]")
	}

	// Check that all capability providers exist (if specified)
	capabilities := map[string]string{
		"thinking_provider":   cfg.Capabilities.ThinkingProvider,
		"coding_provider":     cfg.Capabilities.CodingProvider,
		"vision_provider":     cfg.Capabilities.VisionProvider,
		"memory_provider":     cfg.Capabilities.MemoryProvider,
		"specialist_provider": cfg.Capabilities.SpecialistProvider,
	}

	for capName, providerName := range capabilities {
		if providerName != "" {
			if _, ok := cfg.Providers[providerName]; !ok {
				return fmt.Errorf("%s '%s' not found in [providers]", capName, providerName)
			}
		}
	}

	return nil
}

// validateRouting ensures routing configuration is valid
func validateRouting(cfg *GorkbotConfig) error {
	// Validate intent-based routing
	for intent, provider := range cfg.Routing.IntentClass {
		if _, ok := cfg.Providers[provider]; !ok {
			return fmt.Errorf("routing: intent '%s' -> provider '%s' not found", intent, provider)
		}
	}

	// Validate file-type routing
	for fileType, provider := range cfg.Routing.FileType {
		if _, ok := cfg.Providers[provider]; !ok {
			return fmt.Errorf("routing: file-type '%s' -> provider '%s' not found", fileType, provider)
		}
	}

	// Validate directory routing
	for directory, provider := range cfg.Routing.Directory {
		if _, ok := cfg.Providers[provider]; !ok {
			return fmt.Errorf("routing: directory '%s' -> provider '%s' not found", directory, provider)
		}
	}

	// Validate fallback chains
	for chain, providers := range cfg.Routing.Fallback {
		for _, provider := range providers {
			if _, ok := cfg.Providers[provider]; !ok {
				return fmt.Errorf("routing: fallback chain '%s' references non-existent provider '%s'", chain, provider)
			}
		}
	}

	// Validate cost optimization
	if cfg.Routing.CostOptimize.BudgetPerDay < 0 {
		return fmt.Errorf("routing: budget_per_day cannot be negative")
	}

	costWeight := cfg.Routing.CostOptimize.CostWeight
	speedWeight := cfg.Routing.CostOptimize.SpeedWeight
	sum := costWeight + speedWeight
	if sum > 1.01 || sum < 0.99 { // Allow small floating point error
		return fmt.Errorf("routing: cost_weight + speed_weight must sum to 1.0, got %.2f", sum)
	}

	return nil
}

// validateSpecialist ensures specialist configuration is valid
func validateSpecialist(cfg *GorkbotConfig) error {
	spec := cfg.Specialist

	// If disabled, no need to validate further
	if !spec.Enabled {
		return nil
	}

	// Check provider exists
	if spec.Provider != "" {
		if _, ok := cfg.Providers[spec.Provider]; !ok {
			return fmt.Errorf("specialist: provider '%s' not found in [providers]", spec.Provider)
		}
	}

	// Validate thinking budget
	if spec.ThinkingBudget <= 0 {
		return fmt.Errorf("specialist: thinking_budget must be > 0")
	}

	// Validate thresholds
	if spec.ComplexityThreshold < 1 || spec.ComplexityThreshold > 10 {
		return fmt.Errorf("specialist: complexity_threshold must be 1-10")
	}

	if spec.FilesThreshold < 1 {
		return fmt.Errorf("specialist: files_threshold must be >= 1")
	}

	if spec.DurationThreshold < 1 {
		return fmt.Errorf("specialist: duration_threshold must be >= 1")
	}

	// Validate autonomy level
	autonomyLevels := map[string]bool{
		"supervised":  true,
		"validated":   true,
		"autonomous":  true,
	}
	if !autonomyLevels[spec.AutonomyLevel] {
		return fmt.Errorf("specialist: autonomy_level must be one of: supervised, validated, autonomous")
	}

	return nil
}

// validateOptimization ensures optimization configuration is valid
func validateOptimization(cfg *GorkbotConfig) error {
	opt := cfg.Optimization

	// Budget validation
	if opt.DailyBudget < 0 {
		return fmt.Errorf("optimization: daily_budget cannot be negative")
	}

	// Weight validation
	costWeight := opt.CostWeight
	speedWeight := opt.SpeedWeight
	sum := costWeight + speedWeight
	if sum > 1.01 || sum < 0.99 { // Allow small floating point error
		return fmt.Errorf("optimization: cost_weight + speed_weight must sum to 1.0, got %.2f", sum)
	}

	// Batching validation
	if opt.Batching.Enabled && opt.Batching.Size < 2 {
		return fmt.Errorf("optimization: batching.size must be >= 2")
	}

	return nil
}

// ValidateProviderConfig checks if a single provider config is valid
func ValidateProviderConfig(name string, provider *ProviderConfig) error {
	if provider == nil {
		return fmt.Errorf("provider config is nil")
	}

	if provider.APIKey == "" && provider.BaseURL == "" {
		return fmt.Errorf("provider '%s': must have either api_key or base_url", name)
	}

	if provider.Model == "" {
		return fmt.Errorf("provider '%s': model is required", name)
	}

	if provider.Timeout <= 0 {
		return fmt.Errorf("provider '%s': timeout must be > 0", name)
	}

	return nil
}

// ValidateRoutingDecision validates a routing decision
func ValidateRoutingDecision(decision string, cfg *GorkbotConfig) error {
	if decision == "" {
		return fmt.Errorf("routing decision cannot be empty")
	}

	// Check if provider exists
	if _, ok := cfg.Providers[decision]; !ok {
		return fmt.Errorf("provider '%s' not found in configuration", decision)
	}

	return nil
}

// ValidationError represents a configuration validation error
type ValidationError struct {
	Field   string
	Message string
}

// ValidationErrors is a slice of validation errors
type ValidationErrors []ValidationError

// Error returns string representation of all validation errors
func (ve ValidationErrors) Error() string {
	var msgs []string
	for _, err := range ve {
		msgs = append(msgs, fmt.Sprintf("%s: %s", err.Field, err.Message))
	}
	return "configuration validation errors:\n  " + strings.Join(msgs, "\n  ")
}

// ValidateMulti validates configuration and returns all errors
func ValidateMulti(cfg *GorkbotConfig) ValidationErrors {
	var errors ValidationErrors

	if cfg == nil {
		return ValidationErrors{{Field: "root", Message: "configuration is nil"}}
	}

	// Check providers
	if len(cfg.Providers) == 0 {
		errors = append(errors, ValidationError{
			Field:   "providers",
			Message: "no providers configured",
		})
	}

	// Check capabilities
	if cfg.Capabilities.DefaultProvider == "" {
		errors = append(errors, ValidationError{
			Field:   "capabilities.default_provider",
			Message: "default_provider is required",
		})
	}

	// Collect all errors from individual validators
	if err := validateProviders(cfg); err != nil {
		errors = append(errors, ValidationError{
			Field:   "providers",
			Message: err.Error(),
		})
	}

	if err := validateCapabilities(cfg); err != nil {
		errors = append(errors, ValidationError{
			Field:   "capabilities",
			Message: err.Error(),
		})
	}

	if err := validateRouting(cfg); err != nil {
		errors = append(errors, ValidationError{
			Field:   "routing",
			Message: err.Error(),
		})
	}

	if err := validateSpecialist(cfg); err != nil {
		errors = append(errors, ValidationError{
			Field:   "specialist",
			Message: err.Error(),
		})
	}

	if err := validateOptimization(cfg); err != nil {
		errors = append(errors, ValidationError{
			Field:   "optimization",
			Message: err.Error(),
		})
	}

	return errors
}
