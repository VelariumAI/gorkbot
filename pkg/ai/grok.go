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

// GrokRequest represents the payload for xAI's chat completions.
type GrokRequest struct {
	Model    string        `json:"model"`
	Messages []GrokMessage `json:"messages"`
	Stream   bool          `json:"stream,omitempty"`
	// MaxTokens sets the upper bound on response length.
	// Grok-3 supports up to 131072 output tokens. Omit (zero) falls back to a
	// conservative API default (~4096) that truncates long agentic tasks.
	MaxTokens int `json:"max_tokens,omitempty"`
	// ReasoningEffort controls native chain-of-thought depth on models that support it
	// (e.g. grok-3-mini family). Valid values: "low", "medium", "high".
	// Omit entirely for models that do not support it — sending it to unsupported
	// models returns a 400 Bad Request.
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	Temperature     *float32 `json:"temperature,omitempty"`
	TopP            *float32 `json:"top_p,omitempty"`
}

type GrokMessage struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	ToolCalls []GrokToolCall `json:"tool_calls,omitempty"`
}

// ── Native function calling types ────────────────────────────────────────────

// GrokToolSchema represents a function available for the AI to call.
type GrokToolSchema struct {
	Type     string          `json:"type"` // always "function"
	Function GrokFunctionDef `json:"function"`
}

// GrokFunctionDef describes a single function the AI may invoke.
type GrokFunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // JSON Schema object
}

// GrokToolCall appears in assistant responses when the model calls a function.
type GrokToolCall struct {
	ID       string              `json:"id"`
	Type     string              `json:"type"` // "function"
	Function GrokFunctionCallDef `json:"function"`
}

// GrokFunctionCallDef holds the called function name and its JSON-encoded arguments.
type GrokFunctionCallDef struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// NativeToolResult is returned by GenerateWithTools.
// Either Content or ToolCalls is populated (never both).
type NativeToolResult struct {
	Content   string         // Final answer text (no tool calls)
	ToolCalls []GrokToolCall // Requested tool invocations
}

// NativeToolCaller is implemented by providers that support native function calling.
type NativeToolCaller interface {
	GenerateWithTools(ctx context.Context, history *ConversationHistory, tools []GrokToolSchema) (*NativeToolResult, error)
}

// grokNativeMsg is the full per-message structure for native-call requests.
// We keep it separate from GrokMessage to avoid polluting the standard path
// with fields that must sometimes be null (e.g. content for tool-call turns).
type grokNativeMsg struct {
	Role       string         `json:"role"`
	Content    interface{}    `json:"content"`                // string or nil
	ToolCalls  []GrokToolCall `json:"tool_calls,omitempty"`   // assistant → tool calls
	ToolCallID string         `json:"tool_call_id,omitempty"` // tool result → call id
}

// grokNativeRequest is the request body for native function calling.
type grokNativeRequest struct {
	Model           string           `json:"model"`
	Messages        []grokNativeMsg  `json:"messages"`
	Tools           []GrokToolSchema `json:"tools,omitempty"`
	ToolChoice      string           `json:"tool_choice,omitempty"`
	Stream          bool             `json:"stream,omitempty"`
	MaxTokens       int              `json:"max_tokens,omitempty"`
	ReasoningEffort string           `json:"reasoning_effort,omitempty"`
}

// GrokUsage carries token counts from xAI API responses.
type GrokUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// GrokResponse represents the full response.
type GrokResponse struct {
	Choices []struct {
		Message GrokMessage `json:"message"`
	} `json:"choices"`
	Usage GrokUsage `json:"usage"`
}

// UsageReporter is implemented by providers that surface per-call token counts.
type UsageReporter interface {
	LastUsage() GrokUsage
}

// GrokStreamResponse represents a chunk from the SSE stream.
type GrokStreamResponse struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

type GrokProvider struct {
	APIKey           string
	Model            string
	Client           *http.Client
	supportsThinking bool // true when the active model accepts reasoning_effort
	lastUsage        GrokUsage
}

// LastUsage returns the token usage from the most recent API call.
// Implements UsageReporter.
func (g *GrokProvider) LastUsage() GrokUsage { return g.lastUsage }

func NewGrokProvider(apiKey string, defaultModel string) *GrokProvider {
	model := defaultModel
	if model == "" {
		model = "grok-3"
	}
	return &GrokProvider{
		APIKey:           apiKey,
		Model:            model,
		Client:           NewRetryClient(),
		supportsThinking: grokModelSupportsThinking(model),
	}
}

func (g *GrokProvider) Name() string {
	return "Grok"
}

func (g *GrokProvider) GetMetadata() ProviderMetadata {
	return ProviderMetadata{
		ID:          g.Model,
		Name:        "Grok-3",
		Description: "xAI's witty and rebellious LLM.",
		ContextSize: 128000,
	}
}

// ID returns the provider identifier
func (g *GrokProvider) ID() registry.ProviderID {
	return "xai"
}

