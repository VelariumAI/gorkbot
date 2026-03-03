package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/registry"
)

const (
	openRouterBaseURL = "https://openrouter.ai/api/v1"
	openRouterReferer = "https://gorkbot.ai"
	openRouterTitle   = "Gorkbot"
)

// OpenRouterProvider implements AIProvider for OpenRouter's OpenAI-compatible API.
// OpenRouter is a gateway to 400+ models (Claude, GPT, Gemini, Llama, Mistral, etc.)
// via a single API key, using provider-prefixed model IDs like "anthropic/claude-opus-4-6".
type OpenRouterProvider struct {
	APIKey string
	Model  string
	client *http.Client
}

// NewOpenRouterProvider creates a new OpenRouter provider.
func NewOpenRouterProvider(apiKey, model string) *OpenRouterProvider {
	if model == "" {
		model = "anthropic/claude-opus-4-6"
	}
	return &OpenRouterProvider{
		APIKey: apiKey,
		Model:  model,
		client: NewRetryClient(),
	}
}

func (o *OpenRouterProvider) Name() string { return "OpenRouter" }

func (o *OpenRouterProvider) ID() registry.ProviderID { return "openrouter" }

func (o *OpenRouterProvider) GetMetadata() ProviderMetadata {
	return ProviderMetadata{
		ID:          o.Model,
		Name:        "OpenRouter (" + o.Model + ")",
		Description: "OpenRouter gateway — access 400+ models via a single API key.",
		ContextSize: 128000,
	}
}

func (o *OpenRouterProvider) WithModel(model string) AIProvider {
	return &OpenRouterProvider{
		APIKey: o.APIKey,
		Model:  model,
		client: o.client,
	}
}

// addHeaders sets the required OpenRouter headers on a request.
func (o *OpenRouterProvider) addHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+o.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", openRouterReferer)
	req.Header.Set("X-Title", openRouterTitle)
}

// Ping validates the OpenRouter key with a GET /v1/models request.
// Uses NewPingClient (5 s hard timeout, no retries) so it never blocks the UI.
func (o *OpenRouterProvider) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", openRouterBaseURL+"/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+o.APIKey)
	req.Header.Set("HTTP-Referer", openRouterReferer)
	req.Header.Set("X-Title", openRouterTitle)

	resp, err := NewPingClient().Do(req)
	if err != nil {
		return fmt.Errorf("OpenRouter unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("OpenRouter key invalid (%d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// openRouterModelEntry represents one model in the OpenRouter /v1/models response.
type openRouterModelEntry struct {
	ID            string                    `json:"id"`
	ContextLength int                       `json:"context_length"`
	Pricing       openRouterModelPricing    `json:"pricing"`
}

type openRouterModelPricing struct {
	Prompt     string `json:"prompt"`
	Completion string `json:"completion"`
}

type openRouterModelsResponse struct {
	Data []openRouterModelEntry `json:"data"`
}

// FetchModels returns the live model list from OpenRouter, falling back to safe statics on failure.
func (o *OpenRouterProvider) FetchModels(ctx context.Context) ([]registry.ModelDefinition, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
	}

	models, err := fetchOpenRouterModels(ctx, o.APIKey)
	if err != nil || len(models) == 0 {
		return SafeModelDefs("openrouter"), nil
	}
	return models, nil
}

// openRouterSupportsThinking returns true if the model ID hints at thinking/reasoning.
func openRouterSupportsThinking(modelID string) bool {
	lower := strings.ToLower(modelID)
	return strings.Contains(lower, "claude-3") ||
		strings.Contains(lower, "gemini") ||
		strings.Contains(lower, "reasoning") ||
		strings.Contains(lower, "think")
}

// fetchOpenRouterModels retrieves the live model list from the OpenRouter API.
func fetchOpenRouterModels(ctx context.Context, apiKey string) ([]registry.ModelDefinition, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", openRouterBaseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("HTTP-Referer", openRouterReferer)
	req.Header.Set("X-Title", openRouterTitle)

	client := NewRetryClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	var listResp openRouterModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("failed to decode model list: %w", err)
	}

	var models []registry.ModelDefinition
	for _, m := range listResp.Data {
		// Skip models with insufficient context length
		if m.ContextLength < 4096 {
			continue
		}

		// Parse pricing (format: "0.000001" = cost per token, convert to per-1M)
		var inputCost, outputCost float64
		if v, err := strconv.ParseFloat(m.Pricing.Prompt, 64); err == nil {
			inputCost = v * 1_000_000
		}
		if v, err := strconv.ParseFloat(m.Pricing.Completion, 64); err == nil {
			outputCost = v * 1_000_000
		}

		models = append(models, registry.ModelDefinition{
			ID:       registry.ModelID(m.ID),
			Provider: "openrouter",
			Name:     m.ID,
			Capabilities: registry.CapabilitySet{
				MaxContextTokens:  m.ContextLength,
				SupportsStreaming: true,
				SupportsTools:     true,
				SupportsThinking:  openRouterSupportsThinking(m.ID),
				InputCostPer1M:    inputCost,
				OutputCostPer1M:   outputCost,
			},
			Status:      registry.StatusActive,
			LastUpdated: time.Now(),
		})
	}
	return models, nil
}

func (o *OpenRouterProvider) Generate(ctx context.Context, prompt string) (string, error) {
	hist := &ConversationHistory{}
	hist.AddMessage("user", prompt)
	return o.GenerateWithHistory(ctx, hist)
}

func (o *OpenRouterProvider) GenerateWithHistory(ctx context.Context, history *ConversationHistory) (string, error) {
	msgs := o.convertHistory(history)

	reqBody := GrokRequest{
		Model:     o.Model,
		Messages:  msgs,
		Stream:    false,
		MaxTokens: 16384,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", openRouterBaseURL+"/chat/completions", bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	o.addHeaders(req)

	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	var result GrokResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	return result.Choices[0].Message.Content, nil
}

func (o *OpenRouterProvider) Stream(ctx context.Context, prompt string, out io.Writer) error {
	hist := &ConversationHistory{}
	hist.AddMessage("user", prompt)
	return o.StreamWithHistory(ctx, hist, out)
}

func (o *OpenRouterProvider) StreamWithHistory(ctx context.Context, history *ConversationHistory, out io.Writer) error {
	msgs := o.convertHistory(history)

	reqBody := GrokRequest{
		Model:     o.Model,
		Messages:  msgs,
		Stream:    true,
		MaxTokens: 16384,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", openRouterBaseURL+"/chat/completions", bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	o.addHeaders(req)

	streamClient := NewRetryClient()
	resp, err := streamClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return MapStatusError(resp.StatusCode, body)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		var chunk GrokStreamResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			fmt.Fprint(out, chunk.Choices[0].Delta.Content)
		}
	}
	return scanner.Err()
}

func (o *OpenRouterProvider) convertHistory(history *ConversationHistory) []GrokMessage {
	var msgs []GrokMessage
	for _, msg := range history.GetMessages() {
		role := msg.Role
		if role == "tool" {
			role = "user"
		}
		if msg.Content == "" && role != "system" {
			continue
		}
		msgs = append(msgs, GrokMessage{Role: role, Content: msg.Content})
	}
	return msgs
}
