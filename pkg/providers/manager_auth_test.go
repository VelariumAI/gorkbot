package providers

import (
	"log/slog"
	"testing"

	"github.com/velariumai/gorkbot/pkg/ai"
)

func TestBuildBaseOpenAIHybridAuth(t *testing.T) {
	m := &Manager{}
	p := m.buildBase(ProviderOpenAI, "api-key", "oauth-token")

	op, ok := p.(*ai.OpenAIProvider)
	if !ok {
		t.Fatalf("expected *ai.OpenAIProvider, got %T", p)
	}
	if op.APIKey != "api-key" {
		t.Fatalf("expected API key to be preserved")
	}
	if op.OAuthAccessToken != "oauth-token" {
		t.Fatalf("expected oauth token to be preserved")
	}
}

func TestBuildBaseAnthropicHybridAuth(t *testing.T) {
	m := &Manager{}
	p := m.buildBase(ProviderAnthropic, "api-key", "oauth-token")

	ap, ok := p.(*ai.AnthropicProvider)
	if !ok {
		t.Fatalf("expected *ai.AnthropicProvider, got %T", p)
	}
	if ap.APIKey != "api-key" {
		t.Fatalf("expected API key to be preserved")
	}
	if ap.OAuthAccessToken != "oauth-token" {
		t.Fatalf("expected oauth token to be preserved")
	}
}

func TestBuildBaseUnknownProviderReturnsNil(t *testing.T) {
	m := &Manager{logger: slog.Default()}
	if p := m.buildBase("unknown-provider", "k", ""); p != nil {
		t.Fatalf("expected nil provider for unknown id, got %T", p)
	}
}
