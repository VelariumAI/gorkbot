package sre

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/ai"
)

// GroundingExtractor calls the primary AI with a structured JSON extraction prompt
// to produce a WorldModelState before any planning or tool use begins.
type GroundingExtractor struct {
	provider ai.AIProvider
	sense    SENSEProvider // nil-safe
	logger   *slog.Logger
}

func NewGroundingExtractor(provider ai.AIProvider, sense SENSEProvider, logger *slog.Logger) *GroundingExtractor {
	if logger == nil {
		logger = slog.Default()
	}
	return &GroundingExtractor{
		provider: provider,
		sense:    sense,
		logger:   logger,
	}
}

// Extract calls provider.Generate with groundingPrompt(prompt).
// Parse order: strict JSON → strip-fences-retry → heuristic line-scan.
// Never returns nil; sets Confidence=0.1 and GroundedAt on any parse failure.
func (g *GroundingExtractor) Extract(ctx context.Context, prompt string) (*WorldModelState, error) {
	if g.provider == nil {
		return &WorldModelState{Confidence: 0, GroundedAt: time.Now()}, nil
	}

	resp, err := g.provider.Generate(ctx, groundingPrompt(prompt))
	if err != nil {
		g.logger.Error("grounding extraction failed", "error", err)
		return &WorldModelState{Confidence: 0.1, GroundedAt: time.Now()}, err
	}

	ws, err := parseGroundingResponse(resp)
	if err != nil {
		g.logger.Warn("grounding parse failed, using heuristic", "error", err)
		// Return best-effort parsed state with low confidence
		if ws == nil {
			ws = &WorldModelState{Confidence: 0.1, GroundedAt: time.Now()}
		}
		return ws, nil
	}

	ws.GroundedAt = time.Now()
	if g.sense != nil {
		g.sense.LogSREGrounding(ws.Confidence, len(ws.Entities), len(ws.Facts))
	}

	return ws, nil
}

// groundingPrompt builds the extraction instruction.
func groundingPrompt(task string) string {
	return `You are a semantic extractor. Analyze the task and extract structured facts.
Respond ONLY with valid JSON — no prose, no markdown fences.
{
  "entities": [],
  "constraints": [],
  "facts": [],
  "anchors": {},
  "confidence": 0.85
}
Task: ` + task
}

// parseGroundingResponse tries 3 parse strategies.
func parseGroundingResponse(raw string) (*WorldModelState, error) {
	// Strategy 1: strict JSON
	var ws WorldModelState
	if err := json.Unmarshal([]byte(raw), &ws); err == nil {
		return &ws, nil
	}

	// Strategy 2: strip markdown fences and retry
	stripped := stripMarkdownFences(raw)
	if stripped != raw {
		if err := json.Unmarshal([]byte(stripped), &ws); err == nil {
			return &ws, nil
		}
	}

	// Strategy 3: heuristic line-scan (best-effort fallback)
	ws = WorldModelState{
		Entities:    []string{},
		Constraints: []string{},
		Facts:       []string{},
		Anchors:     map[string]string{},
		Confidence:  0.3,
	}

	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			// Heuristic: bullets could be facts or entities
			item := strings.TrimLeft(line, "- *")
			item = strings.TrimSpace(item)
			if len(item) > 0 {
				ws.Facts = append(ws.Facts, item)
			}
		}
	}

	if len(ws.Facts) > 0 {
		ws.Confidence = 0.2
	}

	return &ws, nil
}

func stripMarkdownFences(s string) string {
	s = strings.TrimPrefix(s, "```json\n")
	s = strings.TrimPrefix(s, "```\n")
	s = strings.TrimSuffix(s, "\n```")
	s = strings.TrimSuffix(s, "```")
	return s
}
