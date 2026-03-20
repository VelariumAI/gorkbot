package consultation

// airlock.go — Stage 5: Airlock Validation (Post-Consultation Sanitisation)
//
// The AirlockValidator is the final gate before any Secondary response is
// allowed to touch the Primary model's context array. It performs two passes:
//
// Pass A — Type-based sanitisation:
//   Enforces the contract declared in EntropyVoid.ExpectedType:
//     "code"     → strip markdown fences (``` ... ```)
//     "json"     → attempt json.Unmarshal; extract the first JSON object if
//                  embedded in surrounding prose
//     "analysis" → trim whitespace; require non-empty output
//     "plan"     → normalise leading whitespace; require numbered list
//     "boolean"  → extract TRUE/FALSE prefix; discard everything else
//     "value"    → trim whitespace; require non-empty output
//   On type violation the response is rejected with a structured error that
//   names the violation and includes the raw excerpt for debugging.
//
// Pass B — Reverse semantic check (Self-Correction Loop):
//   Embeds the sanitised response and computes cosine similarity against the
//   original VoidTarget using the local Nomic model.
//   If the distance (1 − similarity) exceeds ConsultationDegradationThreshold,
//   the response is considered semantically drifted — the Secondary produced
//   hallucinated fluff rather than a focused answer — and is rejected with
//   ErrSignalDegradation.
//
//   This check is SKIPPED when the embedder is nil (no local model loaded,
//   e.g., standard build without -tags llamacpp), degrading gracefully to
//   type-only validation.
//
// After both passes, the sanitised response is returned as the Universal Truth
// ready for system-context injection.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"

	"github.com/velariumai/gorkbot/pkg/embeddings"
)

// markdownFenceRe matches fenced code blocks: ```lang\n...\n``` or ~~~\n...\n~~~
// The (?s) flag makes . match newlines so multiline blocks are captured.
var markdownFenceRe = regexp.MustCompile("(?s)```[a-zA-Z0-9]*\\n?(.*?)\\n?```|~~~[a-zA-Z0-9]*\\n?(.*?)\\n?~~~")

// jsonObjectRe matches the first {...} or [...] block in a string.
var jsonObjectRe = regexp.MustCompile(`(?s)(\{.*?\}|\[.*?\])`)

// markdownEmptyHeaderRe matches lines that are markdown heading markers with no content.
// Hoisted from sanitiseAnalysis to avoid recompiling on every line of every response.
var markdownEmptyHeaderRe = regexp.MustCompile(`^#{1,6}\s*$`)

// AirlockValidator performs type sanitisation and reverse semantic validation
// on the Secondary model's raw response.
type AirlockValidator struct {
	// emb is written by SetEmbedder (initEmbedder goroutine) and read
	// concurrently from reverseSemanticCheck goroutines. Interfaces can't use
	// sync/atomic.Pointer, so protect with an RWMutex. Lock duration is
	// nanoseconds (pointer copy only), so no priority-inversion risk.
	embMu sync.RWMutex
	emb   embeddings.Embedder // nil → Pass B is skipped (graceful degradation)
	log   *slog.Logger
}

func newAirlockValidator(emb embeddings.Embedder, log *slog.Logger) *AirlockValidator {
	if log == nil {
		log = slog.Default()
	}
	return &AirlockValidator{emb: emb, log: log}
}

// getEmb returns the current embedder under a minimal read lock.
func (av *AirlockValidator) getEmb() embeddings.Embedder {
	av.embMu.RLock()
	defer av.embMu.RUnlock()
	return av.emb
}

// setEmb atomically replaces the embedder (called from SetEmbedder).
func (av *AirlockValidator) setEmb(emb embeddings.Embedder) {
	av.embMu.Lock()
	av.emb = emb
	av.embMu.Unlock()
}

// Sanitise runs Pass A (type sanitisation) then Pass B (reverse semantic check).
// Returns the cleaned response, or an error if either pass rejects it.
func (av *AirlockValidator) Sanitise(
	ctx context.Context,
	ev EntropyVoid,
	raw string,
) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("consultation/airlock: Secondary returned empty response")
	}

	// ── Pass A: type-based sanitisation ──────────────────────────────────
	sanitised, err := av.sanitiseByType(ev.ExpectedType, raw)
	if err != nil {
		return "", fmt.Errorf("consultation/airlock: type sanitisation failed: %w", err)
	}
	if strings.TrimSpace(sanitised) == "" {
		return "", fmt.Errorf("consultation/airlock: sanitised output is empty for type=%q", ev.ExpectedType)
	}

	// ── Pass B: reverse semantic check ───────────────────────────────────
	// Embed both the sanitised response and the VoidTarget, then compute
	// cosine distance. Responses that drift too far are hallucinated fluff.
	if err := av.reverseSemanticCheck(ctx, ev.VoidTarget, sanitised); err != nil {
		return "", err
	}

	return sanitised, nil
}

