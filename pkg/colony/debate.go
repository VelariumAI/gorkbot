package colony

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Role represents a bee's analytical role in the debate
type Role struct {
	Name   string
	Stance string // e.g. "advocate", "critic", "neutral", "contrarian", "pragmatist"
	Focus  string // e.g. "security", "performance", "maintainability", "user experience"
}

// DefaultRoles provides a balanced set of analytical perspectives
var DefaultRoles = []Role{
	{Name: "Advocate", Stance: "advocate", Focus: "benefits and opportunities"},
	{Name: "Critic", Stance: "critic", Focus: "risks, flaws, and failure modes"},
	{Name: "Pragmatist", Stance: "pragmatist", Focus: "implementation complexity and trade-offs"},
	{Name: "Contrarian", Stance: "contrarian", Focus: "alternative approaches and unconventional angles"},
}

// Bee represents a single analyst in the colony
type Bee struct {
	Role   Role
	Result string
	Err    error
	Took   time.Duration
}

// Hive orchestrates parallel analysis and synthesis
type Hive struct {
	// Runner executes a prompt and returns a response. In production this is
	// wired to the orchestrator's ExecuteTask. Signature: func(ctx, systemPrompt, userPrompt) (string, error)
	Runner func(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// NewHive creates a Hive with the given runner function.
func NewHive(runner func(ctx context.Context, systemPrompt, userPrompt string) (string, error)) *Hive {
	return &Hive{Runner: runner}
}

// Debate runs all roles in parallel against the given question, then synthesizes.
// Returns the synthesized markdown report.
func (h *Hive) Debate(ctx context.Context, question string, roles []Role) (string, error) {
	if len(roles) == 0 {
		roles = DefaultRoles
	}

	bees := make([]Bee, len(roles))
	var wg sync.WaitGroup

	for i, role := range roles {
		wg.Add(1)
		go func(idx int, r Role) {
			defer wg.Done()
			start := time.Now()
			sys := fmt.Sprintf(
				"You are the %s analyst. Your role is to examine the given question through the lens of %s. "+
					"Be specific, concrete, and take a clear stance as a %s. "+
					"Write 2-4 focused paragraphs. Do not hedge — commit to your perspective.",
				r.Name, r.Focus, r.Stance,
			)
			result, err := h.Runner(ctx, sys, question)
			bees[idx] = Bee{
				Role:   r,
				Result: result,
				Err:    err,
				Took:   time.Since(start),
			}
		}(i, role)
	}

	wg.Wait()

	// Check for total failure
	successCount := 0
	for _, b := range bees {
		if b.Err == nil {
			successCount++
		}
	}
	if successCount == 0 {
		return "", fmt.Errorf("all %d bees failed", len(bees))
	}

	// Build synthesis prompt
	var sb strings.Builder
	sb.WriteString("You are the Queen Bee — the synthesizer. Below are analyses from multiple specialist analysts examining the same question.\n\n")
	sb.WriteString("Your task: synthesize these perspectives into a single coherent, actionable response.\n")
	sb.WriteString("- Identify where analysts agree (high-confidence insights)\n")
	sb.WriteString("- Identify where they disagree (areas of genuine trade-off)\n")
	sb.WriteString("- Provide a clear recommendation or conclusion\n")
	sb.WriteString("- Format as well-structured markdown\n\n")
	sb.WriteString("---\n\n")
	sb.WriteString("## Question\n\n")
	sb.WriteString(question)
	sb.WriteString("\n\n---\n\n## Analyst Perspectives\n\n")

	for _, b := range bees {
		if b.Err != nil {
			continue
		}
		sb.WriteString(fmt.Sprintf("### %s (%s)\n\n%s\n\n", b.Role.Name, b.Role.Focus, b.Result))
	}

	synthesis, err := h.Runner(ctx, "You are a synthesis expert.", sb.String())
	if err != nil {
		// Fall back to raw perspectives if synthesis fails
		return sb.String(), nil
	}

	// Build final report
	var out strings.Builder
	out.WriteString("# Bee Colony Analysis\n\n")
	out.WriteString(fmt.Sprintf("**Question**: %s\n\n", question))
	out.WriteString(fmt.Sprintf("**Analysts**: %d  |  **Successful**: %d\n\n", len(bees), successCount))
	out.WriteString("---\n\n")
	out.WriteString("## Synthesis\n\n")
	out.WriteString(synthesis)
	out.WriteString("\n\n---\n\n## Individual Perspectives\n\n")
	for _, b := range bees {
		if b.Err != nil {
			out.WriteString(fmt.Sprintf("### %s — ERROR: %v\n\n", b.Role.Name, b.Err))
			continue
		}
		out.WriteString(fmt.Sprintf("### %s (%s) — %s\n\n%s\n\n", b.Role.Name, b.Role.Stance, b.Took.Round(time.Millisecond), b.Result))
	}
	return out.String(), nil
}
