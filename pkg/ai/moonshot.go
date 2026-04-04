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

type MoonshotProvider struct {
	APIKey string
	Model  string
	client *http.Client
}

func NewMoonshotProvider(apiKey, model string) *MoonshotProvider {
	if model == "" {
		model = "moonshot-v1-8k"
	}
	return &MoonshotProvider{
		APIKey: apiKey,
		Model:  model,
		client: NewRetryClient(),
	}
}

func (m *MoonshotProvider) Name() string            { return "Moonshot" }
func (m *MoonshotProvider) ID() registry.ProviderID { return "moonshot" }
func (m *MoonshotProvider) GetMetadata() ProviderMetadata {
	ctxSize := 8192
	lower := strings.ToLower(m.Model)
	if strings.Contains(lower, "32k") {
		ctxSize = 32768
	} else if strings.Contains(lower, "128k") {
		ctxSize = 131072
	} else if strings.Contains(lower, "256k") || strings.Contains(lower, "k2.5") {
		ctxSize = 131072 // Safe assumption for k2.5
	}

	return ProviderMetadata{
		ID:          m.Model,
		Name:        "Moonshot (" + m.Model + ")",
		Description: "Moonshot AI models (Kimi).",
		ContextSize: ctxSize,
	}
}
func (m *MoonshotProvider) WithModel(model string) AIProvider {
	return &MoonshotProvider{
		APIKey: m.APIKey,
		Model:  model,
		client: m.client,
	}
}

func (m *MoonshotProvider) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.moonshot.ai/v1/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.APIKey)
	resp, err := NewPingClient().Do(req)
	if err != nil {
		return fmt.Errorf("Moonshot unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Moonshot key invalid (%d): %s", resp.StatusCode, string(body))
	}
	return nil
}

func (m *MoonshotProvider) FetchModels(ctx context.Context) ([]registry.ModelDefinition, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}
	url := "https://api.moonshot.ai/v1/models"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+m.APIKey)
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var defs []registry.ModelDefinition
	for _, md := range result.Data {
		defs = append(defs, registry.ModelDefinition{
			ID:          registry.ModelID(md.ID),
			Name:        md.ID,
			Provider:    "moonshot",
			Description: "Moonshot " + md.ID,
		})
	}
	return defs, nil
}

func (m *MoonshotProvider) isK25Series() bool {
	return strings.Contains(strings.ToLower(m.Model), "k2.5") || strings.Contains(strings.ToLower(m.Model), "kimi-k2.5")
}

func (m *MoonshotProvider) Generate(ctx context.Context, prompt string) (string, error) {
	hist := &ConversationHistory{}
	hist.AddMessage("user", prompt)
	return m.GenerateWithHistory(ctx, hist)
}

func (m *MoonshotProvider) getReqBody(msgs []GrokMessage, stream bool) GrokRequest {
	reqBody := GrokRequest{
		Model:    m.Model,
		Messages: msgs,
		Stream:   stream,
	}
	if m.isK25Series() {
		temp := float32(1.0)
		topP := float32(0.95)
		reqBody.Temperature = &temp
		reqBody.TopP = &topP
	}
	return reqBody
}

func (m *MoonshotProvider) GenerateWithHistory(ctx context.Context, history *ConversationHistory) (string, error) {
	msgs := m.convertHistory(history)
	reqBody := m.getReqBody(msgs, false)

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.moonshot.ai/v1/chat/completions", bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.APIKey)

	resp, err := m.client.Do(req)
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

func (m *MoonshotProvider) Stream(ctx context.Context, prompt string, out io.Writer) error {
	hist := &ConversationHistory{}
	hist.AddMessage("user", prompt)
	return m.StreamWithHistory(ctx, hist, out)
}

func (m *MoonshotProvider) StreamWithHistory(ctx context.Context, history *ConversationHistory, out io.Writer) error {
	msgs := m.convertHistory(history)
	reqBody := m.getReqBody(msgs, true)

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.moonshot.ai/v1/chat/completions", bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.APIKey)

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

	if !guard.WasComplete() {
		retries, _ := ctx.Value(streamRetriesKey).(int)
		if retries < 2 {
			fmt.Fprint(out, "[__GORKBOT_STREAM_RETRY__]")
			retryCtx := context.WithValue(ctx, streamRetriesKey, retries+1)
			return m.StreamWithHistory(retryCtx, history, out)
		}
		partial := guard.PartialContent()
		if len(partial) > 100 {
			partial = partial[:100] + "..."
		}
		slog.Warn("Stream incomplete after max retries",
			"provider", "moonshot",
			"partial_length", len(guard.PartialContent()),
			"partial_preview", partial)
		return nil
	}
	return nil
}

func (m *MoonshotProvider) convertHistory(history *ConversationHistory) []GrokMessage {
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
