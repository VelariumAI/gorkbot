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
	"os"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/registry"
)

type GeminiProvider struct {
	APIKey           string
	Model            string
	Client           *http.Client
	VerboseThoughts  bool
	ProjectID        string // Google Cloud Project ID for Vertex AI (Professional/Enterprise)
	Location         string // GCP Region (default: us-central1)
	supportsThinking bool   // true when the active model supports native thinking mode
}

// Gemini request structure
type GeminiRequest struct {
	Contents         []GeminiContent  `json:"contents"`
	GenerationConfig GeminiConfig     `json:"generationConfig,omitempty"`
}

type GeminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []GeminiPart `json:"parts"`
}

type GeminiPart struct {
	Text    string `json:"text,omitempty"`
	Thought bool   `json:"thought,omitempty"` 
}

type GeminiConfig struct {
	Temperature    float32         `json:"temperature,omitempty"`
	ThinkingConfig *ThinkingConfig `json:"thinking_config,omitempty"` // nil = omitted; only set for models that support it
}

type ThinkingConfig struct {
	IncludeThoughts bool `json:"include_thoughts"`
	// ThinkingBudget controls how many tokens the model may spend on internal
	// reasoning before producing output. Omit (zero) to use the model default.
	// Valid range varies by model; 0 = omitted (omitempty).
	ThinkingBudget int `json:"thinking_budget,omitempty"`
}

// Gemini response structure (non-streaming)
type GeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []GeminiPart `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
}

// Gemini stream chunk (SSE structure varies, but JSON stream usually contains array elements)
type GeminiStreamChunk struct {
	Candidates []struct {
		Content struct {
			Parts []GeminiPart `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func NewGeminiProvider(apiKey string, defaultModel string, verboseThoughts bool) *GeminiProvider {
	model := defaultModel
	if model == "" {
		model = "gemini-2.0-flash"
	}
	// Try to load project ID from env for enterprise/premium support
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	location := os.Getenv("GOOGLE_CLOUD_LOCATION")
	if location == "" {
		location = "us-central1"
	}

	return &GeminiProvider{
		APIKey:          apiKey,
		Model:           model,
		Client:          NewRetryClient(),
		VerboseThoughts: verboseThoughts,
		ProjectID:       projectID,
		Location:        location,
		supportsThinking: geminiModelSupportsThinking(model),
	}
}


func (g *GeminiProvider) Name() string {
	return "Gemini"
}

func (g *GeminiProvider) GetMetadata() ProviderMetadata {
	return ProviderMetadata{
		ID:          g.Model,
		Name:        "Gemini (" + g.Model + ")",
		Description: "Google's multimodal reasoning expert.",
		ContextSize: 1000000,
	}
}

// ID returns the provider identifier
func (g *GeminiProvider) ID() registry.ProviderID {
	return "google"
}

// FetchModels returns the live model list from Gemini, falling back to safe statics on failure.
func (g *GeminiProvider) FetchModels(ctx context.Context) ([]registry.ModelDefinition, error) {
	if g.APIKey == "" {
		return SafeModelDefs("google"), nil
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}
	models, err := FetchGeminiModels(ctx, g.APIKey)
	if err != nil || len(models) == 0 {
		return SafeModelDefs("google"), nil
	}
	return models, nil
}

// WithModel returns a new instance of the provider configured to use the specified model.
// It automatically detects whether the new model supports native thinking mode.
func (g *GeminiProvider) WithModel(model string) AIProvider {
	return &GeminiProvider{
		APIKey:           g.APIKey,
		Model:            model,
		Client:           g.Client,
		VerboseThoughts:  g.VerboseThoughts,
		supportsThinking: geminiModelSupportsThinking(model),
	}
}

func (g *GeminiProvider) getURL(streaming bool) string {
	method := "generateContent"
	if streaming {
		method = "streamGenerateContent"
	}

	// Professional Personal Premium Endpoint (Gemini API)
	// Base URL: generativelanguage.googleapis.com (Current standard for 2026)
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:%s", g.Model, method)
	if streaming {
		url += "?alt=sse"
	}
	if g.APIKey != "" {
		url += "&key=" + g.APIKey
	}
	return url
}

