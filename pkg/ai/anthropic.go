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

// ── Anthropic wire types ──────────────────────────────────────────────────────

type anthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string or []anthropicBlock
}

type anthropicBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Stream    bool               `json:"stream,omitempty"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicResponse struct {
	Content []anthropicContentBlock `json:"content"`
	Error   *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// SSE event types for Anthropic streaming
type anthropicStreamEvent struct {
	Type  string `json:"type"`
	Delta *struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta,omitempty"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Anthropic model listing response
type anthropicModelListResponse struct {
	Data []struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"data"`
}

// ── AnthropicProvider ─────────────────────────────────────────────────────────

// AnthropicProvider implements AIProvider for Anthropic's Claude API.
type AnthropicProvider struct {
	APIKey           string
	BaseURL          string // default: "https://api.anthropic.com/v1"
	Model            string
	client           *http.Client
	supportsThinking bool
	bearerAuth       bool // if true, use "Authorization: Bearer" instead of "x-api-key" (for MiniMax compat)
}

// NewAnthropicProvider creates a new Anthropic provider.
func NewAnthropicProvider(apiKey, model string) *AnthropicProvider {
	if model == "" {
		model = "claude-sonnet-4-5"
	}
	return &AnthropicProvider{
		APIKey:           apiKey,
		BaseURL:          "https://api.anthropic.com/v1",
		Model:            model,
		client:           NewRetryClient(),
		supportsThinking: anthropicModelSupportsThinking(model),
	}
}

func (a *AnthropicProvider) Name() string { return "Claude" }

func (a *AnthropicProvider) ID() registry.ProviderID { return "anthropic" }

func (a *AnthropicProvider) GetMetadata() ProviderMetadata {
	return ProviderMetadata{
		ID:          a.Model,
		Name:        "Claude (" + a.Model + ")",
		Description: "Anthropic's Claude — helpful, harmless, and honest.",
		ContextSize: 200000,
	}
}

func (a *AnthropicProvider) WithModel(model string) AIProvider {
	return &AnthropicProvider{
		APIKey:           a.APIKey,
		BaseURL:          a.BaseURL,
		Model:            model,
		client:           a.client,
		supportsThinking: anthropicModelSupportsThinking(model),
	}
}

