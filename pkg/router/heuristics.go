package router

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/velariumai/gorkbot/pkg/registry"
)

// ModelTier represents the inferred performance class
type ModelTier int

const (
	TierUnknown   ModelTier = iota
	TierFast                // Low context, efficiency focused
	TierStandard            // Balanced
	TierReasoning           // High context, high version number
)

// TaskComplexity represents the estimated difficulty of a prompt
type TaskComplexity int

const (
	ComplexitySimple TaskComplexity = iota
	ComplexityStandard
	ComplexityAdvanced
)

// calculateDynamicScore returns a capability-based score (0-1000)
// Higher = Better/Smarter/Newer
func calculateDynamicScore(m registry.ModelDefinition) int {
	score := 0
	id := strings.ToLower(string(m.ID))

	// 1. Version Heuristic (Newer is usually better)
	// Extract "2.0", "3", "1.5" etc.
	// STRICTER REGEX: Look for version patterns like "v1", "1.5", "3.5"
	// Avoid matching "1206" (dates) or "001" (builds) as massive version numbers.
	re := regexp.MustCompile(`(?:v|-)?(\d+(\.\d+)?)`)
	matches := re.FindAllStringSubmatch(id, -1)

	maxVer := 0.0
	for _, m := range matches {
		if len(m) >= 2 {
			if ver, err := strconv.ParseFloat(m[1], 64); err == nil {
				// Sanity check: Real model versions are usually < 10.
				// Dates (2024, 1206) and Context sizes (128k) should be ignored here.
				if ver > 0 && ver < 10 {
					if ver > maxVer {
						maxVer = ver
					}
				}
			}
		}
	}

	if maxVer > 0 {
		// Grok 3 (150pts) vs Gemini 1.5 (75pts)
		score += int(maxVer * 50)
	}

	// 2. Context Window Proxy (Capacity = Power)
	// > 1M tokens = Massive Power (Gemini Pro/Flash)
	// > 100k = High Power (Grok-3)
	// < 8k = Legacy
	if m.Capabilities.MaxContextTokens >= 1000000 {
		score += 300
	} else if m.Capabilities.MaxContextTokens >= 100000 {
		score += 200
	} else if m.Capabilities.MaxContextTokens >= 32000 {
		score += 100
	} else if m.Capabilities.MaxContextTokens < 8192 {
		score -= 100 // Penalize tiny context legacy models
	}

	// 3. Keyword Signals (Capability markers)
	if strings.Contains(id, "pro") || strings.Contains(id, "ultra") || strings.Contains(id, "opus") {
		score += 150
	}
	if strings.Contains(id, "reasoning") || strings.Contains(id, "thinking") {
		score += 200
	}
	if strings.Contains(id, "flash") || strings.Contains(id, "fast") {
		// Fast models are good, but "lighter" than Pro.
		// We don't penalize much, but Pro beats Flash.
		score += 50
	}
	if strings.Contains(id, "mini") || strings.Contains(id, "nano") || strings.Contains(id, "micro") {
		score -= 100 // Explicitly lightweight
	}

	// 4. Vision/Tools Bonus
	if m.Capabilities.SupportsVision {
		score += 20
	}
	if m.Capabilities.SupportsTools {
		score += 20
	}

	return score
}

// classifyModelTier dynamically assigns a tier based on score
func classifyModelTier(m registry.ModelDefinition) ModelTier {
	score := calculateDynamicScore(m)

	// These thresholds need to be relative to the ecosystem
	if score > 400 {
		return TierReasoning
	}
	if score > 200 {
		return TierStandard
	}
	return TierFast
}

// analyzePromptComplexity estimates task difficulty based on heuristics
func analyzePromptComplexity(prompt string) TaskComplexity {
	p := strings.ToLower(prompt)
	length := len(p)

	// Length Heuristic
	if length < 50 {
		return ComplexitySimple
	}
	if length > 2000 {
		return ComplexityAdvanced
	}

	// Keyword Heuristics - Reasoning Indicators
	reasoningKeywords := []string{
		"architect", "design", "refactor", "debug", "analyze",
		"explain why", "compare", "strategy", "plan", "complex",
		"algorithm", "security", "optimize", "synthesis",
	}

	// Keyword Heuristics - Simple Indicators
	simpleKeywords := []string{
		"hello", "hi", "thanks", "list", "what is", "define",
		"translate", "correct", "fix typo",
	}

	score := 0

	for _, kw := range reasoningKeywords {
		if strings.Contains(p, kw) {
			score += 2
		}
	}

	for _, kw := range simpleKeywords {
		if strings.Contains(p, kw) {
			score -= 1
		}
	}

	if score >= 4 {
		return ComplexityAdvanced
	}
	if score <= 0 && length < 200 {
		return ComplexitySimple
	}

	return ComplexityStandard
}
