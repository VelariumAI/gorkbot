package sense

// compression.go implements the SENSE 4-stage unified state compression pipeline.
//
// The pipeline produces a StateSnapshot from a conversation window:
//   Stage 1 — Scratchpad:      Extract raw key facts and action items from messages.
//   Stage 2 — DraftSnapshot:   Synthesize facts into a structured summary draft.
//   Stage 3 — SelfCritique:    Score the draft for completeness and coherence.
//   Stage 4 — StateSnapshot:   Emit the final, compact representation.
//
// Because Gorkbot uses API-based models, the compression is performed by
// calling the consultant (Gemini) as a Summarizer rather than a local model.
// The compressor is intentionally stateless — callers drive stage progression.

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// TextGenerator is the minimal interface required by the compressor.
// Both ai.AIProvider and any test stub that implements Generate() satisfy it.
type TextGenerator interface {
	Generate(ctx context.Context, prompt string) (string, error)
}

// ConversationMessage is the minimal shape the compressor needs from history.
type ConversationMessage struct {
	Role    string
	Content string
}

// StateSnapshot is the output of the 4-stage compression pipeline.
type StateSnapshot struct {
	// Stage outputs — available individually for debugging.
	Scratchpad    string `json:"scratchpad"`
	DraftSnapshot string `json:"draft_snapshot"`
	SelfCritique  string `json:"self_critique"`
	// Final compact representation (stage 4).
	Summary   string    `json:"summary"`
	TokenSave int       `json:"token_save"` // approx tokens saved vs. raw history
	CreatedAt time.Time `json:"created_at"`
}

// Compressor runs the 4-stage pipeline using a TextGenerator.
type Compressor struct {
	gen TextGenerator
}

// NewCompressor creates a Compressor backed by the given generator.
func NewCompressor(gen TextGenerator) *Compressor {
	return &Compressor{gen: gen}
}

// Compress runs all 4 stages and returns the StateSnapshot.
// messages is the conversation window to compress (typically the "middle" slice
// that is no longer needed in the hot context window).
//
// Pre-pass: high-importance messages (tool results, approvals, engrams) are
// scored and preserved verbatim; only the remaining messages go through the
// 4-stage compression pipeline.
func (c *Compressor) Compress(ctx context.Context, messages []ConversationMessage) (StateSnapshot, error) {
	// Phase 2.3: Importance-weighted pre-pass
	compressable, preserved := partitionByImportance(messages)

	// Compress only the non-preserved messages
	rawMessages := compressable
	if len(rawMessages) == 0 && len(preserved) > 0 {
		// Nothing to compress — just format preserved messages as the snapshot
		return StateSnapshot{
			Summary:   formatMessages(preserved),
			TokenSave: 0,
			CreatedAt: time.Now(),
		}, nil
	}
	raw := formatMessages(rawMessages)

	// Stage 1 — Scratchpad: surface raw facts.
	scratchpad, err := c.stage1Scratchpad(ctx, raw)
	if err != nil {
		return StateSnapshot{}, fmt.Errorf("compress stage1: %w", err)
	}

	// Stage 2 — Draft snapshot: structure the facts.
	draft, err := c.stage2Draft(ctx, scratchpad)
	if err != nil {
		return StateSnapshot{}, fmt.Errorf("compress stage2: %w", err)
	}

	// Stage 3 — Self-critique: evaluate the draft.
	critique, err := c.stage3Critique(ctx, draft)
	if err != nil {
		return StateSnapshot{}, fmt.Errorf("compress stage3: %w", err)
	}

	// Stage 4 — State snapshot: produce the final compact form.
	final, err := c.stage4Snapshot(ctx, draft, critique)
	if err != nil {
		return StateSnapshot{}, fmt.Errorf("compress stage4: %w", err)
	}

	rawTokens := estimateTokens(raw)
	finalTokens := estimateTokens(final)
	saved := rawTokens - finalTokens
	if saved < 0 {
		saved = 0
	}

	// Append preserved turns verbatim after the compressed summary
	summaryWithPreserved := final
	if len(preserved) > 0 {
		summaryWithPreserved += "\n\n### Preserved Context (high-importance turns):\n" + formatMessages(preserved)
	}

	return StateSnapshot{
		Scratchpad:    scratchpad,
		DraftSnapshot: draft,
		SelfCritique:  critique,
		Summary:       summaryWithPreserved,
		TokenSave:     saved,
		CreatedAt:     time.Now(),
	}, nil
}

// partitionByImportance scores each message and splits them into:
//   - compressable: messages below the preservation threshold (go through pipeline)
//   - preserved: high-importance messages injected verbatim into the output
func partitionByImportance(messages []ConversationMessage) (compressable, preserved []ConversationMessage) {
	// Build a set of all message content for forward-reference detection
	allContent := make([]string, len(messages))
	for i, m := range messages {
		allContent[i] = strings.ToLower(m.Content)
	}

	for i, msg := range messages {
		score := importanceScore(msg, i, allContent)
		if score >= 0.7 {
			preserved = append(preserved, msg)
		} else {
			compressable = append(compressable, msg)
		}
	}
	return
}