// FetchModels returns the live model list from xAI, falling back to safe statics on failure.
func (g *GrokProvider) FetchModels(ctx context.Context) ([]registry.ModelDefinition, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}
	models, err := FetchOpenAIModels(ctx, "https://api.x.ai", g.APIKey)
	if err != nil || len(models) == 0 {
		return SafeModelDefs("xai"), nil
	}
	for i := range models {
		models[i].Provider = g.ID()
	}
	return models, nil
}

// WithModel returns a new instance of the provider configured to use the specified model.
// It automatically detects whether the new model supports the reasoning_effort parameter.
func (g *GrokProvider) WithModel(model string) AIProvider {
	return &GrokProvider{
		APIKey:           g.APIKey,
		Model:            model,
		Client:           g.Client,
		supportsThinking: grokModelSupportsThinking(model),
	}
}

func (g *GrokProvider) Generate(ctx context.Context, prompt string) (string, error) {
	url := "https://api.x.ai/v1/chat/completions"

	reqBody := GrokRequest{
		Model: g.Model,
		Messages: []GrokMessage{
			{Role: "user", Content: prompt},
		},
		Stream:    false,
		MaxTokens: 131072,
	}
	// Only send reasoning_effort for models that support it — prevents 400 errors.
	if g.supportsThinking {
		reqBody.ReasoningEffort = "high"
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.APIKey)

	resp, err := g.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var result GrokResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	g.lastUsage = result.Usage
	return result.Choices[0].Message.Content, nil
}

func (g *GrokProvider) GenerateWithHistory(ctx context.Context, history *ConversationHistory) (string, error) {
	url := "https://api.x.ai/v1/chat/completions"

	// Convert conversation history to Grok messages
	messages := g.convertHistoryToMessages(history)

	reqBody := GrokRequest{
		Model:     g.Model,
		Messages:  messages,
		Stream:    false,
		MaxTokens: 131072,
	}
	// Only send reasoning_effort for models that support it — prevents 400 errors.
	if g.supportsThinking {
		reqBody.ReasoningEffort = "high"
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.APIKey)

	resp, err := g.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var result GrokResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	g.lastUsage = result.Usage
	return result.Choices[0].Message.Content, nil
}

