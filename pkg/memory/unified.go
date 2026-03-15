package memory

// unified.go — UnifiedMemory: single query interface over AgeMem + Engrams + MEL.
//
// Replaces the triple-query pattern in orchestrator.go:buildMemoryContext()
// with a single deduplicating, ranked call.

import (
	"crypto/md5"
	"fmt"
	"strings"

	"github.com/velariumai/gorkbot/pkg/adaptive"
	"github.com/velariumai/gorkbot/pkg/sense"
)

// UnifiedMemory wraps the three memory systems under one Query/Store interface.
type UnifiedMemory struct {
	AgeMem  *sense.AgeMem
	Engrams *sense.EngramStore
	MEL     *adaptive.VectorStore
}

// NewUnifiedMemory creates a UnifiedMemory backed by the given subsystems.
// Any component may be nil — Query degrades gracefully.
func NewUnifiedMemory(ageMem *sense.AgeMem, engrams *sense.EngramStore, store *adaptive.VectorStore) *UnifiedMemory {
	return &UnifiedMemory{
		AgeMem:  ageMem,
		Engrams: engrams,
		MEL:     store,
	}
}

// Query retrieves relevant context from all three memory systems, deduplicates
// by content hash, and returns up to maxTokens characters of combined output.
func (um *UnifiedMemory) Query(query string, maxTokens int) string {
	seen := make(map[[16]byte]bool)
	var parts []string

	addPart := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		h := md5.Sum([]byte(s))
		if seen[h] {
			return
		}
		seen[h] = true
		parts = append(parts, s)
	}

	// AgeMem: episodic memory relevant to the prompt
	if um.AgeMem != nil {
		relevant := um.AgeMem.FormatRelevant(query, maxTokens/3)
		addPart(relevant)
	}

	// Engrams: persistent tool preferences
	if um.Engrams != nil {
		engCtx := um.Engrams.FormatAsContext(query)
		addPart(engCtx)
	}

	// MEL: learned heuristics (top 5 most relevant)
	if um.MEL != nil {
		heuristics := um.MEL.Query(query, 5)
		if len(heuristics) > 0 {
			var hb strings.Builder
			hb.WriteString("### Learned Heuristics (MEL):\n")
			for _, h := range heuristics {
				hb.WriteString("- ")
				hb.WriteString(h.Text())
				hb.WriteString("\n")
			}
			addPart(hb.String())
		}
	}

	if len(parts) == 0 {
		return ""
	}

	combined := strings.Join(parts, "\n\n")
	// Truncate to maxTokens chars (rough token estimate: 1 token ≈ 4 chars)
	maxChars := maxTokens * 4
	if len(combined) > maxChars {
		combined = combined[:maxChars]
	}
	return combined
}

// Store writes content to AgeMem; if priority >= 0.7, also records to Engrams.
// persist=true promotes the entry to LTM.
func (um *UnifiedMemory) Store(key, content string, priority float64, persist bool) {
	if um.AgeMem == nil {
		return
	}
	um.AgeMem.Store(key, content, priority, nil, persist)
	// High-priority items also go to the engram store as preferences
	if um.Engrams != nil && priority >= 0.7 {
		um.Engrams.Record(sense.Engram{
			Preference: content,
			Condition:  fmt.Sprintf("key: %s", key),
			ToolName:   "",
			Confidence: priority,
		})
	}
}
