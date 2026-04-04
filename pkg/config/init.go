package config

import (
	"log/slog"
	"os"
)

// Initialize loads the global config and sets up watchers
// This should be called early in application startup
func Initialize(configPath string) error {
	// Load config
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return err
	}

	// Validate config
	if err := ValidateConfig(cfg); err != nil {
		return err
	}

	// Log loaded configuration
	slog.Info("Configuration loaded",
		"config_version", cfg.ConfigVersion,
		"models", len(cfg.Models),
		"tools", len(cfg.Tools),
		"path", configPath,
	)

	// Start watching for config changes
	WatchConfigFile(func(newCfg *Config) {
		slog.Info("Configuration reloaded",
			"models", len(newCfg.Models),
			"tools", len(newCfg.Tools),
		)
	})

	return nil
}

// CreateExampleConfig creates config.example.yaml if it doesn't exist
func CreateExampleConfig(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil // File already exists
	}

	exampleContent := `# Gorkbot Configuration
# This is an example configuration file. Copy to config.yaml and customize.

config_version: 2

models:
  - name: grok
    display_name: "xAI Grok"
    use: "pkg.ai:XAIChatModel"
    model: "grok-3-mini"
    api_key: $XAI_API_KEY
    max_tokens: 8192
    supports_thinking: true
    supports_vision: false

  - name: claude-4
    display_name: "Claude 4"
    use: "pkg.ai:AnthropicChatModel"
    model: "claude-opus-4-6"
    api_key: $ANTHROPIC_API_KEY
    max_tokens: 4096
    supports_thinking: true
    supports_vision: true

sandbox:
  use: "pkg.sandbox.local:LocalSandboxProvider"

skills:
  path: "./skills"
  container_path: "/mnt/skills"

memory:
  enabled: true
  injection_enabled: true
  storage_path: "~/.gorkbot/memory.db"
  debounce_seconds: 30
  max_facts: 100
  fact_confidence_threshold: 0.7
  max_injection_tokens: 2000

subagents:
  enabled: true
  max_concurrent: 3
  timeout_seconds: 900

channels:
  langgraph_url: "http://localhost:2024"
  gateway_url: "http://localhost:8001"

guardrails:
  enabled: false
  use: "pkg.guardrails:AllowlistProvider"
`

	return os.WriteFile(path, []byte(exampleContent), 0644)
}
