package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	xaiModelsURL       = "https://api.x.ai/v1/models"
	geminiModelsURL    = "https://generativelanguage.googleapis.com/v1beta/models"
	anthropicModelsURL = "https://api.anthropic.com/v1/models"
	openaiModelsURL    = "https://api.openai.com/v1/models"
	minimaxModelsURL    = "https://api.minimax.io/v1/models"
	openrouterModelsURL = "https://openrouter.ai/api/v1/models"

	pollInterval = 30 * time.Minute
	httpTimeout  = 15 * time.Second
)

// KeyGetter is satisfied by any struct that can return API keys by provider ID.
// Using an interface avoids an import cycle (discovery ← providers would be circular).
// The providers.KeyStore implements this via its GetKey method.
type KeyGetter interface {
	GetKey(provider string) string
}

// Manager polls all 5 AI providers for live model lists every 30 minutes.
type Manager struct {
	// legacy direct keys (set when constructed without KeyGetter)
	xaiKey     string
	geminiKey  string
	anthropicKey string
	openaiKey  string
	minimaxKey    string
	openrouterKey string

	// optional live key source (preferred; overrides direct keys if non-nil)
	keyGetter KeyGetter

	mu          sync.RWMutex
	models      []DiscoveredModel
	subMu       sync.Mutex
	subscribers []chan []DiscoveredModel
	agentsMu    sync.RWMutex
	agents      []*AgentNode
	stopCh      chan struct{}
	logger      *slog.Logger
}

// NewManager creates a manager using the given API keys (legacy constructor).
func NewManager(xaiKey, geminiKey string, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		xaiKey:    xaiKey,
		geminiKey: geminiKey,
		stopCh:    make(chan struct{}),
		logger:    logger,
	}
}

// NewManagerWithKeys creates a manager that reads keys from a KeyGetter
// (e.g. providers.KeyStore).  All 5 providers are polled on each cycle.
func NewManagerWithKeys(kg KeyGetter, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	m := &Manager{
		keyGetter: kg,
		stopCh:    make(chan struct{}),
		logger:    logger,
	}
	return m
}

