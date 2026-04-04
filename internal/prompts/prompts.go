package prompts

import (
	"fmt"
	"log/slog"
	"sync"
)

// PromptManager manages prompt variants and selection
type PromptManager struct {
	variants    map[string]Variant
	cache       map[string]string
	optimizer   *TokenOptimizer
	logger      *slog.Logger
	mu          sync.RWMutex
	defaultVariant string
}

// Variant represents a prompt variant for a specific use case
type Variant interface {
	Name() string
	Build(ctx *PromptContext) string
	MaxTokens() int
	EstimateCost(tokens int, costPer1M float64) float64
	SupportsThinking() bool
	SupportsVision() bool
	Priority() int // Higher = preferred
}

// PromptContext provides context for prompt building
type PromptContext struct {
	Task            string
	SystemPrompt    string
	ConversationHistory []string
	Tools           []string
	Provider        string
	Model           string
	ThinkingBudget  int
	VisionSupported bool
	MaxTokens       int
	Metadata        map[string]interface{}
}

// NewPromptManager creates a new prompt manager
func NewPromptManager(logger *slog.Logger) *PromptManager {
	if logger == nil {
		logger = slog.Default()
	}

	pm := &PromptManager{
		variants:        make(map[string]Variant),
		cache:           make(map[string]string),
		optimizer:       NewTokenOptimizer(),
		logger:          logger,
		defaultVariant: "generic",
	}

	// Register all variants
	pm.registerVariants()

	return pm
}

// registerVariants registers all 8 prompt variants
func (pm *PromptManager) registerVariants() {
	pm.variants["generic"] = NewGenericVariant()
	pm.variants["nextgen"] = NewNextGenVariant()
	pm.variants["gpt5"] = NewGPT5Variant()
	pm.variants["gemini3"] = NewGemini3Variant()
	pm.variants["claude_thinking"] = NewClaudeThinkingVariant()
	pm.variants["xs"] = NewXSVariant()
	pm.variants["vision"] = NewVisionVariant()
	pm.variants["specialist"] = NewSpecialistVariant()

	pm.logger.Debug("registered 8 prompt variants")
}

// SelectVariant automatically selects the best variant for a provider
func (pm *PromptManager) SelectVariant(provider string, hasThinking bool, hasVision bool) (Variant, string) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// Priority: use provider-specific variant if available
	switch provider {
	case "anthropic":
		if hasThinking {
			return pm.variants["claude_thinking"], "claude_thinking"
		}
		return pm.variants["generic"], "generic"

	case "openai":
		return pm.variants["gpt5"], "gpt5"

	case "google":
		if hasVision {
			return pm.variants["vision"], "vision"
		}
		return pm.variants["gemini3"], "gemini3"

	case "bedrock":
		if hasThinking {
			return pm.variants["nextgen"], "nextgen"
		}
		return pm.variants["generic"], "generic"

	case "azure":
		return pm.variants["nextgen"], "nextgen"

	case "ollama":
		return pm.variants["xs"], "xs"

	default:
		return pm.variants["generic"], "generic"
	}
}

// Build builds a prompt using the specified variant
func (pm *PromptManager) Build(ctx *PromptContext, variantName string) (string, error) {
	pm.mu.RLock()
	variant, ok := pm.variants[variantName]
	pm.mu.RUnlock()

	if !ok {
		// Fallback to generic
		pm.logger.Warn("unknown variant, falling back to generic", slog.String("variant", variantName))
		variant = pm.variants["generic"]
	}

	// Check cache
	cacheKey := fmt.Sprintf("%s:%s:%s", variantName, ctx.Provider, ctx.Task[:min(50, len(ctx.Task))])
	pm.mu.RLock()
	if cached, ok := pm.cache[cacheKey]; ok {
		pm.mu.RUnlock()
		return cached, nil
	}
	pm.mu.RUnlock()

	// Build prompt
	prompt := variant.Build(ctx)

	// Cache result
	pm.mu.Lock()
	pm.cache[cacheKey] = prompt
	pm.mu.Unlock()

	pm.logger.Debug("built prompt",
		slog.String("variant", variantName),
		slog.String("provider", ctx.Provider),
		slog.Int("length", len(prompt)),
	)

	return prompt, nil
}

