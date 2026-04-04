package providers

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/velariumai/gorkbot/internal/config"
)

func TestHasConfiguredAPIKey(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		valid bool
	}{
		{name: "empty", key: "", valid: false},
		{name: "spaces", key: "   ", valid: false},
		{name: "placeholder", key: "placeholder", valid: false},
		{name: "replace_me", key: "replace-me", valid: false},
		{name: "api_key_here", key: "api_key_here", valid: false},
		{name: "valid", key: "sk-real-key", valid: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasConfiguredAPIKey(tc.key); got != tc.valid {
				t.Fatalf("hasConfiguredAPIKey(%q)=%v want %v", tc.key, got, tc.valid)
			}
		})
	}
}

func TestRemoteProviderHealthCheckRejectsPlaceholderKeys(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	makeCfg := func() *config.ProviderConfig {
		return &config.ProviderConfig{
			APIKey:  "placeholder",
			Model:   "test-model",
			Timeout: 30,
		}
	}

	cases := []struct {
		name  string
		build func(*config.ProviderConfig, *slog.Logger) (AIProvider, error)
	}{
		{name: "anthropic", build: NewAnthropicProvider},
		{name: "openai", build: NewOpenAIProvider},
		{name: "google", build: NewGoogleProvider},
		{name: "azure", build: NewAzureProvider},
		{name: "bedrock", build: NewBedrockProvider},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prov, err := tc.build(makeCfg(), logger)
			if err != nil {
				t.Fatalf("build provider: %v", err)
			}
			if err := prov.HealthCheck(ctx); err == nil {
				t.Fatalf("expected health check error for placeholder key")
			}
		})
	}
}

func TestOllamaHealthCheckAllowsBaseURLWithoutAPIKey(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	cfg := &config.ProviderConfig{
		Model:   "llama3",
		BaseURL: "http://localhost:11434",
		Timeout: 30,
	}
	prov, err := NewOllamaProvider(cfg, logger)
	if err != nil {
		t.Fatalf("build provider: %v", err)
	}
	if err := prov.HealthCheck(ctx); err != nil {
		t.Fatalf("expected ollama health check success, got: %v", err)
	}
}
