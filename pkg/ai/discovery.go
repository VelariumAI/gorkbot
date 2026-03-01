package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/registry"
)

// OpenAIModel represents a single model from an OpenAI-compatible /v1/models endpoint
type OpenAIModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// OpenAIModelListResponse represents the response from /v1/models
type OpenAIModelListResponse struct {
	Object string        `json:"object"`
	Data   []OpenAIModel `json:"data"`
}

// GeminiModel represents a single model from Google's API
type GeminiModel struct {
	Name             string   `json:"name"` // format: "models/model-id"
	Version          string   `json:"version"`
	DisplayName      string   `json:"displayName"`
	Description      string   `json:"description"`
	InputTokenLimit  int      `json:"inputTokenLimit"`
	OutputTokenLimit int      `json:"outputTokenLimit"`
	SupportedMethods []string `json:"supportedGenerationMethods"`
	Temperature      float64  `json:"temperature"`
	TopP             float64  `json:"topP"`
	TopK             int      `json:"topK"`
}

// GeminiModelListResponse represents the response from Google's models.list
type GeminiModelListResponse struct {
	Models []GeminiModel `json:"models"`
}

// FetchOpenAIModels performs model discovery against an OpenAI-compatible endpoint
func FetchOpenAIModels(ctx context.Context, baseURL string, apiKey string) ([]registry.ModelDefinition, error) {
	url := fmt.Sprintf("%s/v1/models", strings.TrimRight(baseURL, "/"))
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := NewRetryClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discovery request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var listResp OpenAIModelListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("failed to decode model list: %w", err)
	}

	var models []registry.ModelDefinition
	for _, m := range listResp.Data {
		// Heuristic mapping for capabilities
		// OpenAI response doesn't give context window or modality, so we must infer or lookup
		capabilities := InferOpenAICapabilities(m.ID)
		
		def := registry.ModelDefinition{
			ID:           registry.ModelID(m.ID),
			Provider:     "xai", // Defaulting to xAI if used by Grok provider, but logic should be generic
			Name:         m.ID,
			Description:  fmt.Sprintf("Discovered model %s owned by %s", m.ID, m.OwnedBy),
			Capabilities: capabilities,
			Status:       registry.StatusActive,
			LastUpdated:  time.Now(),
		}
		models = append(models, def)
	}

	return models, nil
}

// FetchGeminiModels performs model discovery against Google's API
func FetchGeminiModels(ctx context.Context, apiKey string) ([]registry.ModelDefinition, error) {
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models?key=%s", apiKey)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery request: %w", err)
	}

	client := NewRetryClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discovery request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var listResp GeminiModelListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("failed to decode model list: %w", err)
	}

	var models []registry.ModelDefinition
	for _, m := range listResp.Models {
		// Filter for generateContent models
		canGenerate := false
		for _, method := range m.SupportedMethods {
			if method == "generateContent" {
				canGenerate = true
				break
			}
		}
		if !canGenerate {
			continue
		}

		// Google IDs come as "models/gemini-pro". We treat that as the ID.
		// Or strip "models/"? Usually libraries prefer the full string or just the name.
		// Let's keep "models/" prefix as it's the official resource name, 
		// OR strip it if the Generate function expects pure names.
		// Standard Google AI client usually takes "gemini-pro".
		cleanID := strings.TrimPrefix(m.Name, "models/")

		capabilities := registry.CapabilitySet{
			MaxContextTokens:  m.InputTokenLimit,
			SupportsStreaming: true, // Most Gemini models support streaming
			SupportsVision:    strings.Contains(cleanID, "vision") || strings.Contains(cleanID, "1.5") || strings.Contains(cleanID, "2.0") || strings.Contains(cleanID, "2.5"), // 1.5+ are multimodal
			SupportsTools:     strings.Contains(cleanID, "gemini"), // Most modern Gemini models support tools
			SupportsJSONMode:  strings.Contains(cleanID, "1.5") || strings.Contains(cleanID, "2.0") || strings.Contains(cleanID, "2.5"),
			SupportsThinking:  geminiModelSupportsThinking(cleanID),
		}

		def := registry.ModelDefinition{
			ID:           registry.ModelID(cleanID),
			Provider:     "google",
			Name:         m.DisplayName,
			Description:  m.Description,
			Capabilities: capabilities,
			Status:       registry.StatusActive,
			LastUpdated:  time.Now(),
		}
		models = append(models, def)
	}

	return models, nil
}

