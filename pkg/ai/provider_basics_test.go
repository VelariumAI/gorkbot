package ai

import (
	"net/http"
	"testing"
)

func TestGrokBasics(t *testing.T) {
	g := NewGrokProvider("k", "")
	if g.Name() != "Grok" || g.ID() != "xai" {
		t.Fatalf("unexpected grok identity")
	}
	if g.Model != "grok-3" {
		t.Fatalf("expected default grok model")
	}
	g.SetConvID("conv-1")
	req, _ := http.NewRequest("GET", "https://example.com", nil)
	g.injectGrokCacheHeader(req)
	if req.Header.Get("x-grok-conv-id") != "conv-1" {
		t.Fatalf("expected conv header")
	}
	_ = g.LastUsage()
}

func TestGeminiBasics(t *testing.T) {
	g := NewGeminiProvider("k", "", true)
	if g.Name() != "Gemini" || g.ID() != "google" {
		t.Fatalf("unexpected gemini identity")
	}
	if g.GetMetadata().ContextSize == 0 {
		t.Fatalf("expected metadata context size")
	}
	g.SetCachedContent("cached/1")
	if g.CachedContentName != "cached/1" {
		t.Fatalf("expected cached content to be set")
	}
	if _, ok := g.WithModel("gemini-2.5-pro").(*GeminiProvider); !ok {
		t.Fatalf("expected WithModel to return GeminiProvider")
	}
}

func TestAnthropicBasics(t *testing.T) {
	a := NewAnthropicProviderWithAuth("api", "oauth", "")
	if a.Name() != "Claude" || a.ID() != "anthropic" {
		t.Fatalf("unexpected anthropic identity")
	}
	if a.GetMetadata().ContextSize == 0 {
		t.Fatalf("expected metadata context size")
	}
	a.SetThinkingBudget(1234)
	if a.GetThinkingBudget() != 1234 {
		t.Fatalf("expected thinking budget set/get")
	}
	if _, ok := a.WithModel("claude-opus-4").(*AnthropicProvider); !ok {
		t.Fatalf("expected WithModel to return AnthropicProvider")
	}
}

func TestOpenAIBasics(t *testing.T) {
	o := NewOpenAIProviderWithAuth("api", "oauth", "")
	if o.Name() != "OpenAI" || o.ID() != "openai" {
		t.Fatalf("unexpected openai identity")
	}
	if o.GetMetadata().ContextSize == 0 {
		t.Fatalf("expected metadata context size")
	}
	if _, ok := o.WithModel("gpt-4o").(*OpenAIProvider); !ok {
		t.Fatalf("expected WithModel to return OpenAIProvider")
	}
}

func TestOpenRouterBasics(t *testing.T) {
	o := NewOpenRouterProvider("k", "")
	if o.Name() != "OpenRouter" || o.ID() != "openrouter" {
		t.Fatalf("unexpected openrouter identity")
	}
	req, _ := http.NewRequest("GET", "https://example.com", nil)
	o.addHeaders(req)
	if req.Header.Get("Authorization") == "" || req.Header.Get("HTTP-Referer") == "" || req.Header.Get("X-Title") == "" {
		t.Fatalf("expected required openrouter headers")
	}
	if _, ok := o.WithModel("openai/gpt-4o").(*OpenRouterProvider); !ok {
		t.Fatalf("expected WithModel to return OpenRouterProvider")
	}
}

func TestMoonshotBasics(t *testing.T) {
	m := NewMoonshotProvider("k", "")
	if m.Name() != "Moonshot" || m.ID() != "moonshot" {
		t.Fatalf("unexpected moonshot identity")
	}
	if m.GetMetadata().ContextSize == 0 {
		t.Fatalf("expected metadata context size")
	}
	if _, ok := m.WithModel("moonshot-v1-32k").(*MoonshotProvider); !ok {
		t.Fatalf("expected WithModel to return MoonshotProvider")
	}
}

func TestMiniMaxBasics(t *testing.T) {
	m := NewMiniMaxProvider("k", "")
	if m.Name() != "MiniMax" || m.ID() != "minimax" {
		t.Fatalf("unexpected minimax identity")
	}
	if m.GetMetadata().ContextSize == 0 {
		t.Fatalf("expected metadata context size")
	}
	m.SetThinkingBudget(222)
	if m.GetThinkingBudget() != 222 {
		t.Fatalf("expected minimax thinking budget set/get")
	}
	if _, ok := m.WithModel("MiniMax-M2.5").(*MiniMaxProvider); !ok {
		t.Fatalf("expected WithModel to return MiniMaxProvider")
	}
	if len(fallbackMiniMaxModels()) == 0 {
		t.Fatalf("expected fallback minimax models")
	}
	if minimaxAnthropicVersion() == "" {
		t.Fatalf("expected minimax anthropic version")
	}
}
