package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/registry"
)

// OpenAIProvider implements AIProvider for OpenAI's chat completions API.
// It reuses the same OpenAI-compatible format as GrokProvider.
type OpenAIProvider struct {
	APIKey    string
	Model     string
	BaseURL   string // default: "https://api.openai.com/v1"
	client    *http.Client
	isOSeries bool // true for o1/o3/o4-mini etc. (reasoning models)
}

// NewOpenAIProvider creates a new OpenAI provider.
func NewOpenAIProvider(apiKey, model string) *OpenAIProvider {
	if model == "" {
		model = "gpt-4o"
	}
	return &OpenAIProvider{
		APIKey:    apiKey,
		Model:     model,
		BaseURL:   "https://api.openai.com/v1",
		client:    NewRetryClient(),
		isOSeries: openAIIsOSeries(model),
	}
}

func (o *OpenAIProvider) Name() string { return "OpenAI" }

func (o *OpenAIProvider) ID() registry.ProviderID { return "openai" }

func (o *OpenAIProvider) GetMetadata() ProviderMetadata {
	return ProviderMetadata{
		ID:          o.Model,
		Name:        "OpenAI (" + o.Model + ")",
		Description: "OpenAI's GPT family of language models.",
		ContextSize: 128000,
	}
}

func (o *OpenAIProvider) WithModel(model string) AIProvider {
	return &OpenAIProvider{
		APIKey:    o.APIKey,
		Model:     model,
		BaseURL:   o.BaseURL,
		client:    o.client,
		isOSeries: openAIIsOSeries(model),
	}
}

// Ping validates the OpenAI key with a single lightweight GET /v1/models request.
// Uses NewPingClient (5 s hard timeout, no retries) so it never blocks the UI.
func (o *OpenAIProvider) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", o.BaseURL+"/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+o.APIKey)
	resp, err := NewPingClient().Do(req)
	if err != nil {
		return fmt.Errorf("OpenAI unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("OpenAI key invalid (%d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// FetchModels returns the live model list from OpenAI, falling back to safe statics on failure.
func (o *OpenAIProvider) FetchModels(ctx context.Context) ([]registry.ModelDefinition, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}
	models, err := FetchOpenAIModels_OpenAI(ctx, o.APIKey)
	if err != nil || len(models) == 0 {
		return SafeModelDefs("openai"), nil
	}
	return models, nil
}

func (o *OpenAIProvider) Generate(ctx context.Context, prompt string) (string, error) {
	hist := &ConversationHistory{}
	hist.AddMessage("user", prompt)
	return o.GenerateWithHistory(ctx, hist)
}

func (o *OpenAIProvider) GenerateWithHistory(ctx context.Context, history *ConversationHistory) (string, error) {
	msgs := o.convertHistory(history)

	reqBody := GrokRequest{
		Model:     o.Model,
		Messages:  msgs,
		Stream:    false,
		MaxTokens: 16384,
	}
	// o-series models handle reasoning internally; do NOT send reasoning_effort
	// (they don't support it from the request side the same way)

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.BaseURL+"/chat/completions", bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.APIKey)

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

func (o *OpenAIProvider) Stream(ctx context.Context, prompt string, out io.Writer) error {
	hist := &ConversationHistory{}
	hist.AddMessage("user", prompt)
	return o.StreamWithHistory(ctx, hist, out)
}

func (o *OpenAIProvider) StreamWithHistory(ctx context.Context, history *ConversationHistory, out io.Writer) error {
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

	req, err := http.NewRequestWithContext(ctx, "POST", o.BaseURL+"/chat/completions", bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.APIKey)

	streamClient := NewRetryClient()
	resp, err := streamClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	guard := NewStreamGuard()
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		guard.ObserveLine(line)
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
			content := chunk.Choices[0].Delta.Content
			guard.ObserveContent(content)
			fmt.Fprint(out, content)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	// Detect stream dropout: HTTP 200 but no [DONE] or finish_reason terminal.
	// On mobile networks (Termux/Android) SSE connections frequently die silently.
	// Retry up to 2 times transparently before accepting the partial response.
	if !guard.WasComplete() {
		retries, _ := ctx.Value(streamRetriesKey).(int)
		if retries < 2 {
			fmt.Fprint(out, "[__GORKBOT_STREAM_RETRY__]")
			retryCtx := context.WithValue(ctx, streamRetriesKey, retries+1)
			return o.StreamWithHistory(retryCtx, history, out)
		}
		// Max retries reached — log the incomplete stream for diagnostics
		partial := guard.PartialContent()
		if len(partial) > 100 {
			partial = partial[:100] + "..."
		}
		slog.Warn("Stream incomplete after max retries",
			"provider", "openai",
			"partial_length", len(guard.PartialContent()),
			"partial_preview", partial)
		// Return nil so partial content is visible to user
		return nil
	}
	return nil
}

func (o *OpenAIProvider) convertHistory(history *ConversationHistory) []GrokMessage {
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

// openAIIsOSeries returns true for o1, o3, o4-mini etc.
// These models do reasoning internally — we tag them for display with 🧠
// but do NOT send any special parameter.
func openAIIsOSeries(modelID string) bool {
	lower := strings.ToLower(modelID)
	return strings.HasPrefix(lower, "o1") ||
		strings.HasPrefix(lower, "o3") ||
		strings.HasPrefix(lower, "o4")
}

// isOpenAIChatModel returns true if the model is a chat/instruction-tuned model.
// We filter out embeddings, dall-e, whisper, tts from the listing.
func isOpenAIChatModel(modelID string) bool {
	lower := strings.ToLower(modelID)
	// Exclude non-chat models
	for _, prefix := range []string{"text-embedding", "dall-e", "whisper", "tts", "babbage", "davinci"} {
		if strings.HasPrefix(lower, prefix) {
			return false
		}
	}
	// Include gpt-* and o-series
	return strings.HasPrefix(lower, "gpt-") ||
		strings.HasPrefix(lower, "o1") ||
		strings.HasPrefix(lower, "o3") ||
		strings.HasPrefix(lower, "o4") ||
		strings.HasPrefix(lower, "chatgpt")
}

// FetchOpenAIModels_OpenAI retrieves chat-capable models from api.openai.com.
func FetchOpenAIModels_OpenAI(ctx context.Context, apiKey string) ([]registry.ModelDefinition, error) {
	url := "https://api.openai.com/v1/models"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

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

	var listResp OpenAIModelListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("failed to decode model list: %w", err)
	}

	var models []registry.ModelDefinition
	for _, m := range listResp.Data {
		if !isOpenAIChatModel(m.ID) {
			continue
		}
		ctx128k := 128000
		if strings.Contains(m.ID, "gpt-4o") || strings.Contains(m.ID, "gpt-4-turbo") {
			ctx128k = 128000
		} else if strings.Contains(m.ID, "gpt-4") {
			ctx128k = 8192
		} else if strings.Contains(m.ID, "gpt-3.5") {
			ctx128k = 16385
		}
		models = append(models, registry.ModelDefinition{
			ID:       registry.ModelID(m.ID),
			Provider: "openai",
			Name:     m.ID,
			Capabilities: registry.CapabilitySet{
				MaxContextTokens:  ctx128k,
				SupportsStreaming: true,
				SupportsTools:     strings.Contains(m.ID, "gpt-4") || strings.HasPrefix(m.ID, "o"),
				SupportsJSONMode:  strings.Contains(m.ID, "gpt-4"),
				SupportsThinking:  openAIIsOSeries(m.ID),
			},
			Status:      registry.StatusActive,
			LastUpdated: time.Now(),
		})
	}
	return models, nil
}