// Ping validates the Anthropic key with a single lightweight GET /v1/models request.
// Uses NewPingClient (5 s hard timeout, no retries) so it never blocks the UI.
func (a *AnthropicProvider) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", a.BaseURL+"/models?limit=1", nil)
	if err != nil {
		return err
	}
	a.setHeaders(req)
	resp, err := NewPingClient().Do(req)
	if err != nil {
		return fmt.Errorf("Anthropic unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Anthropic key invalid (%d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// FetchModels returns the live model list from Anthropic, falling back to safe statics on failure.
func (a *AnthropicProvider) FetchModels(ctx context.Context) ([]registry.ModelDefinition, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}
	models, err := FetchAnthropicModels(ctx, a.APIKey)
	if err != nil || len(models) == 0 {
		return SafeModelDefs("anthropic"), nil
	}
	return models, nil
}

func (a *AnthropicProvider) Generate(ctx context.Context, prompt string) (string, error) {
	hist := &ConversationHistory{}
	hist.AddMessage("user", prompt)
	return a.GenerateWithHistory(ctx, hist)
}

func (a *AnthropicProvider) GenerateWithHistory(ctx context.Context, history *ConversationHistory) (string, error) {
	systemMsg, msgs := a.convertHistory(history)

	reqBody := anthropicRequest{
		Model:     a.Model,
		MaxTokens: 16384,
		System:    systemMsg,
		Messages:  msgs,
		Stream:    false,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", a.BaseURL+"/messages", bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	a.setHeaders(req)

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	var result anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}
	if result.Error != nil {
		return "", fmt.Errorf("API error: %s", result.Error.Message)
	}

	var sb strings.Builder
	for _, block := range result.Content {
		if block.Type == "text" {
			sb.WriteString(block.Text)
		}
	}
	return sb.String(), nil
}

func (a *AnthropicProvider) Stream(ctx context.Context, prompt string, out io.Writer) error {
	hist := &ConversationHistory{}
	hist.AddMessage("user", prompt)
	return a.StreamWithHistory(ctx, hist, out)
}

func (a *AnthropicProvider) StreamWithHistory(ctx context.Context, history *ConversationHistory, out io.Writer) error {
	systemMsg, msgs := a.convertHistory(history)

	reqBody := anthropicRequest{
		Model:     a.Model,
		MaxTokens: 16384,
		System:    systemMsg,
		Messages:  msgs,
		Stream:    true,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", a.BaseURL+"/messages", bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	a.setHeaders(req)

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

		var event anthropicStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		if event.Type == "content_block_delta" && event.Delta != nil && event.Delta.Type == "text_delta" {
			guard.ObserveContent(event.Delta.Text)
			fmt.Fprint(out, event.Delta.Text)
		}
		if event.Type == "error" && event.Error != nil {
			return fmt.Errorf("stream error: %s", event.Error.Message)
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	// Detect stream dropout: HTTP 200 but no message_stop or finish terminal.
	// On mobile networks (Termux/Android) SSE connections frequently die silently.
	// Retry up to 2 times transparently before accepting the partial response.
	if !guard.WasComplete() {
		retries, _ := ctx.Value(streamRetriesKey).(int)
		if retries < 2 {
			fmt.Fprint(out, "[__GORKBOT_STREAM_RETRY__]")
			retryCtx := context.WithValue(ctx, streamRetriesKey, retries+1)
			return a.StreamWithHistory(retryCtx, history, out)
		}
		// Max retries reached — log the incomplete stream for diagnostics
		partial := guard.PartialContent()
		if len(partial) > 100 {
			partial = partial[:100] + "..."
		}
		slog.Warn("Stream incomplete after max retries",
			"provider", "anthropic",
			"partial_length", len(guard.PartialContent()),
			"partial_preview", partial)
		// Return nil so partial content is visible to user
		return nil
	}
	return nil
}

// convertHistory splits system messages and converts history to Anthropic format.
// Anthropic requires alternating user/assistant turns; we merge consecutive same-role messages.
func (a *AnthropicProvider) convertHistory(history *ConversationHistory) (systemMsg string, msgs []anthropicMessage) {
	for _, msg := range history.GetMessages() {
		if msg.Role == "system" {
			if systemMsg != "" {
				systemMsg += "\n\n"
			}
			systemMsg += msg.Content
			continue
		}
		role := msg.Role
		if role == "tool" {
			role = "user"
		}
		// Merge consecutive same-role messages
		if len(msgs) > 0 && msgs[len(msgs)-1].Role == role {
			prev := msgs[len(msgs)-1]
			prevStr, ok := prev.Content.(string)
			if ok {
				msgs[len(msgs)-1].Content = prevStr + "\n\n" + msg.Content
			}
		} else {
			msgs = append(msgs, anthropicMessage{Role: role, Content: msg.Content})
		}
	}
	// Anthropic requires first message to be user
	if len(msgs) > 0 && msgs[0].Role != "user" {
		msgs = append([]anthropicMessage{{Role: "user", Content: "(start)"}}, msgs...)
	}
	return
}

func (a *AnthropicProvider) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if a.bearerAuth {
		req.Header.Set("Authorization", "Bearer "+a.APIKey)
	} else {
		req.Header.Set("x-api-key", a.APIKey)
	}
	req.Header.Set("anthropic-version", "2023-06-01")
}

// anthropicModelSupportsThinking returns true for models that natively support extended thinking.
func anthropicModelSupportsThinking(modelID string) bool {
	lower := strings.ToLower(modelID)
	// claude-3-7+ and claude-sonnet-4+ (claude-4 family) support extended thinking
	return strings.Contains(lower, "claude-3-7") ||
		strings.Contains(lower, "claude-sonnet-4") ||
		strings.Contains(lower, "claude-opus-4") ||
		strings.Contains(lower, "claude-haiku-4")
}

// FetchAnthropicModels retrieves the model list from Anthropic's API.
func FetchAnthropicModels(ctx context.Context, apiKey string) ([]registry.ModelDefinition, error) {
	url := "https://api.anthropic.com/v1/models"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

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

	var listResp anthropicModelListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("failed to decode model list: %w", err)
	}

	var models []registry.ModelDefinition
	for _, m := range listResp.Data {
		name := m.DisplayName
		if name == "" {
			name = m.ID
		}
		models = append(models, registry.ModelDefinition{
			ID:       registry.ModelID(m.ID),
			Provider: "anthropic",
			Name:     name,
			Capabilities: registry.CapabilitySet{
				MaxContextTokens:  200000,
				SupportsStreaming: true,
				SupportsTools:     true,
				SupportsJSONMode:  true,
				SupportsThinking:  anthropicModelSupportsThinking(m.ID),
			},
			Status:      registry.StatusActive,
			LastUpdated: time.Now(),
		})
	}
	return models, nil
}

// ── Native Tool Calling (NativeToolCaller interface) ─────────────────────────

// anthropicTool is the Anthropic API's tool schema.
type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// anthropicToolRequest is the full request body for tool-enabled calls.
type anthropicToolRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
	Thinking  *anthropicThinking `json:"thinking,omitempty"`
}