// importanceScore scores a single message 0.0–1.0 for preservation.
func importanceScore(msg ConversationMessage, idx int, allContent []string) float64 {
	lower := strings.ToLower(msg.Content)
	score := 0.0

	// Tool result or call: high signal
	if strings.Contains(lower, "tool_result") || strings.Contains(lower, `"result"`) ||
		strings.Contains(lower, "executed successfully") || strings.Contains(lower, "tool call") {
		score += 0.4
	}
	// User explicit approval
	if msg.Role == "user" && (strings.Contains(lower, "approve") || strings.Contains(lower, "confirmed") ||
		strings.Contains(lower, "yes, proceed") || strings.Contains(lower, "go ahead")) {
		score += 0.3
	}
	// Engram recording — always preserve
	if strings.Contains(lower, "record_engram") || strings.Contains(lower, "engram:") {
		score += 0.5
	}
	// Referenced in a later message (forward reference detection)
	if idx < len(allContent)-1 {
		snippet := lower
		if len(snippet) > 40 {
			snippet = snippet[:40]
		}
		for j := idx + 1; j < len(allContent); j++ {
			if strings.Contains(allContent[j], snippet[:min(20, len(snippet))]) {
				score += 0.3
				break
			}
		}
	}

	if score > 1.0 {
		score = 1.0
	}
	return score
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// CompressFast produces a concise summary in a single LLM call.
// Used by auto-compression paths (TieredCompactor) where speed matters more
// than exhaustive quality. The full 4-stage Compress() is retained for the
// explicit /compress and /compact commands.
func (c *Compressor) CompressFast(ctx context.Context, messages []ConversationMessage) (StateSnapshot, error) {
	raw := formatMessages(messages)
	prompt := fmt.Sprintf(
		`Summarize this conversation into a concise system-message context block (under 200 words).
Cover: what the user is working on, key decisions made, tool outputs that matter, user preferences noted.
Be factual and brief. Start with "## Session Context\n".

CONVERSATION:
%s

SUMMARY:`, raw)
	summary, err := c.gen.Generate(ctx, prompt)
	if err != nil {
		return StateSnapshot{}, fmt.Errorf("compress fast: %w", err)
	}
	saved := estimateTokens(raw) - estimateTokens(summary)
	if saved < 0 {
		saved = 0
	}
	return StateSnapshot{Summary: summary, TokenSave: saved, CreatedAt: time.Now()}, nil
}

// stage1Scratchpad extracts raw key facts from the conversation.
func (c *Compressor) stage1Scratchpad(ctx context.Context, raw string) (string, error) {
	prompt := fmt.Sprintf(
		`You are a scratchpad extractor. Read the following conversation segment and list ONLY the raw key facts, decisions, tool outputs, and user preferences as brief bullet points. Do not summarise yet — just extract facts.

CONVERSATION:
%s

FACTS (bullet list):`, raw)
	return c.gen.Generate(ctx, prompt)
}

// stage2Draft synthesises the scratchpad into a structured draft summary.
func (c *Compressor) stage2Draft(ctx context.Context, scratchpad string) (string, error) {
	prompt := fmt.Sprintf(
		`You are a technical writer. Given the following raw facts extracted from a conversation, write a concise structured summary in 3 sections:
1. **Context** — What was the user trying to accomplish?
2. **Actions Taken** — What steps were executed or planned?
3. **Key Outcomes** — What was learned or decided?

Keep it under 150 words.

RAW FACTS:
%s

STRUCTURED SUMMARY:`, scratchpad)
	return c.gen.Generate(ctx, prompt)
}

// stage3Critique evaluates the draft for completeness and coherence.
func (c *Compressor) stage3Critique(ctx context.Context, draft string) (string, error) {
	prompt := fmt.Sprintf(
		`You are a critical reviewer. Evaluate the following summary on a scale of 1–10 for:
- Completeness (does it cover all important facts?)
- Coherence (is it clear and well-structured?)
- Conciseness (is it appropriately brief?)

Respond in ONE SHORT sentence per criterion, then give a final verdict: ACCEPT or REVISE.

SUMMARY:
%s

CRITIQUE:`, draft)
	return c.gen.Generate(ctx, prompt)
}

// stage4Snapshot produces the final compact state snapshot.
func (c *Compressor) stage4Snapshot(ctx context.Context, draft, critique string) (string, error) {
	// If critique says ACCEPT, emit the draft directly.
	if strings.Contains(strings.ToUpper(critique), "ACCEPT") {
		return draft, nil
	}
	// Otherwise ask the model to revise.
	prompt := fmt.Sprintf(
		`Revise the following summary based on the critique. Keep it under 150 words.

ORIGINAL SUMMARY:
%s

CRITIQUE:
%s

REVISED SUMMARY:`, draft, critique)
	return c.gen.Generate(ctx, prompt)
}

// formatMessages converts messages to a plain-text representation.
func formatMessages(msgs []ConversationMessage) string {
	var sb strings.Builder
	for _, m := range msgs {
		role := m.Role
		if len(role) == 0 {
			role = "Unknown"
		} else {
			role = strings.ToUpper(role[:1]) + role[1:]
		}
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", role, m.Content))
	}
	return sb.String()
}

// estimateTokens uses the 4-chars-per-token heuristic.
func estimateTokens(s string) int {
	return (len(s) + 3) / 4
}
