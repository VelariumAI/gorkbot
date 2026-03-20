package adaptive

// IngressGuard validates that a pruned prompt preserves the semantic intent
// of the original, guarding against ARC Classifier Evasion attacks.
//
// Attack scenario: a malicious user crafts a prompt that, after pruning,
// strips enough semantic signal to trick the ARC Router into classifying
// a complex / resource-heavy request as a lightweight "conversational" task.
// This could drain API credits or cause incorrect tool routing.
//
// Defence: compute the Jaccard similarity of the word bags from the raw and
// pruned prompts. When similarity falls below the evasion threshold (default
// 0.60), the guard signals that the raw prompt should be used for ARC routing
// (even though the LLM still receives the pruned version to save tokens).
type IngressGuard struct {
	// EvasionThreshold is the minimum Jaccard similarity required between
	// raw and pruned word bags before the guard raises a flag.
	// Default: 0.60 (40 % word change triggers re-route via raw prompt).
	EvasionThreshold float64
}

// NewIngressGuard creates a guard with the recommended default threshold.
func NewIngressGuard() *IngressGuard {
	return &IngressGuard{EvasionThreshold: 0.60}
}

// GuardResult is the output of a Validate call.
type GuardResult struct {
	// Similarity is the Jaccard score between raw and pruned word bags.
	Similarity float64
	// EvasionRisk is true when Similarity < EvasionThreshold.
	// The caller should route ARC using the RawPrompt, not the pruned one.
	EvasionRisk bool
	// RawPrompt is a copy of the original prompt (pass-through for callers
	// that branch on EvasionRisk).
	RawPrompt string
}

// Validate compares raw and pruned prompts and returns a GuardResult.
// It is allocation-light and safe for concurrent use.
func (g *IngressGuard) Validate(raw, pruned string) GuardResult {
	rawWords := tokeniseWords(raw)
	prunedWords := tokeniseWords(pruned)
	sim := jaccardWords(rawWords, prunedWords)

	return GuardResult{
		Similarity:  sim,
		EvasionRisk: sim < g.EvasionThreshold,
		RawPrompt:   raw,
	}
}
