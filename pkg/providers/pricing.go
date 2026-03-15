package providers

// pricing.go — Static model pricing table (USD per 1M tokens).
// Updated periodically; used by cost-aware routing to prefer cheaper models
// when ARC classifies a task as CostTierCheap.

// ModelPrice holds per-million-token pricing for a model.
type ModelPrice struct {
	InputPerMTok  float64 // USD per 1M input tokens
	OutputPerMTok float64 // USD per 1M output tokens
}

// ModelPricing maps model IDs to their pricing.
// Use lowercase model ID prefixes for prefix matching via BestPriceFor().
var ModelPricing = map[string]ModelPrice{
	// xAI Grok
	"grok-3-mini":      {InputPerMTok: 0.30, OutputPerMTok: 0.50},
	"grok-3-mini-fast": {InputPerMTok: 0.20, OutputPerMTok: 0.40},
	"grok-3":           {InputPerMTok: 3.00, OutputPerMTok: 15.00},
	"grok-3-fast":      {InputPerMTok: 5.00, OutputPerMTok: 25.00},
	"grok-2":           {InputPerMTok: 2.00, OutputPerMTok: 10.00},
	"grok-2-mini":      {InputPerMTok: 0.20, OutputPerMTok: 0.40},

	// Google Gemini
	"gemini-2.0-flash":      {InputPerMTok: 0.10, OutputPerMTok: 0.40},
	"gemini-2.0-flash-lite": {InputPerMTok: 0.075, OutputPerMTok: 0.30},
	"gemini-2.0-pro":        {InputPerMTok: 1.25, OutputPerMTok: 10.00},
	"gemini-1.5-flash":      {InputPerMTok: 0.075, OutputPerMTok: 0.30},
	"gemini-1.5-flash-8b":   {InputPerMTok: 0.0375, OutputPerMTok: 0.15},
	"gemini-1.5-pro":        {InputPerMTok: 1.25, OutputPerMTok: 5.00},

	// Anthropic Claude
	"claude-haiku-4-5":  {InputPerMTok: 0.80, OutputPerMTok: 4.00},
	"claude-sonnet-4-6": {InputPerMTok: 3.00, OutputPerMTok: 15.00},
	"claude-opus-4-6":   {InputPerMTok: 15.00, OutputPerMTok: 75.00},
	"claude-3-5-haiku":  {InputPerMTok: 0.80, OutputPerMTok: 4.00},
	"claude-3-5-sonnet": {InputPerMTok: 3.00, OutputPerMTok: 15.00},
	"claude-3-7-sonnet": {InputPerMTok: 3.00, OutputPerMTok: 15.00},
	"claude-3-opus":     {InputPerMTok: 15.00, OutputPerMTok: 75.00},

	// OpenAI
	"gpt-4o-mini": {InputPerMTok: 0.15, OutputPerMTok: 0.60},
	"gpt-4o":      {InputPerMTok: 2.50, OutputPerMTok: 10.00},
	"gpt-4-turbo": {InputPerMTok: 10.00, OutputPerMTok: 30.00},
	"o4-mini":     {InputPerMTok: 1.10, OutputPerMTok: 4.40},
	"o3-mini":     {InputPerMTok: 1.10, OutputPerMTok: 4.40},
	"o3":          {InputPerMTok: 10.00, OutputPerMTok: 40.00},

	// MiniMax
	"minimax-text-01": {InputPerMTok: 0.20, OutputPerMTok: 1.10},
	"abab6.5s-chat":   {InputPerMTok: 0.10, OutputPerMTok: 0.10},
}

// GetPrice returns pricing for a model ID. Tries exact match first,
// then prefix match (longest prefix wins).
func GetPrice(modelID string) (ModelPrice, bool) {
	if p, ok := ModelPricing[modelID]; ok {
		return p, true
	}
	// Prefix match — find the longest matching prefix key
	best := ""
	var bestPrice ModelPrice
	for k, v := range ModelPricing {
		if len(k) > len(best) && len(modelID) >= len(k) && modelID[:len(k)] == k {
			best = k
			bestPrice = v
		}
	}
	if best != "" {
		return bestPrice, true
	}
	return ModelPrice{}, false
}

// CostPerRequest estimates the cost in USD for a request given token counts.
func CostPerRequest(modelID string, inputTokens, outputTokens int) float64 {
	p, ok := GetPrice(modelID)
	if !ok {
		return 0
	}
	return float64(inputTokens)*p.InputPerMTok/1e6 + float64(outputTokens)*p.OutputPerMTok/1e6
}

// IsCheapModel returns true if the model is in the "cheap" tier
// (input cost <= $0.30/MTok).
func IsCheapModel(modelID string) bool {
	p, ok := GetPrice(modelID)
	if !ok {
		return false
	}
	return p.InputPerMTok <= 0.30
}
