package provider

import (
	"fmt"

	"github.com/velariumai/gorkbot/pkg/ai"
)

// RegisterAIProviders registers all 7 built-in AI provider factories with the registry.
func RegisterAIProviders(r *FactoryRegistry) {
	r.RegisterAI("pkg.ai:XAIChatModel", newXAIFactory)
	r.RegisterAI("pkg.ai:AnthropicChatModel", newAnthropicFactory)
	r.RegisterAI("pkg.ai:GoogleChatModel", newGoogleFactory)
	r.RegisterAI("pkg.ai:OpenAIChatModel", newOpenAIFactory)
	r.RegisterAI("pkg.ai:MiniMaxChatModel", newMiniMaxFactory)
	r.RegisterAI("pkg.ai:OpenRouterChatModel", newOpenRouterFactory)
	r.RegisterAI("pkg.ai:MoonshotChatModel", newMoonshotFactory)
}

// newXAIFactory creates a Grok provider for xAI.
func newXAIFactory(params AIFactoryParams) (ai.AIProvider, error) {
	if params.APIKey == "" {
		return nil, fmt.Errorf("XAI provider requires APIKey")
	}
	return ai.NewGrokProvider(params.APIKey, params.Model), nil
}

// newAnthropicFactory creates an Anthropic provider.
func newAnthropicFactory(params AIFactoryParams) (ai.AIProvider, error) {
	if params.APIKey == "" {
		return nil, fmt.Errorf("Anthropic provider requires APIKey")
	}
	return ai.NewAnthropicProvider(params.APIKey, params.Model), nil
}

// newGoogleFactory creates a Google Gemini provider.
func newGoogleFactory(params AIFactoryParams) (ai.AIProvider, error) {
	if params.APIKey == "" {
		return nil, fmt.Errorf("Google provider requires APIKey")
	}
	return ai.NewGeminiProvider(params.APIKey, params.Model, params.VerboseThoughts), nil
}

// newOpenAIFactory creates an OpenAI provider.
func newOpenAIFactory(params AIFactoryParams) (ai.AIProvider, error) {
	if params.APIKey == "" {
		return nil, fmt.Errorf("OpenAI provider requires APIKey")
	}
	return ai.NewOpenAIProvider(params.APIKey, params.Model), nil
}

// newMiniMaxFactory creates a MiniMax provider.
func newMiniMaxFactory(params AIFactoryParams) (ai.AIProvider, error) {
	if params.APIKey == "" {
		return nil, fmt.Errorf("MiniMax provider requires APIKey")
	}
	return ai.NewMiniMaxProvider(params.APIKey, params.Model), nil
}

// newOpenRouterFactory creates an OpenRouter provider.
func newOpenRouterFactory(params AIFactoryParams) (ai.AIProvider, error) {
	if params.APIKey == "" {
		return nil, fmt.Errorf("OpenRouter provider requires APIKey")
	}
	return ai.NewOpenRouterProvider(params.APIKey, params.Model), nil
}

// newMoonshotFactory creates a Moonshot provider.
func newMoonshotFactory(params AIFactoryParams) (ai.AIProvider, error) {
	if params.APIKey == "" {
		return nil, fmt.Errorf("Moonshot provider requires APIKey")
	}
	return ai.NewMoonshotProvider(params.APIKey, params.Model), nil
}
