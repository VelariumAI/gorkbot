package ai

import "testing"

func TestGrokThinkingSupportDetection(t *testing.T) {
	if !grokModelSupportsThinking("grok-3-mini") {
		t.Fatalf("expected grok-3-mini to support configurable thinking")
	}
	if grokModelSupportsThinking("grok-4-fast-reasoning") {
		t.Fatalf("did not expect grok-4-fast-reasoning to support reasoning_effort")
	}
}

func TestGeminiThinkingSupportDetection(t *testing.T) {
	if !geminiModelSupportsThinking("gemini-2.5-pro") {
		t.Fatalf("expected gemini-2.5-pro thinking support")
	}
	if !geminiModelSupportsThinking("gemini-thinking-exp") {
		t.Fatalf("expected explicit thinking variant support")
	}
	if geminiModelSupportsThinking("gemini-2.0-flash") {
		t.Fatalf("did not expect gemini-2.0-flash thinking support")
	}
}

func TestInferOpenAICapabilities(t *testing.T) {
	caps := InferOpenAICapabilities("grok-3-mini")
	if !caps.SupportsThinking || !caps.SupportsTools || !caps.SupportsVision {
		t.Fatalf("unexpected grok capability inference: %+v", caps)
	}

	caps = InferOpenAICapabilities("gpt-4o")
	if caps.MaxContextTokens != 128000 || !caps.SupportsVision || !caps.SupportsTools || !caps.SupportsJSONMode {
		t.Fatalf("unexpected gpt-4o capability inference: %+v", caps)
	}

	caps = InferOpenAICapabilities("gpt-3.5-turbo")
	if caps.MaxContextTokens != 16385 {
		t.Fatalf("unexpected gpt-3.5 context: %+v", caps)
	}
}

func TestSafeModelDefs(t *testing.T) {
	for _, provider := range []string{"xai", "google", "anthropic", "openai", "minimax", "openrouter"} {
		models := SafeModelDefs(provider)
		if len(models) == 0 {
			t.Fatalf("expected fallback models for provider %s", provider)
		}
	}
	if got := SafeModelDefs("unknown-provider"); got != nil {
		t.Fatalf("expected nil for unknown provider")
	}
}
