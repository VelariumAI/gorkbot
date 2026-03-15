package sense

// Engrams implement the SENSE "conditional memory" concept for Gorkbot's
// tool registry.  When the agent learns that a specific tool configuration or
// parameter pattern works well (or poorly), it writes that preference into an
// Engram so the preference persists across sessions and can be injected into
// the system context automatically.
//
// Concretely, Engrams are stored in the AgeMem LTM with a special
// "engram:" key prefix so they can be retrieved as a group.

import (
	"fmt"
	"strings"
	"time"
)

// Engram represents a learned preference or conditional memory about a tool
// or behaviour pattern.
type Engram struct {
	ToolName   string    `json:"tool_name"`  // empty if behaviour-level
	Preference string    `json:"preference"` // human-readable preference text
	Condition  string    `json:"condition"`  // when does this apply?
	Confidence float64   `json:"confidence"` // 0.0–1.0
	UpdatedAt  time.Time `json:"updated_at"`
}

// key returns the AgeMem key for this engram.
func (e *Engram) key() string {
	if e.ToolName != "" {
		return fmt.Sprintf("engram:tool:%s", e.ToolName)
	}
	// Use the first 40 chars of the condition as a key fragment.
	frag := e.Condition
	if len(frag) > 40 {
		frag = frag[:40]
	}
	return fmt.Sprintf("engram:behaviour:%s", frag)
}

// EngramStore wraps AgeMem to provide a tool-preference–aware API.
type EngramStore struct {
	mem *AgeMem
}

// NewEngramStore creates an EngramStore backed by the given AgeMem instance.
func NewEngramStore(mem *AgeMem) *EngramStore {
	return &EngramStore{mem: mem}
}

// Record explicitly writes a learned preference into the engram store.
// This is the "explicit write-back" requirement from the SENSE specification —
// whenever the agent discovers a preference, it MUST call this method so the
// learning is not lost.
func (es *EngramStore) Record(engram Engram) {
	if engram.Confidence <= 0 {
		engram.Confidence = 0.7 // sensible default
	}
	engram.UpdatedAt = time.Now()

	content := fmt.Sprintf("Preference: %s | Condition: %s | Confidence: %.2f",
		engram.Preference, engram.Condition, engram.Confidence)

	meta := map[string]interface{}{
		"tool_name":  engram.ToolName,
		"condition":  engram.Condition,
		"confidence": engram.Confidence,
		"updated_at": engram.UpdatedAt,
	}

	// High-confidence engrams (≥ 0.85) are also persisted to LTM.
	// Update: User requested full implementation, so we persist ALL explicit engrams.
	persist := true // engram.Confidence >= 0.85
	es.mem.Store(engram.key(), content, engram.Confidence, meta, persist)
}

// GetForTool retrieves all engrams relevant to a specific tool name.
func (es *EngramStore) GetForTool(toolName string) []*MemoryEntry {
	query := fmt.Sprintf("engram:tool:%s", toolName)
	results := es.mem.Search(query, 10)
	var out []*MemoryEntry
	for _, r := range results {
		if strings.HasPrefix(r.Key, "engram:tool:"+toolName) {
			out = append(out, r)
		}
	}
	return out
}

// GetRelevant returns engrams relevant to the given freeform query.
func (es *EngramStore) GetRelevant(query string) []*MemoryEntry {
	return es.mem.Search("engram "+query, 5)
}

// FormatAsContext returns a compact text block suitable for injection into a
// system prompt.  It surfaces the most relevant engrams for the current prompt.
func (es *EngramStore) FormatAsContext(promptQuery string) string {
	engrams := es.GetRelevant(promptQuery)
	if len(engrams) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("### Learned Preferences (Engrams):\n")
	for _, e := range engrams {
		sb.WriteString(fmt.Sprintf("- %s\n", e.Content))
	}
	return sb.String()
}