// grokModelSupportsThinking returns true if the xAI model supports the reasoning_effort parameter.
//
// Per xAI's official documentation (https://docs.x.ai/docs/guides/reasoning):
//   - ONLY the grok-3-mini family supports reasoning_effort ("low" / "high").
//   - grok-3, grok-4, grok-4-fast-reasoning, grok-4-1-fast-reasoning, and all other
//     models do NOT accept reasoning_effort and return 400 Bad Request if it is sent.
//
// IMPORTANT: models with "-reasoning" in their name (e.g. grok-4-fast-reasoning) have
// reasoning baked in and cannot be configured via this parameter. Do NOT treat the word
// "reasoning" in a model ID as a signal that reasoning_effort is supported — it is not.
func grokModelSupportsThinking(modelID string) bool {
	lower := strings.ToLower(modelID)
	// Strict allowlist: only grok-3-mini and its dated/fast/beta variants.
	return strings.HasPrefix(lower, "grok-3-mini")
}

// geminiModelSupportsThinking returns true if the model ID indicates native thinking/reasoning mode support.
// Based on Google's published capability matrix: thinking mode is available in Gemini 2.5+ and
// any model variant explicitly named "thinking" or "reasoning".
func geminiModelSupportsThinking(modelID string) bool {
	lower := strings.ToLower(modelID)
	// Explicit thinking/reasoning variants
	if strings.Contains(lower, "thinking") || strings.Contains(lower, "reasoning") {
		return true
	}
	// Gemini 2.5+ family supports thinking natively
	if strings.HasPrefix(lower, "gemini-2.5") {
		return true
	}
	// Gemini 3+ (future models) — forward-compatible assumption
	if strings.HasPrefix(lower, "gemini-3") {
		return true
	}
	return false
}

// FetchAnthropicModels_Discovery fetches models from Anthropic for the discovery manager.
// This is a thin wrapper around FetchAnthropicModels defined in anthropic.go.
func FetchAnthropicModels_Discovery(ctx context.Context, apiKey string) ([]registry.ModelDefinition, error) {
	return FetchAnthropicModels(ctx, apiKey)
}

// FetchMiniMaxModels_Discovery fetches models from MiniMax for the discovery manager.
func FetchMiniMaxModels_Discovery(ctx context.Context, apiKey string) ([]registry.ModelDefinition, error) {
	return FetchMiniMaxModels(ctx, apiKey)
}

// InferOpenAICapabilities tries to deduce model features from its ID
func InferOpenAICapabilities(modelID string) registry.CapabilitySet {
	caps := registry.CapabilitySet{
		MaxContextTokens: 4096,  // Conservative default
		SupportsStreaming: true,
	}

	lowerID := strings.ToLower(modelID)

	// xAI Specifics
	if strings.Contains(lowerID, "grok") {
		if strings.Contains(lowerID, "3") || strings.Contains(lowerID, "4") ||
			strings.Contains(lowerID, "beta") || strings.Contains(lowerID, "code") {
			caps.MaxContextTokens = 131072
			caps.SupportsTools = true
			caps.SupportsVision = true
			caps.SupportsJSONMode = true
			// Only grok-3-mini supports reasoning_effort; all others (incl. grok-4*-reasoning) do not.
			caps.SupportsThinking = grokModelSupportsThinking(lowerID)
		} else {
			// Grok-2 / Grok-1 legacy
			caps.MaxContextTokens = 8192
		}
	}

	// OpenAI Specifics (for future proofing)
	if strings.Contains(lowerID, "gpt-4") {
		caps.MaxContextTokens = 8192
		if strings.Contains(lowerID, "turbo") || strings.Contains(lowerID, "o") {
			caps.MaxContextTokens = 128000
			caps.SupportsVision = true
			caps.SupportsTools = true
			caps.SupportsJSONMode = true
		}
	} else if strings.Contains(lowerID, "gpt-3.5") {
		caps.MaxContextTokens = 16385
	}

	return caps
}
