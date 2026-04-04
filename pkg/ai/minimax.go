package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/registry"
)

// MiniMaxProvider wraps AnthropicProvider with the MiniMax Anthropic-compat endpoint.
// MiniMax exposes an Anthropic-compatible API at https://api.minimax.io/anthropic/v1
// and an OpenAI-compatible listing endpoint at https://api.minimax.io/v1/models.
type MiniMaxProvider struct {
	inner *AnthropicProvider
}

const minimaxAnthropicBase = "https://api.minimax.io/anthropic/v1"
const minimaxModelsURL = "https://api.minimax.io/v1/models"

// minimaxFallbackModels is the static fallback list when the listing endpoint fails.
var minimaxFallbackModels = []struct {
	id   string
	name string
}{
	{"MiniMax-M1", "MiniMax M1"},
	{"MiniMax-M2", "MiniMax M2"},
	{"MiniMax-M2.1", "MiniMax M2.1"},
	{"MiniMax-M2.5", "MiniMax M2.5"},
}

// NewMiniMaxProvider creates a new MiniMax provider.
func NewMiniMaxProvider(apiKey, model string) *MiniMaxProvider {
	if model == "" {
		model = "MiniMax-M1"
	}
	inner := &AnthropicProvider{
		APIKey:           apiKey,
		BaseURL:          minimaxAnthropicBase,
		Model:            model,
		client:           NewRetryClient(),
		supportsThinking: minimaxModelSupportsThinking(model),
		bearerAuth:       true, // MiniMax Anthropic-compat endpoint uses Bearer auth
	}
	return &MiniMaxProvider{inner: inner}
}

func (m *MiniMaxProvider) Name() string { return "MiniMax" }

func (m *MiniMaxProvider) ID() registry.ProviderID { return "minimax" }

func (m *MiniMaxProvider) GetMetadata() ProviderMetadata {
	return ProviderMetadata{
		ID:          m.inner.Model,
		Name:        "MiniMax (" + m.inner.Model + ")",
		Description: "MiniMax's powerful language models with extended thinking support.",
		ContextSize: 1000000,
	}
}

func (m *MiniMaxProvider) WithModel(model string) AIProvider {
	newInner := &AnthropicProvider{
		APIKey:           m.inner.APIKey,
		BaseURL:          minimaxAnthropicBase,
		Model:            model,
		client:           m.inner.client,
		supportsThinking: minimaxModelSupportsThinking(model),
		ThinkingBudget:   m.inner.ThinkingBudget, // PRESERVE thinking budget
		bearerAuth:       true,
	}
	return &MiniMaxProvider{inner: newInner}
}

// Ping validates the MiniMax key using the OpenAI-compat models endpoint.
// Uses NewPingClient (5 s hard timeout, no retries) so it never blocks the UI.
// MiniMax's Anthropic-compat path (/anthropic/v1) does NOT expose /models,
// so we must NOT delegate to the inner AnthropicProvider.Ping().
func (m *MiniMaxProvider) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", minimaxModelsURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.inner.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := NewPingClient().Do(req)
	if err != nil {
		return fmt.Errorf("MiniMax unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("MiniMax key invalid (%d): %s", resp.StatusCode, string(body))
	}
	// 404 on /v1/models is acceptable for some account tiers — auth is valid.
	return nil
}

// FetchModels returns the live model list from MiniMax, falling back to safe statics on failure.
func (m *MiniMaxProvider) FetchModels(ctx context.Context) ([]registry.ModelDefinition, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}
	models, err := FetchMiniMaxModels(ctx, m.inner.APIKey)
	if err != nil || len(models) == 0 {
		return SafeModelDefs("minimax"), nil
	}
	return models, nil
}

func (m *MiniMaxProvider) Generate(ctx context.Context, prompt string) (string, error) {
	return m.inner.Generate(ctx, prompt)
}

func (m *MiniMaxProvider) GenerateWithHistory(ctx context.Context, history *ConversationHistory) (string, error) {
	return m.inner.GenerateWithHistory(ctx, history)
}

func (m *MiniMaxProvider) Stream(ctx context.Context, prompt string, out io.Writer) error {
	return m.inner.Stream(ctx, prompt, out)
}

func (m *MiniMaxProvider) StreamWithHistory(ctx context.Context, history *ConversationHistory, out io.Writer) error {
	return m.inner.StreamWithHistory(ctx, history, out)
}

// SetThinkingBudget implements ThinkingBudgetProvider.
func (m *MiniMaxProvider) SetThinkingBudget(budget int) {
	m.inner.SetThinkingBudget(budget)
}

// GetThinkingBudget implements ThinkingBudgetProvider.
func (m *MiniMaxProvider) GetThinkingBudget() int {
	return m.inner.GetThinkingBudget()
}

// minimaxModelSupportsThinking returns true for MiniMax models that support extended thinking.
func minimaxModelSupportsThinking(modelID string) bool {
	lower := strings.ToLower(modelID)
	return strings.Contains(lower, "m2.5") ||
		strings.Contains(lower, "m2.1") ||
		strings.Contains(lower, "m2") ||
		strings.Contains(lower, "m1")
}

// FetchMiniMaxModels retrieves the model list from the MiniMax OpenAI-compat listing endpoint.
// Falls back to a static known-good list if the endpoint fails or returns no results.
func FetchMiniMaxModels(ctx context.Context, apiKey string) ([]registry.ModelDefinition, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", minimaxModelsURL, nil)
	if err != nil {
		return fallbackMiniMaxModels(), nil
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	client := NewRetryClient()
	resp, err := client.Do(req)
	if err != nil {
		return fallbackMiniMaxModels(), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fallbackMiniMaxModels(), nil
	}

	// Try OpenAI list format first
	type openAIModel struct {
		ID string `json:"id"`
	}
	type openAIList struct {
		Data []openAIModel `json:"data"`
	}
	var list openAIList
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &list); err == nil && len(list.Data) > 0 {
		var models []registry.ModelDefinition
		for _, m := range list.Data {
			models = append(models, registry.ModelDefinition{
				ID:       registry.ModelID(m.ID),
				Provider: "minimax",
				Name:     m.ID,
				Capabilities: registry.CapabilitySet{
					MaxContextTokens:  1000000,
					SupportsStreaming: true,
					SupportsTools:     true,
					SupportsThinking:  minimaxModelSupportsThinking(m.ID),
				},
				Status:      registry.StatusActive,
				LastUpdated: time.Now(),
			})
		}
		return models, nil
	}

	return fallbackMiniMaxModels(), nil
}

func fallbackMiniMaxModels() []registry.ModelDefinition {
	var models []registry.ModelDefinition
	for _, m := range minimaxFallbackModels {
		models = append(models, registry.ModelDefinition{
			ID:       registry.ModelID(m.id),
			Provider: "minimax",
			Name:     m.name,
			Capabilities: registry.CapabilitySet{
				MaxContextTokens:  1000000,
				SupportsStreaming: true,
				SupportsTools:     true,
				SupportsThinking:  minimaxModelSupportsThinking(m.id),
			},
			Status:      registry.StatusActive,
			LastUpdated: time.Now(),
		})
	}
	return models
}

// minimaxAnthropicVersion returns the Anthropic version header required by MiniMax.
func minimaxAnthropicVersion() string {
	return fmt.Sprintf("minimax-anthropic/%s", time.Now().Format("2006-01-02"))
}