// sanitiseByType applies ExpectedType-specific cleaning rules.
func (av *AirlockValidator) sanitiseByType(expectedType, raw string) (string, error) {
	switch expectedType {
	case "code":
		return sanitiseCode(raw)
	case "json":
		return sanitiseJSON(raw)
	case "analysis":
		return sanitiseAnalysis(raw)
	case "plan":
		return sanitisePlan(raw)
	case "boolean":
		return sanitiseBoolean(raw)
	case "value":
		return strings.TrimSpace(raw), nil
	default:
		// Unreachable because ParseAndValidate already enforced the closed set,
		// but defend against direct calls to Sanitise with a raw EntropyVoid.
		return strings.TrimSpace(raw), nil
	}
}

// sanitiseCode strips all markdown fences and trims surrounding whitespace.
// If no fence is found the raw content is returned trimmed — the model may
// have complied with the "no fences" directive, which is the desired outcome.
func sanitiseCode(raw string) (string, error) {
	// Try to extract the first fenced block's content.
	matches := markdownFenceRe.FindStringSubmatch(raw)
	if len(matches) > 0 {
		// matches[1] = content inside ``` block, matches[2] = content inside ~~~ block
		for _, m := range matches[1:] {
			if m != "" {
				return strings.TrimSpace(m), nil
			}
		}
	}
	// No fence detected — the Secondary may have followed the directive correctly.
	// Return raw content trimmed.
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("code response is empty after fence-strip")
	}
	return trimmed, nil
}

// sanitiseJSON attempts to unmarshal the raw string as JSON.
// If unmarshal fails it searches for an embedded JSON object/array and
// attempts to unmarshal that instead. This handles models that wrap JSON
// in prose sentences like "Here is the result: {...}".
func sanitiseJSON(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)

	// First try: is the whole response valid JSON?
	if json.Valid([]byte(trimmed)) {
		return trimmed, nil
	}

	// Second try: strip markdown fences in case the model ignored the directive.
	stripped := stripMarkdownFences(trimmed)
	if json.Valid([]byte(stripped)) {
		return stripped, nil
	}

	// Third try: extract the first JSON object or array from prose.
	match := jsonObjectRe.FindString(stripped)
	if match != "" && json.Valid([]byte(match)) {
		return match, nil
	}

	return "", fmt.Errorf("response does not contain valid JSON: %q…", trimmed[:clamp(len(trimmed), 120)])
}

// sanitiseAnalysis ensures the analysis is non-empty and removes any
// markdown heading prefixes (## Overview) that add visual noise in a
// raw system message.
func sanitiseAnalysis(raw string) (string, error) {
	lines := strings.Split(raw, "\n")
	var clean []string
	for _, l := range lines {
		// Keep lines that are not purely markdown headers with no content.
		if markdownEmptyHeaderRe.MatchString(strings.TrimSpace(l)) {
			continue
		}
		clean = append(clean, l)
	}
	result := strings.TrimSpace(strings.Join(clean, "\n"))
	if result == "" {
		return "", fmt.Errorf("analysis response is empty after sanitisation")
	}
	return result, nil
}

// sanitisePlan normalises a numbered list: ensures at least one numbered step
// and trims trailing whitespace per line.
func sanitisePlan(raw string) (string, error) {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	var clean []string
	hasNumbered := false
	numberedRe := regexp.MustCompile(`^\s*\d+[.)]\s`)

	for _, l := range lines {
		trimmed := strings.TrimRight(l, " \t")
		if numberedRe.MatchString(trimmed) {
			hasNumbered = true
		}
		clean = append(clean, trimmed)
	}

	result := strings.TrimSpace(strings.Join(clean, "\n"))
	if result == "" {
		return "", fmt.Errorf("plan response is empty after sanitisation")
	}
	if !hasNumbered {
		// Accept unnumbered lists (bullet points) but log a warning.
		// Not a hard rejection — the Secondary may have used bullets intentionally.
		slog.Default().Debug("airlock: plan response has no numbered steps — accepting anyway")
	}
	return result, nil
}