// EstimateCost estimates cost for a prompt
func (pm *PromptManager) EstimateCost(variant string, tokens int, costPer1M float64) float64 {
	pm.mu.RLock()
	v, ok := pm.variants[variant]
	pm.mu.RUnlock()

	if !ok {
		v = pm.variants["generic"]
	}

	return v.EstimateCost(tokens, costPer1M)
}

// ClearCache clears the prompt cache
func (pm *PromptManager) ClearCache() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.cache = make(map[string]string)
}

// GetStats returns cache statistics
func (pm *PromptManager) GetStats() map[string]interface{} {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	return map[string]interface{}{
		"cached_prompts": len(pm.cache),
		"variants":       len(pm.variants),
	}
}

// TokenOptimizer optimizes token usage
type TokenOptimizer struct {
	minSaveThreshold int
}

// NewTokenOptimizer creates a new token optimizer
func NewTokenOptimizer() *TokenOptimizer {
	return &TokenOptimizer{
		minSaveThreshold: 100, // Minimum tokens to save
	}
}

// CanOptimize returns true if optimization would save tokens
func (to *TokenOptimizer) CanOptimize(currentTokens int) bool {
	return currentTokens > 1000 // Only optimize longer prompts
}

// EstimateSavings estimates token savings with compression
func (to *TokenOptimizer) EstimateSavings(currentTokens int, strategy string) int {
	switch strategy {
	case "semantic":
		return int(float64(currentTokens) * 0.20) // 20% reduction
	case "selective":
		return int(float64(currentTokens) * 0.15) // 15% reduction
	case "aggressive":
		return int(float64(currentTokens) * 0.30) // 30% reduction
	default:
		return 0
	}
}

// Helper functions

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// VariantComparator compares variants by quality/cost
type VariantComparator struct {
	variants []Variant
}

// NewVariantComparator creates a comparator
func NewVariantComparator(variants []Variant) *VariantComparator {
	return &VariantComparator{variants: variants}
}

// FindBest finds the best variant for the context
func (vc *VariantComparator) FindBest(ctx *PromptContext) Variant {
	if len(vc.variants) == 0 {
		return nil
	}

	best := vc.variants[0]
	for _, v := range vc.variants[1:] {
		if vc.isBetter(v, best, ctx) {
			best = v
		}
	}
	return best
}

func (vc *VariantComparator) isBetter(a, b Variant, ctx *PromptContext) bool {
	// Prefer variants that support available features
	if ctx.ThinkingBudget > 0 && a.SupportsThinking() && !b.SupportsThinking() {
		return true
	}

	if ctx.VisionSupported && a.SupportsVision() && !b.SupportsVision() {
		return true
	}

	// Prefer higher priority
	return a.Priority() > b.Priority()
}

// PromptMetrics tracks prompt usage metrics
type PromptMetrics struct {
	variantUsage map[string]int
	totalBuilds  int
	cacheHits    int
	cacheMisses  int
	mu           sync.RWMutex
}

// NewPromptMetrics creates new metrics
func NewPromptMetrics() *PromptMetrics {
	return &PromptMetrics{
		variantUsage: make(map[string]int),
	}
}

// RecordBuild records a prompt build
func (pm *PromptMetrics) RecordBuild(variant string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.totalBuilds++
	pm.variantUsage[variant]++
}

// RecordCacheHit records a cache hit
func (pm *PromptMetrics) RecordCacheHit() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.cacheHits++
}

// RecordCacheMiss records a cache miss
func (pm *PromptMetrics) RecordCacheMiss() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.cacheMisses++
}

// GetStats returns metrics statistics
func (pm *PromptMetrics) GetStats() map[string]interface{} {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	totalCache := pm.cacheHits + pm.cacheMisses
	var hitRate float64
	if totalCache > 0 {
		hitRate = float64(pm.cacheHits) / float64(totalCache) * 100
	}

	return map[string]interface{}{
		"total_builds":  pm.totalBuilds,
		"cache_hits":    pm.cacheHits,
		"cache_misses":  pm.cacheMisses,
		"cache_hit_rate": hitRate,
		"variant_usage": pm.variantUsage,
	}
}
