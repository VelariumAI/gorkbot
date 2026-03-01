package sense

// lie.go — Length-Incentivized Exploration (LIE)
//
// LIE is the SENSE reward mechanism that encourages the agent to produce deep,
// diverse reasoning trajectories rather than short or repetitive outputs.
//
// For Gorkbot, LIE operates at inference time (not training time) by:
//   1. Evaluating each AI response against the recent trajectory.
//   2. Generating a feedback signal (reward/penalty narrative).
//   3. Injecting that signal as a SYSTEM message into the conversation history
//      so the model can self-correct on the next turn.
//
// This mirrors the SENSE GRPO reward function but adapted to prompt engineering
// rather than weight updates (since cloud model weights are not adjustable).

import (
	"fmt"
	"math"
	"strings"
)

// RewardMetrics holds the LIE evaluation for one AI response.
type RewardMetrics struct {
	LengthScore    float64 // 0–1: rewards comprehensive responses
	RedundancyPenalty float64 // 0–1: penalises repeated content
	StructureBonus float64 // 0–1: rewards structured reasoning (reasoning + answer)
	FinalReward    float64 // combined score, positive = good
	Feedback       string  // human-readable narrative for injection into context
}

// LIEEvaluator tracks the reasoning trajectory of a conversation and evaluates
// each new response against it.
type LIEEvaluator struct {
	// Minimum response length (in tokens) considered "comprehensive".
	MinComprehensiveTokens int
	// Weight applied to the length score component.
	LengthWeight float64
	// Weight applied to the redundancy penalty.
	RedundancyWeight float64
	// Weight applied to the structure bonus.
	StructureWeight float64
	// Sliding window of recent AI responses for redundancy detection.
	recentResponses []string
	maxWindowSize   int
}

// NewLIEEvaluator creates a LIEEvaluator with production-safe defaults.
func NewLIEEvaluator() *LIEEvaluator {
	return &LIEEvaluator{
		MinComprehensiveTokens: 80,
		LengthWeight:           0.5,
		RedundancyWeight:       0.3,
		StructureWeight:        0.2,
		maxWindowSize:          5,
	}
}

// Evaluate scores a new AI response against the recent trajectory.
// It returns a RewardMetrics and a bool indicating whether feedback should be
// injected into the context (true = inject).
func (l *LIEEvaluator) Evaluate(response string) (RewardMetrics, bool) {
	tokens := estimateTokens(response)

	// ── Length score ─────────────────────────────────────────────────────────
	// Sigmoid curve: 0 at 0 tokens, ~0.5 at minComprehensive, approaches 1.
	x := float64(tokens) / float64(l.MinComprehensiveTokens)
	lengthScore := 1 / (1 + math.Exp(-3*(x-1))) // centred at MinComprehensiveTokens

	// ── Redundancy penalty ───────────────────────────────────────────────────
	redundancy := l.measureRedundancy(response)

	// ── Structure bonus ──────────────────────────────────────────────────────
	structureBonus := l.measureStructure(response)

	// ── Final reward ─────────────────────────────────────────────────────────
	reward := l.LengthWeight*lengthScore -
		l.RedundancyWeight*redundancy +
		l.StructureWeight*structureBonus

	// Clamp to [-1, 1].
	if reward > 1 {
		reward = 1
	} else if reward < -1 {
		reward = -1
	}

	metrics := RewardMetrics{
		LengthScore:       lengthScore,
		RedundancyPenalty: redundancy,
		StructureBonus:    structureBonus,
		FinalReward:       reward,
	}

	// Build feedback narrative — only inject when meaningful.
	inject := false
	if reward < 0.2 || redundancy > 0.6 {
		metrics.Feedback = l.buildFeedback(metrics, tokens)
		inject = true
	}

	// Update trajectory window.
	l.recentResponses = append(l.recentResponses, response)
	if len(l.recentResponses) > l.maxWindowSize {
		l.recentResponses = l.recentResponses[1:]
	}

	return metrics, inject
}

// FormatSystemMessage returns a SYSTEM message suitable for injection into
// the conversation history when feedback is warranted.
func (l *LIEEvaluator) FormatSystemMessage(feedback string) string {
	return fmt.Sprintf(
		"[SENSE-LIE FEEDBACK]: %s\n"+
			"Please adjust your next response to be more comprehensive and diverse.",
		feedback)
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// measureRedundancy computes the Jaccard similarity between the new response
// and the recent trajectory (0 = fully unique, 1 = completely redundant).
func (l *LIEEvaluator) measureRedundancy(response string) float64 {
	if len(l.recentResponses) == 0 {
		return 0
	}
	responseTrigrams := trigrams(response)
	if len(responseTrigrams) == 0 {
		return 0
	}

	var totalSimilarity float64
	for _, prev := range l.recentResponses {
		prevTrigrams := trigrams(prev)
		intersection := 0
		for t := range responseTrigrams {
			if prevTrigrams[t] {
				intersection++
			}
		}
		union := len(responseTrigrams) + len(prevTrigrams) - intersection
		if union > 0 {
			totalSimilarity += float64(intersection) / float64(union)
		}
	}
	avg := totalSimilarity / float64(len(l.recentResponses))
	return avg
}

// measureStructure rewards responses that have explicit reasoning structure:
// presence of numbered sections, code blocks, or reasoning-then-answer pattern.
func (l *LIEEvaluator) measureStructure(response string) float64 {
	score := 0.0
	lower := strings.ToLower(response)

	// Reasoning markers.
	if strings.Contains(lower, "let me think") ||
		strings.Contains(lower, "reasoning:") ||
		strings.Contains(lower, "step 1") ||
		strings.Contains(lower, "first,") {
		score += 0.4
	}

	// Code block present.
	if strings.Contains(response, "```") {
		score += 0.3
	}

	// Sectioned output (numbered list or headers).
	if strings.Contains(response, "1.") || strings.Contains(response, "##") {
		score += 0.3
	}

	if score > 1 {
		score = 1
	}
	return score
}

// buildFeedback constructs a concise human-readable feedback string.
func (l *LIEEvaluator) buildFeedback(m RewardMetrics, tokens int) string {
	var parts []string
	if m.LengthScore < 0.4 {
		parts = append(parts, fmt.Sprintf("response is too brief (%d tokens, aim for ≥%d)", tokens, l.MinComprehensiveTokens))
	}
	if m.RedundancyPenalty > 0.6 {
		parts = append(parts, "response is repetitive compared to recent turns")
	}
	if m.StructureBonus < 0.1 {
		parts = append(parts, "consider structuring your response with numbered steps or sections")
	}
	if len(parts) == 0 {
		return "low overall quality score — please provide a more detailed and original response"
	}
	return strings.Join(parts, "; ")
}

// trigrams converts a string to a set of character trigrams for similarity comparison.
func trigrams(s string) map[string]bool {
	words := strings.Fields(strings.ToLower(s))
	set := make(map[string]bool)
	for i := 0; i+2 < len(words); i++ {
		set[words[i]+"|"+words[i+1]+"|"+words[i+2]] = true
	}
	return set
}