// Start begins polling (initial poll immediately, then every 30 min). Non-blocking.
func (dm *Manager) Start(ctx context.Context) {
	go func() {
		dm.poll(ctx)
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				dm.poll(ctx)
			case <-dm.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Stop signals the polling goroutine to exit.
func (dm *Manager) Stop() {
	select {
	case <-dm.stopCh:
	default:
		close(dm.stopCh)
	}
}

// Subscribe returns a buffered channel that receives the full model list on each update.
func (dm *Manager) Subscribe() chan []DiscoveredModel {
	ch := make(chan []DiscoveredModel, 1)
	dm.subMu.Lock()
	dm.subscribers = append(dm.subscribers, ch)
	dm.subMu.Unlock()
	return ch
}

// Models returns a snapshot of the currently discovered models.
func (dm *Manager) Models() []DiscoveredModel {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	cp := make([]DiscoveredModel, len(dm.models))
	copy(cp, dm.models)
	return cp
}

// BestForCap returns the best discovered model for a capability class.
// Pass preferProvider to bias selection; "" means any.
func (dm *Manager) BestForCap(cap CapabilityClass, preferProvider string) *DiscoveredModel {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	var best *DiscoveredModel
	for i := range dm.models {
		m := &dm.models[i]
		if !m.HasCap(cap) {
			continue
		}
		if preferProvider != "" && m.Provider != preferProvider {
			continue
		}
		if best == nil || (m.BestCap() == cap && best.BestCap() != cap) {
			best = m
		}
	}
	if best == nil && preferProvider != "" {
		return dm.BestForCap(cap, "")
	}
	return best
}

// RegisterAgent adds a root-level agent node to the live tree.
func (dm *Manager) RegisterAgent(node *AgentNode) {
	dm.agentsMu.Lock()
	defer dm.agentsMu.Unlock()
	dm.agents = append(dm.agents, node)
}

// UpdateAgent sets the status of an agent node (searched recursively by ID).
func (dm *Manager) UpdateAgent(id, status string) {
	dm.agentsMu.Lock()
	defer dm.agentsMu.Unlock()
	updateNode(dm.agents, id, status)
}

func updateNode(nodes []*AgentNode, id, status string) bool {
	for _, n := range nodes {
		if n.ID == id {
			n.Status = status
			return true
		}
		if updateNode(n.Children, id, status) {
			return true
		}
	}
	return false
}

// AgentTree returns a snapshot of root-level agent nodes.
func (dm *Manager) AgentTree() []*AgentNode {
	dm.agentsMu.RLock()
	defer dm.agentsMu.RUnlock()
	cp := make([]*AgentNode, len(dm.agents))
	copy(cp, dm.agents)
	return cp
}

// ─── key resolution ───────────────────────────────────────────────────────────

func (dm *Manager) getKey(provider string) string {
	if dm.keyGetter != nil {
		return dm.keyGetter.GetKey(provider)
	}
	switch provider {
	case ProviderXAI:
		return dm.xaiKey
	case ProviderGoogle:
		return dm.geminiKey
	case ProviderAnthropic:
		return dm.anthropicKey
	case ProviderOpenAI:
		return dm.openaiKey
	case ProviderMiniMax:
		return dm.minimaxKey
	case ProviderOpenRouter:
		return dm.openrouterKey
	}
	return ""
}

// ─── polling ─────────────────────────────────────────────────────────────────

func (dm *Manager) poll(ctx context.Context) {
	client := &http.Client{Timeout: httpTimeout}
	var discovered []DiscoveredModel
	now := time.Now()

	// xAI
	if key := dm.getKey(ProviderXAI); key != "" {
		if models, err := fetchXAIModels(ctx, client, key); err != nil {
			dm.logger.Warn("discovery: xAI poll failed", "error", err)
		} else {
			for _, m := range models {
				discovered = append(discovered, classifyModel(m.ID, m.ID, ProviderXAI, now))
			}
		}
	}

	// Google / Gemini
	if key := dm.getKey(ProviderGoogle); key != "" {
		if models, err := fetchGeminiModels(ctx, client, key); err != nil {
			dm.logger.Warn("discovery: Gemini poll failed", "error", err)
		} else {
			for _, m := range models {
				name := strings.TrimPrefix(m.Name, "models/")
				displayName := m.DisplayName
				if displayName == "" {
					displayName = name
				}
				discovered = append(discovered, classifyModel(name, displayName, ProviderGoogle, now))
			}
		}
	}

	// Anthropic
	if key := dm.getKey(ProviderAnthropic); key != "" {
		if models, err := fetchAnthropicModels(ctx, client, key); err != nil {
			dm.logger.Warn("discovery: Anthropic poll failed", "error", err)
		} else {
			for _, m := range models {
				name := m.ID
				disp := m.DisplayName
				if disp == "" {
					disp = name
				}
				discovered = append(discovered, classifyModel(name, disp, ProviderAnthropic, now))
			}
		}
	}

	// OpenAI
	if key := dm.getKey(ProviderOpenAI); key != "" {
		if models, err := fetchOpenAIModels(ctx, client, key, openaiModelsURL); err != nil {
			dm.logger.Warn("discovery: OpenAI poll failed", "error", err)
		} else {
			for _, m := range models {
				if isChatModel(m.ID) {
					discovered = append(discovered, classifyModel(m.ID, m.ID, ProviderOpenAI, now))
				}
			}
		}
	}

	// MiniMax
	if key := dm.getKey(ProviderMiniMax); key != "" {
		if models, err := fetchOpenAIModels(ctx, client, key, minimaxModelsURL); err != nil {
			dm.logger.Warn("discovery: MiniMax poll failed (using fallback)", "error", err)
			// Inject static fallback
			for _, name := range []string{"MiniMax-M1", "MiniMax-M2", "MiniMax-M2.1", "MiniMax-M2.5"} {
				discovered = append(discovered, classifyModel(name, name, ProviderMiniMax, now))
			}
		} else {
			for _, m := range models {
				discovered = append(discovered, classifyModel(m.ID, m.ID, ProviderMiniMax, now))
			}
		}
	}

	// OpenRouter
	if key := dm.getKey(ProviderOpenRouter); key != "" {
		if models, err := fetchOpenRouterDiscoveryModels(ctx, client, key); err != nil {
			dm.logger.Warn("discovery: OpenRouter poll failed", "error", err)
		} else {
			for _, m := range models {
				if m.ContextLength >= 4096 {
					discovered = append(discovered, classifyModel(m.ID, m.ID, ProviderOpenRouter, now))
				}
			}
		}
	}

	if len(discovered) == 0 {
		return
	}

	dm.mu.Lock()
	dm.models = discovered
	dm.mu.Unlock()

	dm.notify(discovered)
	dm.logger.Info("discovery: poll complete", "count", len(discovered))
}

func (dm *Manager) notify(models []DiscoveredModel) {
	dm.subMu.Lock()
	defer dm.subMu.Unlock()
	cp := make([]DiscoveredModel, len(models))
	copy(cp, models)
	for _, ch := range dm.subscribers {
		select {
		case ch <- cp:
		default:
		}
	}
}

// ─── capability classification ────────────────────────────────────────────────

func classifyModel(id, displayName, provider string, ts time.Time) DiscoveredModel {
	lower := strings.ToLower(id)
	var caps []CapabilityClass

	if strings.Contains(lower, "reasoning") || strings.Contains(lower, "thinking") ||
		strings.Contains(lower, "pro") || strings.Contains(lower, "opus") ||
		strings.Contains(lower, "o1") || strings.Contains(lower, "o3") || strings.Contains(lower, "o4") {
		caps = append(caps, CapReasoning)
	}
	if strings.Contains(lower, "fast") || strings.Contains(lower, "flash") ||
		strings.Contains(lower, "lite") || strings.Contains(lower, "haiku") ||
		strings.Contains(lower, "mini") {
		caps = append(caps, CapSpeed)
	}
	if strings.Contains(lower, "code") || strings.Contains(lower, "vision") ||
		strings.Contains(lower, "gpt-4o") {
		caps = append(caps, CapCoding)
	}
	if len(caps) == 0 {
		caps = []CapabilityClass{CapGeneral}
	}

	return DiscoveredModel{
		ID:           id,
		Name:         displayName,
		Provider:     provider,
		Caps:         caps,
		DiscoveredAt: ts,
	}
}

// isChatModel returns true for OpenAI chat-capable models.
func isChatModel(id string) bool {
	lower := strings.ToLower(id)
	for _, p := range []string{"text-embedding", "dall-e", "whisper", "tts", "babbage", "davinci"} {
		if strings.HasPrefix(lower, p) {
			return false
		}
	}
	return strings.HasPrefix(lower, "gpt-") ||
		strings.HasPrefix(lower, "o1") ||
		strings.HasPrefix(lower, "o3") ||
		strings.HasPrefix(lower, "o4") ||
		strings.HasPrefix(lower, "chatgpt")
}

// ─── xAI API ──────────────────────────────────────────────────────────────────

type xaiModel struct {
	ID string `json:"id"`
}

type xaiModelsResponse struct {
	Data []xaiModel `json:"data"`
}

func fetchXAIModels(ctx context.Context, client *http.Client, apiKey string) ([]xaiModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, xaiModelsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("xAI models API returned %d", resp.StatusCode)
	}

	var data xaiModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data.Data, nil
}

// ─── Gemini API ───────────────────────────────────────────────────────────────

type geminiModel struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
}

