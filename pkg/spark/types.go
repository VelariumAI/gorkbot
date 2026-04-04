package spark

import (
	"context"
	"time"

	"github.com/velariumai/gorkbot/pkg/sense"
)

// Config holds SPARK runtime parameters.
type Config struct {
	Enabled              bool
	ConfigDir            string
	MaxIDLEntries        int           // max IDL queue size (default 50)
	TIIAlpha             float64       // EWMA α for TII success/latency (default 0.1)
	DriveAlpha           float64       // EWMA α for MotivationalCore DriveScore (default 0.1)
	MinCycleInterval     time.Duration // throttle: min time between cycles (default 30s)
	ResearchObjectiveMax int           // max ResearchModule queue depth (default 20)
	LLMObjectiveEnabled  bool          // enable LLM-generated research objectives
}

// DefaultConfig returns a Config with production-safe defaults.
func DefaultConfig(configDir string) *Config {
	return &Config{
		Enabled:              true,
		ConfigDir:            configDir,
		MaxIDLEntries:        50,
		TIIAlpha:             0.1,
		DriveAlpha:           0.1,
		MinCycleInterval:     30 * time.Second,
		ResearchObjectiveMax: 20,
		LLMObjectiveEnabled:  false,
	}
}

// DirectiveKind classifies the type of self-improvement directive.
type DirectiveKind uint8

const (
	DirectiveRetry     DirectiveKind = iota // retry with adjusted params
	DirectiveFallback                       // switch provider
	DirectivePromptFix                      // amend system prompt
	DirectiveToolBan                        // suspend tool
	DirectiveResearch                       // enqueue research objective
)

// EvolutionaryDirective is an actionable improvement proposed by DiagnosisKernel.
type EvolutionaryDirective struct {
	Kind      DirectiveKind
	ToolName  string
	Rationale string
	Magnitude float64 // 0–1 urgency/confidence
	CreatedAt time.Time
	AppliedAt *time.Time
	Applied   bool
}

// TIIEntry is the per-tool record in the Tool Intelligence Index.
type TIIEntry struct {
	ToolName    string
	Invocations int64
	SuccessRate float64 // EWMA(α=0.1) of 1.0/0.0 outcomes; optimistic start = 1.0
	LatencyEWMA float64 // EWMA latency in milliseconds
	LastError   string
	LastUsed    time.Time
}

// IDLEntry is a priority-keyed improvement debt item.
type IDLEntry struct {
	ID          string
	ToolName    string
	Category    sense.FailureCategory
	Severity    float64 // 0–1, priority key for min-heap eviction
	Description string
	Occurrences int
	FirstSeen   time.Time
	LastSeen    time.Time
}

// ResearchObjective describes a goal for the Research Module.
type ResearchObjective struct {
	ID          string
	Topic       string
	Priority    float64
	Source      string // "heuristic" | "llm" | "diagnosis"
	CreatedAt   time.Time
	CompletedAt *time.Time
}

// SPARKState is the point-in-time snapshot from the Introspection Layer.
type SPARKState struct {
	CapturedAt       time.Time
	TIISnapshot      []TIIEntry
	IDLSnapshot      []IDLEntry
	MemoryPressure   float64 // 0–1 from AgeMem usage stats
	ActiveDirectives int
	DriveScore       float64
	TopObjective     *ResearchObjective
	SubsystemHealth  map[string]string // "ok" | "warn" | "error" | "disabled"
}

// HITLFacade is satisfied by *engine.HITLGuard without importing internal/engine.
type HITLFacade interface {
	RequestApproval(ctx context.Context, action string, detail string) (bool, error)
}

// DirectiveCallbacks provides hooks for directive application.
// Set by spark_hooks.go — avoids import cycle.
type DirectiveCallbacks struct {
	OnProviderFallback func(rationale string)
	OnPromptAmend      func(toolName, rationale string)
	OnToolBan          func(toolName, rationale string)
}