// anthropicThinking enables extended thinking (claude-3-7+ / claude-4 family).
type anthropicThinking struct {
	Type         string `json:"type"` // "enabled"
	BudgetTokens int    `json:"budget_tokens"`
}

// anthropicToolResponse extends anthropicResponse for mixed content blocks.
type anthropicToolResponse struct {
	Content []struct {
		Type  string          `json:"type"` // "text" | "tool_use" | "thinking"
		Text  string          `json:"text,omitempty"`
		ID    string          `json:"id,omitempty"`
		Name  string          `json:"name,omitempty"`
		Input json.RawMessage `json:"input,omitempty"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Error      *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// GenerateWithTools implements NativeToolCaller for Anthropic's tool_use API.
// It maps GrokToolSchema (OpenAI-compatible) to Anthropic's input_schema format.
func (a *AnthropicProvider) GenerateWithTools(ctx context.Context, history *ConversationHistory, tools []GrokToolSchema) (*NativeToolResult, error) {
	systemMsg, msgs := a.convertHistory(history)

	anthTools := make([]anthropicTool, 0, len(tools))
	for _, t := range tools {
		anthTools = append(anthTools, anthropicTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		})
	}

	reqBody := anthropicToolRequest{
		Model:     a.Model,
		MaxTokens: 16384,
		System:    systemMsg,
		Messages:  msgs,
		Tools:     anthTools,
	}

	if a.supportsThinking {
		reqBody.Thinking = &anthropicThinking{Type: "enabled", BudgetTokens: 8000}
		reqBody.MaxTokens = 24000
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tool request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", a.BaseURL+"/messages", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	a.setHeaders(req)
	if a.supportsThinking {
		req.Header.Set("anthropic-beta", "thinking-2025-01-01")
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tool request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	var result anthropicToolResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode tool response: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("API error: %s", result.Error.Message)
	}

	var textParts []string
	var toolCalls []GrokToolCall
	for _, block := range result.Content {
		switch block.Type {
		case "text":
			if block.Text != "" {
				textParts = append(textParts, block.Text)
			}
		case "tool_use":
			argsJSON := "{}"
			if len(block.Input) > 0 {
				argsJSON = string(block.Input)
			}
			toolCalls = append(toolCalls, GrokToolCall{
				ID:   block.ID,
				Type: "function",
				Function: GrokFunctionCallDef{
					Name:      block.Name,
					Arguments: argsJSON,
				},
			})
		case "thinking":
			// Prepend extended thinking as an annotation
			if block.Text != "" {
				textParts = append([]string{"[Extended Thinking]\n" + block.Text + "\n---\n"}, textParts...)
			}
		}
	}

	if len(toolCalls) > 0 {
		return &NativeToolResult{ToolCalls: toolCalls}, nil
	}
	return &NativeToolResult{Content: strings.Join(textParts, "")}, nil
}