func (g *GrokProvider) Stream(ctx context.Context, prompt string, out io.Writer) error {
	url := "https://api.x.ai/v1/chat/completions"

	reqBody := GrokRequest{
		Model: g.Model,
		Messages: []GrokMessage{
			{Role: "user", Content: prompt},
		},
		Stream:    true,
		MaxTokens: 131072,
	}
	// Only send reasoning_effort for models that support it — prevents 400 errors.
	if g.supportsThinking {
		reqBody.ReasoningEffort = "high"
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.APIKey)

	// Use a client with no timeout for streaming, or long timeout
	streamClient := NewRetryClient()
	resp, err := streamClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return MapStatusError(resp.StatusCode, bodyBytes)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		// Parse SSE
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk GrokStreamResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			// Skip malformed chunks or log error?
			continue
		}

		if len(chunk.Choices) > 0 {
			content := chunk.Choices[0].Delta.Content
			if content != "" {
				fmt.Fprint(out, content)
				// Ensure it flushes to terminal immediately if out is os.Stdout
				if f, ok := out.(http.Flusher); ok {
					f.Flush()
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stream error: %w", err)
	}

	return nil
}

func (g *GrokProvider) StreamWithHistory(ctx context.Context, history *ConversationHistory, out io.Writer) error {
	url := "https://api.x.ai/v1/chat/completions"

	// Convert conversation history to Grok messages
	messages := g.convertHistoryToMessages(history)

	reqBody := GrokRequest{
		Model:     g.Model,
		Messages:  messages,
		Stream:    true,
		MaxTokens: 131072,
	}
	// Only send reasoning_effort for models that support it — prevents 400 errors.
	if g.supportsThinking {
		reqBody.ReasoningEffort = "high"
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.APIKey)

	// Use a client with no timeout for streaming, or long timeout
	streamClient := NewRetryClient()
	resp, err := streamClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	guard := NewStreamGuard()
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		guard.ObserveLine(line)

		// Parse SSE
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk GrokStreamResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			// Skip malformed chunks or log error?
			continue
		}

		if len(chunk.Choices) > 0 {
			content := chunk.Choices[0].Delta.Content
			if content != "" {
				guard.ObserveContent(content)
				fmt.Fprint(out, content)
				// Ensure it flushes to terminal immediately if out is os.Stdout
				if f, ok := out.(http.Flusher); ok {
					f.Flush()
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stream error: %w", err)
	}

	// Detect stream dropout: HTTP 200 but no [DONE] or finish_reason terminal.
	// On mobile networks (Termux/Android) SSE connections frequently die silently.
	// Retry up to 2 times transparently before accepting the partial response.
	if !guard.WasComplete() {
		retries, _ := ctx.Value(streamRetriesKey).(int)
		if retries < 2 {
			fmt.Fprint(out, "[__GORKBOT_STREAM_RETRY__]")
			retryCtx := context.WithValue(ctx, streamRetriesKey, retries+1)
			return g.StreamWithHistory(retryCtx, history, out)
		}
		// Max retries reached — log the incomplete stream for diagnostics
		partial := guard.PartialContent()
		if len(partial) > 100 {
			partial = partial[:100] + "..."
		}
		slog.Warn("Stream incomplete after max retries",
			"provider", "grok",
			"partial_length", len(guard.PartialContent()),
			"partial_preview", partial)
		// Return nil so partial content is visible to user
		return nil
	}

	return nil
}

// convertHistoryToMessages converts ConversationHistory to Grok messages for the
// standard streaming path.  It sanitises any native function-calling artefacts
// that may have been written by ExecuteTaskWithTools (one-shot mode) so the
// streaming API never receives role:"tool" messages or empty-content assistant
// messages — both of which would cause a 400 from the xAI chat completions API.
func (g *GrokProvider) convertHistoryToMessages(history *ConversationHistory) []GrokMessage {
	messages := []GrokMessage{}

	for _, msg := range history.GetMessages() {
		switch {
		case msg.Role == "tool":
			// Native tool-result message → surface as user context so the model
			// still sees the tool output without an invalid role.
			messages = append(messages, GrokMessage{
				Role:    "user",
				Content: fmt.Sprintf("[Tool result: %s]\n%s", msg.ToolName, msg.Content),
			})

		case len(msg.ToolCalls) > 0:
			// Native assistant message that only contains tool_calls (empty Content).
			// Summarise as a plain assistant text so the conversation remains coherent.
			toolNames := make([]string, len(msg.ToolCalls))
			for i, tc := range msg.ToolCalls {
				toolNames[i] = tc.ToolName
			}
			messages = append(messages, GrokMessage{
				Role:    "assistant",
				Content: fmt.Sprintf("[Calling tools: %s]", strings.Join(toolNames, ", ")),
			})

		case msg.Content == "" && msg.Role != "system":
			// Skip empty non-system messages — they add no value and some API
			// versions reject content:"" on user/assistant turns.
			continue

		default:
			messages = append(messages, GrokMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	return messages
}

// GenerateWithTools uses the xAI native function-calling API.
// It sends the conversation history along with tool schemas, and returns either
// a final text response or a set of tool call requests from the model.
// Implements NativeToolCaller.
func (g *GrokProvider) GenerateWithTools(ctx context.Context, history *ConversationHistory, tools []GrokToolSchema) (*NativeToolResult, error) {
	url := "https://api.x.ai/v1/chat/completions"

	msgs := g.convertHistoryToNativeMsgs(history)

	reqBody := grokNativeRequest{
		Model:      g.Model,
		Messages:   msgs,
		Tools:      tools,
		ToolChoice: "auto",
		MaxTokens:  131072,
	}
	if g.supportsThinking {
		reqBody.ReasoningEffort = "high"
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal native request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.APIKey)

	resp, err := g.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var result GrokResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	g.lastUsage = result.Usage
	msg := result.Choices[0].Message

	// If the model returned tool calls, surface them.
	if len(msg.ToolCalls) > 0 {
		return &NativeToolResult{ToolCalls: msg.ToolCalls}, nil
	}
	return &NativeToolResult{Content: msg.Content}, nil
}

// convertHistoryToNativeMsgs converts ConversationHistory into the richer
// grokNativeMsg format that supports null content and tool_call_id fields.
func (g *GrokProvider) convertHistoryToNativeMsgs(history *ConversationHistory) []grokNativeMsg {
	var out []grokNativeMsg
	for _, m := range history.GetMessages() {
		switch {
		case len(m.ToolCalls) > 0:
			// Assistant message that contains tool call requests.
			calls := make([]GrokToolCall, len(m.ToolCalls))
			for i, tc := range m.ToolCalls {
				calls[i] = GrokToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: GrokFunctionCallDef{
						Name:      tc.ToolName,
						Arguments: tc.Arguments,
					},
				}
			}
			out = append(out, grokNativeMsg{
				Role:      "assistant",
				Content:   nil, // null per OpenAI/xAI spec
				ToolCalls: calls,
			})
		case m.Role == "tool":
			// Tool result message.
			out = append(out, grokNativeMsg{
				Role:       "tool",
				Content:    m.Content,
				ToolCallID: m.ToolCallID,
			})
		default:
			out = append(out, grokNativeMsg{
				Role:    m.Role,
				Content: m.Content,
			})
		}
	}
	return out
}

// Ping validates the xAI key with a single lightweight GET /v1/models request.
// Uses NewPingClient (5 s hard timeout, no retries) so it never blocks the UI.
func (g *GrokProvider) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.x.ai/v1/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+g.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := NewPingClient().Do(req)
	if err != nil {
		return fmt.Errorf("xAI unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("xAI key invalid (%d): %s", resp.StatusCode, string(body))
	}
	return nil
}