type geminiModelsResponse struct {
	Models []geminiModel `json:"models"`
}

func fetchGeminiModels(ctx context.Context, client *http.Client, apiKey string) ([]geminiModel, error) {
	url := fmt.Sprintf("%s?key=%s", geminiModelsURL, apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Gemini models API returned %d", resp.StatusCode)
	}

	var data geminiModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data.Models, nil
}

// ─── Anthropic API ────────────────────────────────────────────────────────────

type anthropicModelEntry struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

type anthropicModelsResponse struct {
	Data []anthropicModelEntry `json:"data"`
}

func fetchAnthropicModels(ctx context.Context, client *http.Client, apiKey string) ([]anthropicModelEntry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, anthropicModelsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Anthropic models API returned %d", resp.StatusCode)
	}

	var data anthropicModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data.Data, nil
}

// ─── OpenAI-compat API ────────────────────────────────────────────────────────

type openAIModel struct {
	ID string `json:"id"`
}

type openAIModelsResponse struct {
	Data []openAIModel `json:"data"`
}

func fetchOpenAIModels(ctx context.Context, client *http.Client, apiKey, url string) ([]openAIModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models API returned %d", resp.StatusCode)
	}

	var data openAIModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data.Data, nil
}

// ─── OpenRouter API ───────────────────────────────────────────────────────────

type openRouterDiscoveryModel struct {
	ID            string `json:"id"`
	ContextLength int    `json:"context_length"`
}

type openRouterDiscoveryResponse struct {
	Data []openRouterDiscoveryModel `json:"data"`
}

func fetchOpenRouterDiscoveryModels(ctx context.Context, client *http.Client, apiKey string) ([]openRouterDiscoveryModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openrouterModelsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("HTTP-Referer", "https://gorkbot.ai")
	req.Header.Set("X-Title", "Gorkbot")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenRouter models API returned %d", resp.StatusCode)
	}

	var data openRouterDiscoveryResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data.Data, nil
}
