package providers

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/velariumai/gorkbot/internal/config"
)

func TestProvidersFailClosedForExecuteAndStream(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	makeCfg := func() *config.ProviderConfig {
		return &config.ProviderConfig{
			APIKey:  "test-key",
			Model:   "test-model",
			BaseURL: "http://localhost:11434",
			Timeout: 30,
		}
	}

	req := &ExecuteRequest{
		Messages: []*Message{{Role: "user", Content: "hello"}},
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
		{name: "ollama", build: NewOllamaProvider},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prov, err := tc.build(makeCfg(), logger)
			if err != nil {
				t.Fatalf("build provider: %v", err)
			}

			_, err = prov.Execute(ctx, req)
			if err == nil {
				t.Fatalf("expected execute to fail closed")
			}
			if !strings.Contains(err.Error(), "execution unavailable in internal/providers runtime") {
				t.Fatalf("unexpected execute error: %v", err)
			}

			stream, err := prov.Stream(ctx, req)
			if err == nil {
				t.Fatalf("expected stream to fail closed")
			}
			if stream != nil {
				t.Fatalf("expected nil stream on fail-closed path")
			}
			if !strings.Contains(err.Error(), "execution unavailable in internal/providers runtime") {
				t.Fatalf("unexpected stream error: %v", err)
			}
		})
	}
}
