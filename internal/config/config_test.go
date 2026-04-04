package config

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("DefaultConfig returned nil")
	}

	if cfg.Capabilities.DefaultProvider == "" {
		t.Error("DefaultProvider not set")
	}

	if len(cfg.Providers) == 0 {
		t.Error("No providers in default config")
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *GorkbotConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name:    "valid default",
			config:  DefaultConfig(),
			wantErr: false,
		},
		{
			name: "missing providers",
			config: &GorkbotConfig{
				Providers:    make(map[string]ProviderConfig),
				Capabilities: CapabilitiesConfig{DefaultProvider: "missing"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestApplyDefaults(t *testing.T) {
	cfg := &GorkbotConfig{
		Providers: map[string]ProviderConfig{
			"test": {
				Model: "test-model",
			},
		},
		Capabilities: CapabilitiesConfig{
			DefaultProvider: "test",
		},
	}

	if err := ApplyDefaults(cfg); err != nil {
		t.Fatalf("ApplyDefaults failed: %v", err)
	}

	provider := cfg.Providers["test"]
	if provider.Timeout == 0 {
		t.Error("Timeout default not applied")
	}

	if provider.MaxRetries == 0 {
		t.Error("MaxRetries default not applied")
	}

	if cfg.Specialist.ThinkingBudget == 0 {
		t.Error("Specialist thinking budget default not applied")
	}
}

func TestValidateProviderConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *ProviderConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			cfg:     nil,
			wantErr: true,
		},
		{
			name: "missing api key and url",
			cfg: &ProviderConfig{
				Model: "test",
			},
			wantErr: true,
		},
		{
			name: "missing model",
			cfg: &ProviderConfig{
				APIKey: "key",
			},
			wantErr: true,
		},
		{
			name: "invalid timeout",
			cfg: &ProviderConfig{
				APIKey:  "key",
				Model:   "model",
				Timeout: -1,
			},
			wantErr: true,
		},
		{
			name: "valid config",
			cfg: &ProviderConfig{
				APIKey:  "key",
				Model:   "model",
				Timeout: 60,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProviderConfig("test", tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateProviderConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfigurationHierarchy(t *testing.T) {
	cfg := DefaultConfig()

	// Verify capability -> provider mapping
	capProvider, err := cfg.Capabilities.GetCapabilityProvider("thinking")
	if err != nil {
		t.Errorf("GetCapabilityProvider failed: %v", err)
	}

	if capProvider == "" {
		t.Error("Capability provider is empty")
	}

	// Verify provider exists
	if _, ok := cfg.Providers[capProvider]; !ok {
		t.Errorf("Provider %s not configured", capProvider)
	}
}

func TestValidationErrors(t *testing.T) {
	cfg := &GorkbotConfig{
		Providers: make(map[string]ProviderConfig),
	}

	errors := ValidateMulti(cfg)
	if len(errors) == 0 {
		t.Error("Expected validation errors, got none")
	}
}

func TestPromptsDefaults(t *testing.T) {
	cfg := &GorkbotConfig{
		Prompts: PromptsConfig{},
	}

	ApplyDefaults(cfg)

	if cfg.Prompts.Default == "" {
		t.Error("Prompts.Default not set")
	}

	if cfg.Prompts.PerProvider == nil {
		t.Error("Prompts.PerProvider is nil")
	}
}

func TestBrowserDefaults(t *testing.T) {
	cfg := &GorkbotConfig{
		Browser: BrowserConfig{},
	}

	ApplyDefaults(cfg)

	if cfg.Browser.Timeout == 0 {
		t.Error("Browser timeout not set")
	}

	if cfg.Browser.Screenshots.MaxWidth == 0 {
		t.Error("Browser screenshots max width not set")
	}
}

func TestSpecialistDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Specialist.ComplexityThreshold == 0 {
		t.Error("Specialist complexity threshold not set")
	}

	if cfg.Specialist.FilesThreshold == 0 {
		t.Error("Specialist files threshold not set")
	}

	if cfg.Specialist.AutonomyLevel == "" {
		t.Error("Specialist autonomy level not set")
	}
}

func TestExportDefaults(t *testing.T) {
	// Create temp file
	tmpfile, err := os.CreateTemp("", "gorkbot_*.toml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	if err := ExportDefaults(tmpfile.Name()); err != nil {
		t.Fatalf("ExportDefaults failed: %v", err)
	}

	// Verify file exists and has content
	content, err := ioutil.ReadFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to read exported file: %v", err)
	}

	if len(content) == 0 {
		t.Error("Exported file is empty")
	}

	if !strings.Contains(string(content), "[providers]") {
		t.Error("Exported file missing [providers] section")
	}
}
