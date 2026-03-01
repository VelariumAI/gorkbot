package ai

import (
	"time"

	"github.com/velariumai/gorkbot/pkg/registry"
)

// SafeModelDefs returns a hardcoded list of well-known models for a provider.
// These are used as a fallback when the provider's /models endpoint is
// unreachable, rate-limited, or returns malformed data.
func SafeModelDefs(provider string) []registry.ModelDefinition {
	now := time.Now()
	switch provider {
	case "xai":
		return []registry.ModelDefinition{
			safeModel("xai", "grok-3", "Grok 3", false, 131072, now),
			safeModel("xai", "grok-3-mini", "Grok 3 Mini", true, 131072, now),
			safeModel("xai", "grok-2-1212", "Grok 2", false, 131072, now),
			safeModel("xai", "grok-2-vision-1212", "Grok 2 Vision", false, 8192, now),
		}
	case "google":
		return []registry.ModelDefinition{
			safeModel("google", "gemini-2.5-pro-preview-05-06", "Gemini 2.5 Pro", true, 1048576, now),
			safeModel("google", "gemini-2.0-flash", "Gemini 2.0 Flash", false, 1048576, now),
			safeModel("google", "gemini-1.5-pro", "Gemini 1.5 Pro", false, 2097152, now),
			safeModel("google", "gemini-1.5-flash", "Gemini 1.5 Flash", false, 1048576, now),
		}
	case "anthropic":
		return []registry.ModelDefinition{
			safeModel("anthropic", "claude-opus-4-5", "Claude Opus 4.5", true, 200000, now),
			safeModel("anthropic", "claude-sonnet-4-5", "Claude Sonnet 4.5", false, 200000, now),
			safeModel("anthropic", "claude-haiku-4-5-20251001", "Claude Haiku 4.5", false, 200000, now),
			safeModel("anthropic", "claude-3-5-sonnet-20241022", "Claude 3.5 Sonnet", false, 200000, now),
		}
	case "openai":
		return []registry.ModelDefinition{
			safeModel("openai", "gpt-4o", "GPT-4o", false, 128000, now),
			safeModel("openai", "gpt-4o-mini", "GPT-4o Mini", false, 128000, now),
			safeModel("openai", "o3", "o3", true, 200000, now),
			safeModel("openai", "o4-mini", "o4-mini", true, 200000, now),
		}
	case "minimax":
		return []registry.ModelDefinition{
			safeModel("minimax", "MiniMax-M1", "MiniMax M1", false, 1000000, now),
			safeModel("minimax", "MiniMax-M2", "MiniMax M2", true, 1000000, now),
			safeModel("minimax", "MiniMax-M2.1", "MiniMax M2.1", true, 1000000, now),
			safeModel("minimax", "MiniMax-M2.5", "MiniMax M2.5", true, 1000000, now),
		}
	case "openrouter":
		return []registry.ModelDefinition{
			safeModel("openrouter", "anthropic/claude-opus-4-6", "Claude Opus 4.6", true, 200000, now),
			safeModel("openrouter", "anthropic/claude-sonnet-4-5", "Claude Sonnet 4.5", true, 200000, now),
			safeModel("openrouter", "openai/gpt-4o", "GPT-4o", false, 128000, now),
			safeModel("openrouter", "meta-llama/llama-3.1-70b-instruct", "Llama 3.1 70B", false, 131072, now),
			safeModel("openrouter", "google/gemini-2.0-flash-exp", "Gemini 2.0 Flash", true, 1048576, now),
			safeModel("openrouter", "deepseek/deepseek-chat", "DeepSeek Chat", false, 64000, now),
		}
	}
	return nil
}

func safeModel(provider, id, name string, thinking bool, ctx int, ts time.Time) registry.ModelDefinition {
	return registry.ModelDefinition{
		ID:       registry.ModelID(id),
		Provider: registry.ProviderID(provider),
		Name:     name,
		Capabilities: registry.CapabilitySet{
			MaxContextTokens:  ctx,
			SupportsStreaming: true,
			SupportsTools:     true,
			SupportsThinking:  thinking,
		},
		Status:      registry.StatusActive,
		LastUpdated: ts,
	}
}
