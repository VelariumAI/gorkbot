package cache

import (
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/velariumai/gorkbot/pkg/ai"
)

// Strategy is the caching approach used for a specific provider.
type Strategy int

const (
	StrategyNone                Strategy = iota
	StrategyAnthropicBreakpoints          // Anthropic, MiniMax, OpenRouter → cache_control blocks
	StrategyGeminiContext                  // Gemini → cachedContents REST lifecycle
	StrategyGrokAutomatic                  // xAI/Grok → automatic + x-grok-conv-id header
	StrategyOpenAIAutomatic                // OpenAI → stable-prefix structural optimisation
	StrategyMoonshotBestEffort             // Moonshot → graceful no-op until API confirmed
	StrategyApplicationLayer               // Universal → local TTL response cache
)

// CacheHints carries the cache instructions the orchestrator applies before
// each LLM call. Fields are zero-valued for strategies that don't use them.
type CacheHints struct {
	// AnthropicBreakpoints — indices in the messages slice that should receive
	// cache_control: {type: ephemeral}. Nil means no breakpoints.
	AnthropicBreakpoints []int

	// GeminiCachedContentName is the "cachedContents/{id}" resource name to
	// reference in the generateContent request. Empty = no cache reference.
	GeminiCachedContentName string

	// GrokConvID is the uuid4 value for the x-grok-conv-id header. Empty =
	// omit the header (cache still works automatically; this maximises hits).
	GrokConvID string

	// SystemPromptChanged is true when the ContentHash of the current system
	// prompt differs from the last observed hash. Providers use this to
	// invalidate and recreate explicit cache entries.
	SystemPromptChanged bool

	// AppCacheHit is true when the local application-layer cache has a valid
	// response for this (provider, model, system_hash) tuple.
	AppCacheHit bool
	// AppCachedResponse is the cached text when AppCacheHit is true.
	AppCachedResponse string
}

// Advisor computes and tracks the correct caching strategy for each provider
// in a session. It is safe for concurrent use.
type Advisor struct {
	mu sync.Mutex

	sessionKey     *SessionKey
	grokConvID     string // stable uuid4 for x-grok-conv-id
	lastSysHash    string // last observed system prompt hash
	geminiCache    *GeminiCacheClient
	appCache       *AppCache
	moonshotClient *MoonshotCacheClient

	// strategyFor maps provider ID → Strategy (derived once, cached).
	strategyFor map[string]Strategy
}

// NewAdvisor creates a session-scoped Advisor. geminiKey is the Gemini API
// key (may be empty — Gemini caching is skipped if so). configDir is used by
// the application-layer cache for its on-disk store.
func NewAdvisor(geminiKey, geminiModel, configDir string) (*Advisor, error) {
	sk, err := NewSessionKey()
	if err != nil {
		return nil, err
	}

	var gc *GeminiCacheClient
	if geminiKey != "" && geminiModel != "" {
		gc = NewGeminiCacheClient(geminiKey, geminiModel)
	}

	return &Advisor{
		sessionKey:     sk,
		grokConvID:     uuid.New().String(),
		geminiCache:    gc,
		appCache:       NewAppCache(configDir),
		moonshotClient: &MoonshotCacheClient{},
		strategyFor:    make(map[string]Strategy),
	}, nil
}

// Advise returns CacheHints for the given provider and current conversation.
// msgs is the snapshot of conversation messages at the time of the call.
// systemPrompt is the full composed system prompt string for this turn.
func (a *Advisor) Advise(providerID, model, systemPrompt string, msgs []ai.ConversationMessage) CacheHints {
	a.mu.Lock()
	defer a.mu.Unlock()

	strategy := a.resolveStrategy(providerID)
	sysHash := ContentHash([]byte(systemPrompt))
	changed := sysHash != a.lastSysHash
	if changed {
		a.lastSysHash = sysHash
	}

	hints := CacheHints{SystemPromptChanged: changed}

	switch strategy {
	case StrategyAnthropicBreakpoints:
		hints.AnthropicBreakpoints = anthropicBreakpoints(model, systemPrompt, msgs)

	case StrategyGeminiContext:
		if a.geminiCache != nil && changed {
			// Invalidate old cache on system-prompt change; new one created lazily.
			a.geminiCache.Invalidate()
		}
		if a.geminiCache != nil {
			hints.GeminiCachedContentName = a.geminiCache.CurrentName()
		}

	case StrategyGrokAutomatic:
		hints.GrokConvID = a.grokConvID

	case StrategyOpenAIAutomatic:
		// No hints needed; structural optimisation is handled in streaming.go
		// by ensuring the system prompt is always the first message.

	case StrategyMoonshotBestEffort:
		// Best-effort: no-op until Moonshot's upload/tag API is confirmed.

	case StrategyApplicationLayer:
		key := appCacheKey(providerID, model, sysHash)
		if resp, ok := a.appCache.Get(key); ok {
			hints.AppCacheHit = true
			hints.AppCachedResponse = resp
		}
	}

	return hints
}

// RecordGeminiCacheName persists the cachedContents name returned by Gemini
// after a successful cache creation so subsequent turns can reference it.
func (a *Advisor) RecordGeminiCacheName(name string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.geminiCache != nil {
		a.geminiCache.SetName(name)
	}
}

// GeminiCacheClient returns the Gemini cache client for direct lifecycle
// management (create / refresh / delete) by the engine.
func (a *Advisor) GeminiCacheClient() *GeminiCacheClient {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.geminiCache
}

// StoreAppCacheResponse records a successful response for app-layer caching.
func (a *Advisor) StoreAppCacheResponse(providerID, model, systemPrompt, response string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	key := appCacheKey(providerID, model, ContentHash([]byte(systemPrompt)))
	a.appCache.Set(key, response)
}

// resolveStrategy derives and caches the Strategy for a provider ID.
// Must be called with a.mu held.
func (a *Advisor) resolveStrategy(providerID string) Strategy {
	if s, ok := a.strategyFor[providerID]; ok {
		return s
	}
	s := strategyForProvider(providerID)
	a.strategyFor[providerID] = s
	return s
}

// strategyForProvider maps a provider ID string to its caching strategy.
func strategyForProvider(providerID string) Strategy {
	switch strings.ToLower(providerID) {
	case "anthropic", "minimax", "openrouter":
		return StrategyAnthropicBreakpoints
	case "gemini", "google":
		return StrategyGeminiContext
	case "grok", "xai":
		return StrategyGrokAutomatic
	case "openai":
		return StrategyOpenAIAutomatic
	case "moonshot":
		return StrategyMoonshotBestEffort
	default:
		return StrategyApplicationLayer
	}
}

// appCacheKey builds a stable cache key for the application-layer store.
func appCacheKey(providerID, model, sysHash string) string {
	return providerID + ":" + model + ":" + sysHash
}
