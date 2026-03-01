package sense

// stabilizer.go — SENSE Stabilizer (Critic role)
//
// The Stabilizer acts as a Subconscious Supervisor that monitors the dual-tier
// reasoning output (Primary: Grok + Consultant: Gemini) and applies corrective
// pressure when quality degrades.  It replaces / enhances the existing
// AnalyzeAgency with a richer scoring model derived from SENSE v4.1.
//
// Quality is scored on four dimensions:
//   - Factual confidence  (does the response express appropriate certainty?)
//   - Task alignment      (does it address the user's actual request?)
//   - Tool utilisation    (did it use tools when appropriate?)
//   - Self-awareness      (does it acknowledge limitations correctly?)

import (
	"context"
	"fmt"
	"strings"
)

// QualityScore holds the Stabilizer's evaluation of one AI response.
type QualityScore struct {
	FactualConfidence  float64 // 0–1
	TaskAlignment      float64 // 0–1
	ToolUtilisation    float64 // 0–1
	SelfAwareness      float64 // 0–1
	Overall            float64 // weighted mean
	Action             StabiliserAction
	Advice             string // injected into context when action != ActionNone
}

// StabiliserAction enumerates what the Stabilizer recommends.
type StabiliserAction int

const (
	ActionNone     StabiliserAction = iota // response is good, do nothing
	ActionAdvise                           // inject gentle correction advice
	ActionEscalate                         // trigger a full Consultant consultation
)

// Stabilizer wraps an AI consultant to score responses and prescribe actions.
type Stabilizer struct {
	consultant TextGenerator
	// Threshold below which ActionAdvise is triggered (default 0.55).
	AdviseThreshold float64
	// Threshold below which ActionEscalate is triggered (default 0.35).
	EscalateThreshold float64
}

// NewStabilizer creates a Stabilizer backed by the given consultant.
// Pass nil to use heuristic-only evaluation (no LLM call).
func NewStabilizer(consultant TextGenerator) *Stabilizer {
	return &Stabilizer{
		consultant:        consultant,
		AdviseThreshold:   0.55,
		EscalateThreshold: 0.35,
	}
}

// Evaluate scores the given AI response against the user prompt.
// If the Stabilizer has a consultant, it uses a lightweight LLM call for
// task-alignment scoring; otherwise it falls back to keyword heuristics.
func (s *Stabilizer) Evaluate(ctx context.Context, userPrompt, response string) QualityScore {
	qs := QualityScore{}

	// ── Factual confidence ───────────────────────────────────────────────────
	qs.FactualConfidence = s.scoreFactualConfidence(response)

	// ── Task alignment ───────────────────────────────────────────────────────
	qs.TaskAlignment = s.scoreTaskAlignment(ctx, userPrompt, response)

	// ── Tool utilisation ─────────────────────────────────────────────────────
	qs.ToolUtilisation = s.scoreToolUtilisation(userPrompt, response)

	// ── Self-awareness ───────────────────────────────────────────────────────
	qs.SelfAwareness = s.scoreSelfAwareness(response)

	// ── Weighted overall ─────────────────────────────────────────────────────
	qs.Overall = 0.35*qs.TaskAlignment +
		0.25*qs.FactualConfidence +
		0.20*qs.ToolUtilisation +
		0.20*qs.SelfAwareness

	// ── Action ───────────────────────────────────────────────────────────────
	switch {
	case qs.Overall < s.EscalateThreshold:
		qs.Action = ActionEscalate
		qs.Advice = s.buildEscalationAdvice(qs)
	case qs.Overall < s.AdviseThreshold:
		qs.Action = ActionAdvise
		qs.Advice = s.buildAdvice(qs)
	default:
		qs.Action = ActionNone
	}

	return qs
}

// FormatSystemMessage returns the advice formatted for context injection.
func (s *Stabilizer) FormatSystemMessage(qs QualityScore) string {
	prefix := "[SENSE-STABILIZER]"
	if qs.Action == ActionEscalate {
		prefix = "[SENSE-STABILIZER ESCALATION]"
	}
	return fmt.Sprintf("%s: %s", prefix, qs.Advice)
}

// ─── Scoring helpers ──────────────────────────────────────────────────────────