// sanitiseBoolean extracts the leading TRUE or FALSE verdict.
// Everything after the first sentence is stripped — the directive allows
// one sentence of justification, and we pass that through.
func sanitiseBoolean(raw string) (string, error) {
	upper := strings.ToUpper(strings.TrimSpace(raw))

	isTrue := strings.HasPrefix(upper, "TRUE")
	isFalse := strings.HasPrefix(upper, "FALSE")

	if !isTrue && !isFalse {
		return "", fmt.Errorf(
			"boolean response must begin with TRUE or FALSE, got: %q",
			raw[:clamp(len(raw), 80)],
		)
	}

	// Extract verdict + first justification sentence (up to '.', '!', or '\n').
	trimmed := strings.TrimSpace(raw)
	end := len(trimmed)
	for i, r := range trimmed {
		if i > 5 && (r == '.' || r == '!' || r == '\n') {
			end = i + 1
			break
		}
	}
	return strings.TrimSpace(trimmed[:end]), nil
}

// ── Reverse semantic check (Pass B) ──────────────────────────────────────

// reverseSemanticCheck embeds both the response and the VoidTarget and
// computes cosine distance. Returns ErrSignalDegradation when the response
// drifts too far from the question's semantic centroid.
//
// Skip conditions (graceful degradation):
//   - av.emb == nil (no local embedder loaded — standard build)
//   - ctx is already cancelled
//   - embedding fails (network / model error)
//
// In all skip cases, the sanitised response passes through without the
// semantic gate. This keeps Gorkbot operational when the local Nomic model
// is unavailable.
func (av *AirlockValidator) reverseSemanticCheck(
	ctx context.Context,
	voidTarget string,
	response string,
) error {
	emb := av.getEmb() // atomic-safe read via RWMutex
	if emb == nil {
		return nil // graceful degradation: no embedder available
	}

	// Embed both strings concurrently to minimise latency on mobile.
	// Capture emb locally so both goroutines use the same instance even if
	// SetEmbedder is called concurrently mid-flight.
	type embedResult struct {
		vec []float32
		err error
	}
	voidCh := make(chan embedResult, 1)
	respCh := make(chan embedResult, 1)

	go func() {
		v, e := emb.Embed(ctx, voidTarget)
		voidCh <- embedResult{vec: v, err: e}
	}()
	go func() {
		v, e := emb.Embed(ctx, response)
		respCh <- embedResult{vec: v, err: e}
	}()

	var voidEmb, respEmb embedResult
	for collected := 0; collected < 2; collected++ {
		select {
		case <-ctx.Done():
			av.log.Debug("airlock: reverse check skipped (context cancelled)")
			return nil // do not fail the consultation on user abort
		case r := <-voidCh:
			voidEmb = r
		case r := <-respCh:
			respEmb = r
		}
	}

	if voidEmb.err != nil || respEmb.err != nil {
		// Embedding failure (model unloaded, OOM, etc.) — skip the check.
		av.log.Warn("airlock: reverse semantic check skipped (embed error)",
			"void_err", voidEmb.err,
			"resp_err", respEmb.err,
		)
		return nil
	}

	// Both vectors are already L2-normalised by the Embedder contract;
	// cosine similarity reduces to a plain dot product.
	sim := embeddings.CosineSimilarity(
		embeddings.L2Normalize(voidEmb.vec),
		embeddings.L2Normalize(respEmb.vec),
	)
	distance := 1.0 - sim

	av.log.Debug("airlock: reverse semantic check",
		"similarity", fmt.Sprintf("%.4f", sim),
		"distance", fmt.Sprintf("%.4f", distance),
		"threshold", ConsultationDegradationThreshold,
	)

	if distance > ConsultationDegradationThreshold {
		return fmt.Errorf(
			"%w: cosine distance=%.3f exceeds threshold=%.3f (similarity=%.3f) — response drifted from VoidTarget",
			ErrSignalDegradation, distance, ConsultationDegradationThreshold, sim,
		)
	}
	return nil
}

// ── Shared helpers ────────────────────────────────────────────────────────

// stripMarkdownFences removes ``` or ~~~ fenced code block delimiters,
// returning the raw inner content. Used by sanitiseJSON as a secondary
// attempt when the full response is not valid JSON.
func stripMarkdownFences(s string) string {
	matches := markdownFenceRe.FindStringSubmatch(s)
	if len(matches) > 0 {
		for _, m := range matches[1:] {
			if m != "" {
				return strings.TrimSpace(m)
			}
		}
	}
	return s
}