func (g *GeminiProvider) GenerateWithHistory(ctx context.Context, history *ConversationHistory) (string, error) {
	url := g.getURL(false)

	// Convert conversation history to Gemini contents
	contents := g.convertHistoryToContents(history)

	reqBody := GeminiRequest{
		Contents: contents,
		GenerationConfig: GeminiConfig{
			Temperature: 0.7,
		},
	}
	// Only request thinking tokens for models that actually support it — prevents 400 errors.
	if g.supportsThinking {
		reqBody.GenerationConfig.ThinkingConfig = &ThinkingConfig{
			IncludeThoughts: true,
		}
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

	resp, err := g.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var result GeminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return g.extractTextFromResponse(result), nil
}

func (g *GeminiProvider) Generate(ctx context.Context, prompt string) (string, error) {
	url := g.getURL(false)

	reqBody := GeminiRequest{
		Contents: []GeminiContent{
			{
				Parts: []GeminiPart{
					{Text: prompt},
				},
			},
		},
		GenerationConfig: GeminiConfig{
			Temperature: 0.7,
		},
	}
	// Only request thinking tokens for models that actually support it — prevents 400 errors.
	if g.supportsThinking {
		reqBody.GenerationConfig.ThinkingConfig = &ThinkingConfig{
			IncludeThoughts: true,
		}
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

	resp, err := g.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var result GeminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no content in response")
	}

	// Concatenate all parts (text)
	var sb strings.Builder
	for _, part := range result.Candidates[0].Content.Parts {
		if part.Thought {
			if g.VerboseThoughts {
				slog.Default().Debug("consultant_thought", "text", part.Text)
			}
			continue
		}
		sb.WriteString(part.Text)
	}

	return sb.String(), nil
}

func (g *GeminiProvider) Stream(ctx context.Context, prompt string, out io.Writer) error {
	// Use SSE mode for easier parsing of the stream
	url := g.getURL(true)

	reqBody := GeminiRequest{
		Contents: []GeminiContent{
			{
				Parts: []GeminiPart{
					{Text: prompt},
				},
			},
		},
		GenerationConfig: GeminiConfig{
			Temperature: 0.7,
		},
	}
	// Only request thinking tokens for models that actually support it — prevents 400 errors.
	if g.supportsThinking {
		reqBody.GenerationConfig.ThinkingConfig = &ThinkingConfig{
			IncludeThoughts: true,
		}
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

	resp, err := g.Client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	scanner := bufio.NewScanner(resp.Body)
	var thoughtBuffer strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		// Debug raw chunks as requested
		slog.Debug("Raw Chunk Received", "data", line)

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		
		var chunk GeminiStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if len(chunk.Candidates) > 0 {
			content := chunk.Candidates[0].Content
			for _, part := range content.Parts {
				// Thought Filtering Logic
				if part.Thought {
					if g.VerboseThoughts {
						// Bypass filter if verbose
						if part.Text != "" {
							fmt.Fprintf(out, "[THOUGHT]: %s", part.Text)
						}
					} else {
						// Buffer thoughts (for debugging or potential logging later)
						thoughtBuffer.WriteString(part.Text)
					}
				} else {
					// Standard Content
					if part.Text != "" {
						fmt.Fprint(out, part.Text)
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stream error: %w", err)
	}

	return nil
}

func (g *GeminiProvider) StreamWithHistory(ctx context.Context, history *ConversationHistory, out io.Writer) error {
	url := g.getURL(true)

	// Convert conversation history to Gemini contents
	contents := g.convertHistoryToContents(history)

	reqBody := GeminiRequest{
		Contents: contents,
		GenerationConfig: GeminiConfig{
			Temperature: 0.7,
		},
	}
	// Only request thinking tokens for models that actually support it — prevents 400 errors.
	if g.supportsThinking {
		reqBody.GenerationConfig.ThinkingConfig = &ThinkingConfig{
			IncludeThoughts: true,
		}
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

	resp, err := g.Client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	scanner := bufio.NewScanner(resp.Body)
	var thoughtBuffer strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		slog.Debug("Raw Chunk Received", "data", line)

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		var chunk GeminiStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if len(chunk.Candidates) > 0 {
			content := chunk.Candidates[0].Content
			for _, part := range content.Parts {
				if part.Thought {
					if g.VerboseThoughts {
						if part.Text != "" {
							fmt.Fprintf(out, "[THOUGHT]: %s", part.Text)
						}
					} else {
						thoughtBuffer.WriteString(part.Text)
					}
				} else {
					if part.Text != "" {
						fmt.Fprint(out, part.Text)
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stream error: %w", err)
	}

	return nil
}

// convertHistoryToContents converts ConversationHistory to Gemini contents
func (g *GeminiProvider) convertHistoryToContents(history *ConversationHistory) []GeminiContent {
	contents := []GeminiContent{}

	for _, msg := range history.GetMessages() {
		// Gemini uses "user" and "model" roles, not "assistant"
		role := msg.Role
		if role == "assistant" {
			role = "model"
		}

		// Skip system messages for now - Gemini handles them differently
		// You could prepend system messages to the first user message if needed
		if role == "system" {
			continue
		}

		contents = append(contents, GeminiContent{
			Role: role,
			Parts: []GeminiPart{
				{Text: msg.Content},
			},
		})
	}

	return contents
}

// extractTextFromResponse extracts text from Gemini response
func (g *GeminiProvider) extractTextFromResponse(result GeminiResponse) string {
	var output strings.Builder
	var thoughts strings.Builder

	for _, candidate := range result.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.Thought {
				if g.VerboseThoughts {
					thoughts.WriteString(fmt.Sprintf("[THOUGHT]: %s\n", part.Text))
				}
			} else {
				output.WriteString(part.Text)
			}
		}
	}

	if g.VerboseThoughts && thoughts.Len() > 0 {
		return thoughts.String() + "\n" + output.String()
	}

	return output.String()
}

// Ping validates the Gemini key with a single lightweight GET /v1beta/models request.
// Uses NewPingClient (5 s hard timeout, no retries) so it never blocks the UI.
func (g *GeminiProvider) Ping(ctx context.Context) error {
	url := "https://generativelanguage.googleapis.com/v1beta/models?key=" + g.APIKey + "&pageSize=1"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := NewPingClient().Do(req)
	if err != nil {
		return fmt.Errorf("Gemini unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Gemini key invalid (%d): %s", resp.StatusCode, string(body))
	}
	return nil
}
