package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigFromYAML(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	configContent := `
config_version: 2

models:
  - name: test-model
    display_name: "Test Model"
    use: "pkg.ai:TestChatModel"
    model: "test-model"
    api_key: "test-key-123"
    max_tokens: 4096
    temperature: 0.7
    supports_thinking: true
    supports_vision: false

tools:
  - use: "pkg.tools:TestTool"
    group: "test"
    enabled: true

sandbox:
  use: "pkg.sandbox.local:LocalSandboxProvider"

skills:
  path: "./skills"
  container_path: "/mnt/skills"

memory:
  enabled: true
  debounce_seconds: 30
  max_facts: 100

subagents:
  enabled: true
  max_concurrent: 3
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	// Load the config
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Verify config was loaded correctly
	if cfg == nil {
		t.Fatal("config is nil")
	}

	if cfg.ConfigVersion != 2 {
		t.Errorf("expected config_version=2, got %d", cfg.ConfigVersion)
	}

	if len(cfg.Models) != 1 {
		t.Errorf("expected 1 model, got %d", len(cfg.Models))
	}

	if cfg.Models[0].Name != "test-model" {
		t.Errorf("expected model name 'test-model', got %q", cfg.Models[0].Name)
	}

	if cfg.Models[0].APIKey != "test-key-123" {
		t.Errorf("expected api_key 'test-key-123', got %q", cfg.Models[0].APIKey)
	}

	if !cfg.Models[0].SupportsThinking {
		t.Error("expected supports_thinking=true")
	}

	if cfg.Models[0].SupportsVision {
		t.Error("expected supports_vision=false")
	}

	if len(cfg.Tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(cfg.Tools))
	}

	if cfg.Memory.MaxFacts != 100 {
		t.Errorf("expected max_facts=100, got %d", cfg.Memory.MaxFacts)
	}

	if cfg.Subagents.MaxConcurrent != 3 {
		t.Errorf("expected max_concurrent=3, got %d", cfg.Subagents.MaxConcurrent)
	}
}

func TestEnvironmentVariableSubstitution(t *testing.T) {
	// Create a temporary config file with env var references
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	configContent := `
config_version: 2

models:
  - name: test-model
    display_name: "Test Model"
    use: "pkg.ai:TestChatModel"
    model: "test-model"
    api_key: $TEST_API_KEY
    max_tokens: 4096
    supports_thinking: false
    supports_vision: false

sandbox:
  use: "pkg.sandbox.local:LocalSandboxProvider"

skills:
  path: "./skills"
  container_path: "/mnt/skills"

memory:
  enabled: true
  debounce_seconds: 30
  max_facts: 100

subagents:
  enabled: true
  max_concurrent: 3
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	// Set environment variable
	os.Setenv("TEST_API_KEY", "env-key-from-env-var")

	// Load the config
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Verify environment variable was substituted
	if cfg.Models[0].APIKey != "env-key-from-env-var" {
		t.Errorf("expected api_key='env-key-from-env-var', got %q", cfg.Models[0].APIKey)
	}
}

func TestConfigValidation(t *testing.T) {
	// Test missing models
	cfg := &Config{
		Models: []ModelConfig{},
	}

	err := ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error for missing models")
	}

	// Test missing model name
	cfg = &Config{
		Models: []ModelConfig{
			{
				Name: "",
			},
		},
	}

	err = ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error for empty model name")
	}

	// Test missing model use path
	cfg = &Config{
		Models: []ModelConfig{
			{
				Name: "test",
				Use:  "",
			},
		},
	}

	err = ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error for missing model use path")
	}

	// Test missing API key
	cfg = &Config{
		Models: []ModelConfig{
			{
				Name:   "test",
				Use:    "pkg.ai:Test",
				APIKey: "",
			},
		},
	}

	err = ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error for missing API key")
	}

	// Test valid config
	cfg = &Config{
		Models: []ModelConfig{
			{
				Name:   "test",
				Use:    "pkg.ai:Test",
				APIKey: "test-key",
			},
		},
	}

	err = ValidateConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestDefaultValues(t *testing.T) {
	cfg := &Config{
		Models: []ModelConfig{
			{
				Name:   "test",
				Use:    "pkg.ai:Test",
				APIKey: "test-key",
			},
		},
	}

	cfg.applyDefaults()

	if cfg.Memory.DebounceSeconds != DefaultMemoryDebounceSeconds {
		t.Errorf("expected debounce_seconds=%d, got %d", DefaultMemoryDebounceSeconds, cfg.Memory.DebounceSeconds)
	}

	if cfg.Memory.MaxFacts != DefaultMemoryMaxFacts {
		t.Errorf("expected max_facts=%d, got %d", DefaultMemoryMaxFacts, cfg.Memory.MaxFacts)
	}

	if cfg.Subagents.MaxConcurrent != DefaultSubagentsMaxConcurrent {
		t.Errorf("expected max_concurrent=%d, got %d", DefaultSubagentsMaxConcurrent, cfg.Subagents.MaxConcurrent)
	}

	if cfg.Skills.ContainerPath != DefaultSkillsContainerPath {
		t.Errorf("expected container_path=%q, got %q", DefaultSkillsContainerPath, cfg.Skills.ContainerPath)
	}
}

func TestGetModelConfig(t *testing.T) {
	cfg := &Config{
		Models: []ModelConfig{
			{Name: "model1", Use: "pkg.ai:Test1"},
			{Name: "model2", Use: "pkg.ai:Test2"},
		},
	}

	// Test existing model
	modelCfg := cfg.GetModelConfig("model1")
	if modelCfg == nil {
		t.Fatal("expected to find model1")
	}
	if modelCfg.Name != "model1" {
		t.Errorf("expected name='model1', got %q", modelCfg.Name)
	}

	// Test non-existing model
	modelCfg = cfg.GetModelConfig("nonexistent")
	if modelCfg != nil {
		t.Fatal("expected nil for nonexistent model")
	}
}

func TestGetEnabledTools(t *testing.T) {
	cfg := &Config{
		Tools: []ToolConfig{
			{Use: "tool1", Group: "test", Enabled: true},
			{Use: "tool2", Group: "test", Enabled: false},
			{Use: "tool3", Group: "test", Enabled: true},
		},
	}

	enabledTools := cfg.GetEnabledTools()
	if len(enabledTools) != 2 {
		t.Errorf("expected 2 enabled tools, got %d", len(enabledTools))
	}
}
