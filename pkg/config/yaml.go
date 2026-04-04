// Package config — yaml.go
//
// GorkbotConfig loads and saves YAML configuration files with env-var expansion.
// The config file is typically located at <configDir>/gorkbot.yaml.
//
// Usage:
//
//	cfg, err := config.LoadYAMLConfig(env.ConfigDir)
//	if err != nil { ... }
//	// cfg.Model, cfg.Sandbox, cfg.Guardrails are now populated from YAML
//
// Environment variable expansion:
//
//	model:
//	  use: ${MODEL_PROVIDER}  # expanded to env var MODEL_PROVIDER
//	  api_key: $ANTHROPIC_KEY # also works with $VAR syntax
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// GorkbotConfig represents the top-level gorkbot.yaml configuration.
type GorkbotConfig struct {
	Model      ModelConfig      `yaml:"model"`
	Sandbox    SandboxConfig    `yaml:"sandbox"`
	Guardrails GuardrailsConfig `yaml:"guardrails"`
}

// LoadYAMLConfig reads and parses <configDir>/gorkbot.yaml.
// If the file does not exist, returns &GorkbotConfig{}, nil (zero config is valid).
// All string values undergo os.ExpandEnv expansion for ${VAR} and $VAR substitution.
func LoadYAMLConfig(configDir string) (*GorkbotConfig, error) {
	configPath := filepath.Join(configDir, "gorkbot.yaml")

	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// File doesn't exist — return empty config (no error)
		return &GorkbotConfig{}, nil
	}

	// Read file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	var cfg GorkbotConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}
	mergeLegacyCustomFields(data, &cfg)

	// Expand environment variables in all string fields
	expandEnvInConfig(&cfg)

	return &cfg, nil
}

// mergeLegacyCustomFields preserves compatibility with configs that use:
// model:
//
//	custom_fields:
//	  ...
//
// even though ModelConfig stores custom fields inline.
func mergeLegacyCustomFields(data []byte, cfg *GorkbotConfig) {
	if cfg == nil {
		return
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return
	}

	model, ok := raw["model"].(map[string]interface{})
	if !ok {
		return
	}
	custom, ok := model["custom_fields"].(map[string]interface{})
	if !ok || len(custom) == 0 {
		return
	}

	if cfg.Model.CustomFields == nil {
		cfg.Model.CustomFields = make(map[string]interface{}, len(custom))
	}
	for k, v := range custom {
		cfg.Model.CustomFields[k] = v
	}
}

// SaveYAMLConfig writes cfg to <configDir>/gorkbot.yaml with atomic write.
// Uses a temp file and os.Rename to ensure atomicity.
func SaveYAMLConfig(configDir string, cfg *GorkbotConfig) error {
	configPath := filepath.Join(configDir, "gorkbot.yaml")

	// Ensure directory exists
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}

	// Write to temp file
	tmpPath := configPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, configPath); err != nil {
		os.Remove(tmpPath) // cleanup on error
		return fmt.Errorf("failed to atomically rename temp file: %w", err)
	}

	return nil
}

// expandEnvInConfig recursively expands environment variables in all string fields.
func expandEnvInConfig(cfg *GorkbotConfig) {
	if cfg == nil {
		return
	}

	cfg.Model.Use = os.ExpandEnv(cfg.Model.Use)
	cfg.Model.APIKey = os.ExpandEnv(cfg.Model.APIKey)
	cfg.Model.Model = os.ExpandEnv(cfg.Model.Model)
	cfg.Model.BaseURL = os.ExpandEnv(cfg.Model.BaseURL)

	expandEnvInMap(cfg.Model.CustomFields)

	cfg.Sandbox.Use = os.ExpandEnv(cfg.Sandbox.Use)
	expandEnvInMap(cfg.Sandbox.CustomFields)

	cfg.Guardrails.Use = os.ExpandEnv(cfg.Guardrails.Use)
	expandEnvInMap(cfg.Guardrails.CustomFields)
}

// expandEnvInMap recursively expands string values in a map.
func expandEnvInMap(m map[string]interface{}) {
	if m == nil {
		return
	}
	for k, v := range m {
		switch val := v.(type) {
		case string:
			m[k] = os.ExpandEnv(val)
		case map[string]interface{}:
			expandEnvInMap(val)
		}
	}
}