// scoreFactualConfidence checks for hallucination-risk signals.
func (s *Stabilizer) scoreFactualConfidence(response string) float64 {
	lower := strings.ToLower(response)
	penalties := 0.0

	// Uncertain hedges reduce the penalty slightly (self-aware model).
	uncertainPhrases := []string{"i'm not sure", "i think", "approximately", "it may", "possibly"}
	confident := 0
	for _, p := range uncertainPhrases {
		if strings.Contains(lower, p) {
			confident++
		}
	}

	// Hallucination red-flags: overly definitive claims about real-time facts.
	hallucFlags := []string{"as of today", "the latest version is", "currently the price"}
	for _, f := range hallucFlags {
		if strings.Contains(lower, f) {
			penalties += 0.25
		}
	}

	base := 0.75 + float64(confident)*0.05 - penalties
	if base < 0 {
		return 0
	}
	if base > 1 {
		return 1
	}
	return base
}

// scoreTaskAlignment uses heuristic keyword overlap; falls back to LLM if available.
func (s *Stabilizer) scoreTaskAlignment(ctx context.Context, prompt, response string) float64 {
	if s.consultant == nil {
		return s.heuristicAlignment(prompt, response)
	}

	query := fmt.Sprintf(
		`Rate how well the RESPONSE addresses the USER REQUEST on a scale of 0.0 to 1.0.
Reply with ONLY a decimal number (e.g. 0.85).

USER REQUEST: %s

RESPONSE: %s

SCORE:`, prompt, response)

	result, err := s.consultant.Generate(ctx, query)
	if err != nil {
		return s.heuristicAlignment(prompt, response)
	}
	score := parseFloatSafe(strings.TrimSpace(result))
	if score < 0 || score > 1 {
		// LLM returned an out-of-range or unparseable value — fall back.
		return s.heuristicAlignment(prompt, response)
	}
	return score
}

// heuristicAlignment uses simple keyword overlap as a fallback.
func (s *Stabilizer) heuristicAlignment(prompt, response string) float64 {
	words := strings.Fields(strings.ToLower(prompt))
	lower := strings.ToLower(response)
	hit := 0
	for _, w := range words {
		if len(w) > 3 && strings.Contains(lower, w) {
			hit++
		}
	}
	if len(words) == 0 {
		return 0.7
	}
	ratio := float64(hit) / float64(len(words))
	if ratio > 1 {
		return 1
	}
	return ratio
}

// scoreToolUtilisation rewards responses that used tools when the prompt
// implied a need for external data or file operations.
func (s *Stabilizer) scoreToolUtilisation(prompt, response string) float64 {
	promptLower := strings.ToLower(prompt)
	needsTools := strings.ContainsAny(promptLower, "file") ||
		strings.Contains(promptLower, "run") ||
		strings.Contains(promptLower, "search") ||
		strings.Contains(promptLower, "find") ||
		strings.Contains(promptLower, "execute")

	hasToolUse := strings.Contains(response, "```json") ||
		strings.Contains(response, "<tool_result") ||
		strings.Contains(response, "\"tool\":")

	if !needsTools {
		return 0.8 // tool use wasn't needed, neutral
	}
	if hasToolUse {
		return 1.0
	}
	return 0.3 // needed tools but didn't use them
}

// scoreSelfAwareness checks whether the model appropriately acknowledges limits.
func (s *Stabilizer) scoreSelfAwareness(response string) float64 {
	lower := strings.ToLower(response)
	// Positive: acknowledges limits or asks for clarification.
	if strings.Contains(lower, "i don't have access") ||
		strings.Contains(lower, "i cannot") ||
		strings.Contains(lower, "could you clarify") {
		return 0.9
	}
	// Negative: confident claims about things the model can't know.
	if strings.Contains(lower, "i can confirm") ||
		strings.Contains(lower, "definitely is") {
		return 0.5
	}
	return 0.75
}

func (s *Stabilizer) buildAdvice(qs QualityScore) string {
	var parts []string
	if qs.TaskAlignment < 0.5 {
		parts = append(parts, "revisit the user's request and ensure your response directly addresses it")
	}
	if qs.ToolUtilisation < 0.4 {
		parts = append(parts, "consider using an appropriate tool to complete this task accurately")
	}
	if qs.FactualConfidence < 0.5 {
		parts = append(parts, "hedge factual claims you're uncertain about or use tools to verify")
	}
	if len(parts) == 0 {
		return "improve overall response quality"
	}
	return strings.Join(parts, "; ")
}

func (s *Stabilizer) buildEscalationAdvice(qs QualityScore) string {
	return fmt.Sprintf(
		"response quality is critically low (%.2f). Consultant escalation recommended. %s",
		qs.Overall, s.buildAdvice(qs),
	)
}

// parseFloatSafe parses a float64 from a string, returning -1 on error.
func parseFloatSafe(s string) float64 {
	// Only accept strings that look like a decimal number.
	s = strings.TrimSpace(s)
	var v float64
	_, err := fmt.Sscanf(s, "%f", &v)
	if err != nil {
		return -1
	}
	return v
}
