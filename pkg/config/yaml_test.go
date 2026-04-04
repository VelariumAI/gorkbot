package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/velariumai/gorkbot/pkg/config"
)

func TestLoadYAMLConfigMissingFile(t *testing.T) {
	// Missing file should return empty config, not error
	tmpDir := t.TempDir()
	cfg, err := config.LoadYAMLConfig(tmpDir)
	if err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config for missing file")
	}
	// Check zero values
	if cfg.Model.Use != "" {
		t.Errorf("expected empty Model.Use, got %q", cfg.Model.Use)
	}
}

func TestLoadYAMLConfigBasic(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "gorkbot.yaml")

	// Write a simple YAML file
	yaml := `model:
  use: "pkg.ai:AnthropicProvider"
  api_key: "test-key"
  max_tokens: 4096
sandbox:
  enabled: true
  use: "pkg.sandbox:LocalSandbox"
guardrails:
  enabled: false
`
	if err := os.WriteFile(configPath, []byte(yaml), 0o644); err != nil {
		t.Fatalf("failed to write test YAML: %v", err)
	}

	cfg, err := config.LoadYAMLConfig(tmpDir)
	if err != nil {
		t.Fatalf("failed to load YAML: %v", err)
	}

	if cfg.Model.Use != "pkg.ai:AnthropicProvider" {
		t.Errorf("expected Model.Use=pkg.ai:AnthropicProvider, got %q", cfg.Model.Use)
	}
	if cfg.Model.APIKey != "test-key" {
		t.Errorf("expected Model.APIKey=test-key, got %q", cfg.Model.APIKey)
	}
	if cfg.Model.MaxTokens != 4096 {
		t.Errorf("expected Model.MaxTokens=4096, got %d", cfg.Model.MaxTokens)
	}
	if !cfg.Sandbox.Enabled {
		t.Errorf("expected Sandbox.Enabled=true, got false")
	}
	if cfg.Sandbox.Use != "pkg.sandbox:LocalSandbox" {
		t.Errorf("expected Sandbox.Use=pkg.sandbox:LocalSandbox, got %q", cfg.Sandbox.Use)
	}
}

func TestLoadYAMLConfigEnvExpansion(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "gorkbot.yaml")

	// Set env vars
	os.Setenv("TEST_MODEL", "TestModel123")
	os.Setenv("TEST_KEY", "SecretKey456")
	defer os.Unsetenv("TEST_MODEL")
	defer os.Unsetenv("TEST_KEY")

	// Write YAML with env var references
	yaml := `model:
  use: ${TEST_MODEL}
  api_key: $TEST_KEY
sandbox:
  use: "static-value"
`
	if err := os.WriteFile(configPath, []byte(yaml), 0o644); err != nil {
		t.Fatalf("failed to write test YAML: %v", err)
	}

	cfg, err := config.LoadYAMLConfig(tmpDir)
	if err != nil {
		t.Fatalf("failed to load YAML: %v", err)
	}

	if cfg.Model.Use != "TestModel123" {
		t.Errorf("expected expanded Model.Use=TestModel123, got %q", cfg.Model.Use)
	}
	if cfg.Model.APIKey != "SecretKey456" {
		t.Errorf("expected expanded Model.APIKey=SecretKey456, got %q", cfg.Model.APIKey)
	}
	if cfg.Sandbox.Use != "static-value" {
		t.Errorf("expected static Sandbox.Use=static-value, got %q", cfg.Sandbox.Use)
	}
}

func TestSaveYAMLConfigRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a config
	cfg1 := &config.GorkbotConfig{
		Model: config.ModelConfig{
			Use:       "pkg.ai:TestProvider",
			APIKey:    "test-api-key",
			Model:     "test-model",
			MaxTokens: 2048,
		},
		Sandbox: config.SandboxConfig{
			Enabled: true,
			Use:     "pkg.sandbox:LocalSandbox",
		},
	}

	// Save it
	if err := config.SaveYAMLConfig(tmpDir, cfg1); err != nil {
		t.Fatalf("failed to save YAML: %v", err)
	}

	// Load it back
	cfg2, err := config.LoadYAMLConfig(tmpDir)
	if err != nil {
		t.Fatalf("failed to load YAML back: %v", err)
	}

	// Compare
	if cfg2.Model.Use != cfg1.Model.Use {
		t.Errorf("Model.Use mismatch: %q vs %q", cfg1.Model.Use, cfg2.Model.Use)
	}
	if cfg2.Model.APIKey != cfg1.Model.APIKey {
		t.Errorf("Model.APIKey mismatch: %q vs %q", cfg1.Model.APIKey, cfg2.Model.APIKey)
	}
	if cfg2.Model.MaxTokens != cfg1.Model.MaxTokens {
		t.Errorf("Model.MaxTokens mismatch: %d vs %d", cfg1.Model.MaxTokens, cfg2.Model.MaxTokens)
	}
	if cfg2.Sandbox.Enabled != cfg1.Sandbox.Enabled {
		t.Errorf("Sandbox.Enabled mismatch: %v vs %v", cfg1.Sandbox.Enabled, cfg2.Sandbox.Enabled)
	}
}

func TestLoadYAMLConfigCustomFields(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "gorkbot.yaml")

	// Write YAML with custom fields
	yaml := `model:
  use: "pkg.ai:AnthropicProvider"
  custom_fields:
    thinking_budget: 8000
    stream_enabled: true
    api_version: "2024-06"
`
	if err := os.WriteFile(configPath, []byte(yaml), 0o644); err != nil {
		t.Fatalf("failed to write test YAML: %v", err)
	}

	cfg, err := config.LoadYAMLConfig(tmpDir)
	if err != nil {
		t.Fatalf("failed to load YAML: %v", err)
	}

	if cfg.Model.CustomFields == nil {
		t.Fatal("expected non-nil CustomFields")
	}
	if tb, ok := cfg.Model.CustomFields["thinking_budget"].(int); !ok || tb != 8000 {
		t.Errorf("expected thinking_budget=8000, got %v", cfg.Model.CustomFields["thinking_budget"])
	}
	if stream, ok := cfg.Model.CustomFields["stream_enabled"].(bool); !ok || !stream {
		t.Errorf("expected stream_enabled=true, got %v", cfg.Model.CustomFields["stream_enabled"])
	}
}
