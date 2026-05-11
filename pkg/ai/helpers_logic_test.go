package ai

import (
	"errors"
	"testing"
	"time"
)

func TestOpenAIModelHelpers(t *testing.T) {
	if !openAIIsOSeries("o3-mini") {
		t.Fatalf("expected o-series detection for o3-mini")
	}
	if openAIIsOSeries("gpt-4o") {
		t.Fatalf("did not expect o-series detection for gpt-4o")
	}

	if !isOpenAIChatModel("gpt-4o") {
		t.Fatalf("expected chat model for gpt-4o")
	}
	if isOpenAIChatModel("text-embedding-3-large") {
		t.Fatalf("did not expect chat model for embedding model")
	}
}

func TestAnthropicModelHelpers(t *testing.T) {
	if floor := anthropicCacheFloor("claude-opus-4-1"); floor != 4096 {
		t.Fatalf("expected opus floor 4096, got %d", floor)
	}
	if floor := anthropicCacheFloor("claude-sonnet-4-6"); floor != 2048 {
		t.Fatalf("expected sonnet 4.6 floor 2048, got %d", floor)
	}
	if !anthropicModelSupportsThinking("claude-sonnet-4") {
		t.Fatalf("expected thinking support for claude-sonnet-4")
	}
	if anthropicModelSupportsThinking("claude-2.1") {
		t.Fatalf("did not expect thinking support for claude-2.1")
	}
}

func TestOpenRouterSupportsThinking(t *testing.T) {
	if !openRouterSupportsThinking("anthropic/claude-3-opus") {
		t.Fatalf("expected thinking support for claude model")
	}
	if openRouterSupportsThinking("meta/llama-3.1-8b-instruct") {
		t.Fatalf("did not expect thinking support for generic llama model")
	}
}

func TestMoonshotK25RequestAndMetadata(t *testing.T) {
	p := NewMoonshotProvider("k", "kimi-k2.5")
	if !p.isK25Series() {
		t.Fatalf("expected k2.5 series detection")
	}
	body := p.getReqBody([]GrokMessage{{Role: "user", Content: "hi"}}, false)
	if body.Temperature == nil || body.TopP == nil {
		t.Fatalf("expected temperature/top_p set for k2.5")
	}
	if meta := p.GetMetadata(); meta.ContextSize != 131072 {
		t.Fatalf("expected 131072 context for k2.5, got %d", meta.ContextSize)
	}
}

func TestParseRetryAfter(t *testing.T) {
	if d := parseRetryAfter("12"); d != 12*time.Second {
		t.Fatalf("expected 12s, got %v", d)
	}
	if d := parseRetryAfter("not-a-date"); d != 0 {
		t.Fatalf("expected zero for invalid header, got %v", d)
	}
}

func TestTranslateStatusAndMapStatusError(t *testing.T) {
	rt := &RetryTransport{}
	if !errors.Is(rt.translateStatusCode(429), ErrRateLimit) {
		t.Fatalf("expected ErrRateLimit for 429")
	}
	if !errors.Is(rt.translateStatusCode(500), ErrProviderDown) {
		t.Fatalf("expected ErrProviderDown for 500")
	}
	if rt.translateStatusCode(418) != nil {
		t.Fatalf("expected nil for unknown status")
	}

	if !errors.Is(MapStatusError(401, []byte("nope")), ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized mapping")
	}
	if !errors.Is(MapStatusError(402, []byte("pay")), ErrNoCredits) {
		t.Fatalf("expected ErrNoCredits mapping")
	}
	if !errors.Is(MapStatusError(504, []byte("gw")), ErrBadGateway) {
		t.Fatalf("expected ErrBadGateway mapping")
	}
}
