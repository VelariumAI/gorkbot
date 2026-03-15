// Package mel implements Meta-Experience Learning: it converts tool failures
// into permanent heuristics that are injected into future system prompts.
package adaptive

import "time"

// Heuristic encodes a learned failure→correction pattern using the template:
// "When [context], I must strictly verify [constraint] to avoid [error]."
type Heuristic struct {
	Context     string    `json:"context"`             // What situation triggers this heuristic
	Constraint  string    `json:"constraint"`          // What must be verified
	Error       string    `json:"error"`               // What failure this prevents
	ContextTags []string  `json:"context_tags"`        // Keywords for similarity retrieval
	Confidence  float64   `json:"confidence"`          // 0.0–1.0, increases with UseCount
	UseCount    int       `json:"use_count"`           // Times this heuristic was surfaced
	Embedding   []float32 `json:"embedding,omitempty"` // Dense vector; nil when not yet computed
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Text returns the heuristic as a natural-language sentence.
func (h *Heuristic) Text() string {
	return "When " + h.Context +
		", I must strictly verify " + h.Constraint +
		" to avoid " + h.Error + "."
}
