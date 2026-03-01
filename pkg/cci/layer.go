package cci

import (
	"log/slog"
	"strings"
)

// ModeManagerIface is the minimal interface CCILayer needs from ModeManager.
// Declaring it here prevents an import cycle with internal/engine.
type ModeManagerIface interface {
	SetMode(name string)
	Name() string
}

// CCILayer is the top-level CCI API used by the Orchestrator.
// It bundles Tier 1, Tier 2, Tier 3, and the Truth Sentry under one handle.
type CCILayer struct {
	Hot           *HotMemory
	Specialists   *SpecialistManager
	ColdStore     *ColdMemoryStore
	DriftDetector *DriftDetector
	Logger        *slog.Logger

	// TriggerFn is the ARC trigger table lookup injected at construction.
	// It maps a file path or task description to a specialist domain name.
	// Set after construction by callers who own the trigger table.
	TriggerFn func(prompt string) string

	configDir string
	cwd       string
}

// NewCCILayer constructs and initializes the full CCI stack.
// configDir: ~/.config/gorkbot or platform equivalent.
// cwd:       current working directory (project root).
func NewCCILayer(configDir, cwd string, logger *slog.Logger) *CCILayer {
	if logger == nil {
		logger = slog.Default()
	}
	cold := NewColdMemoryStore(configDir)
	return &CCILayer{
		Hot:           NewHotMemory(configDir),
		Specialists:   NewSpecialistManager(configDir),
		ColdStore:     cold,
		DriftDetector: NewDriftDetector(cold.SubsystemToFileMap(), cold.docsDir),
		Logger:        logger,
		configDir:     configDir,
		cwd:           cwd,
	}
}

// BuildSystemContext constructs the full system prompt prefix for a session.
//
// Tier 1 (Hot) is always included.
// If a Tier 2 specialist is triggered by the prompt, it is appended.
// Drift warnings (if any) are prepended as Truth Sentry alerts.
func (l *CCILayer) BuildSystemContext(prompt string) string {
	var sb strings.Builder

	// Truth Sentry drift warnings (pre-flight check).
	if l.DriftDetector != nil && l.cwd != "" {
		warnings := l.DriftDetector.Check(l.cwd)
		if len(warnings) > 0 {
			sb.WriteString(FormatDriftWarnings(warnings))
			sb.WriteString("\n\n")
			l.Logger.Warn("CCI drift warnings injected", "count", len(warnings))
		}
	}

	// Tier 1: Hot memory block.
	triggerSummary := l.buildTriggerTableSummary()
	sb.WriteString(l.Hot.BuildBlock(triggerSummary))

	// Tier 2: Specialist if triggered.
	specialist := l.loadSpecialistForPrompt(prompt)
	if specialist != nil {
		sb.WriteString("\n\n")
		sb.WriteString(FormatSpecialistBlock(specialist))
		l.Logger.Info("CCI Tier 2 specialist loaded", "domain", specialist.Domain)
	}

	return sb.String()
}

// RunDriftCheck performs only the drift detection phase and returns formatted
// warnings (empty string if no drift detected). Useful for standalone pre-flight.
func (l *CCILayer) RunDriftCheck(cwd string) string {
	if l.DriftDetector == nil {
		return ""
	}
	warnings := l.DriftDetector.Check(cwd)
	return FormatDriftWarnings(warnings)
}

// RunDriftCheckDefault runs drift detection using the cwd stored at construction time.
func (l *CCILayer) RunDriftCheckDefault() string {
	return l.RunDriftCheck(l.cwd)
}

// HandleGap is called when mcp_context_get_subsystem returns empty string.
// It switches the ModeManager to PLAN mode and returns a notification string
// for the TUI to display to the user.
func (l *CCILayer) HandleGap(subsystem string, modeManager ModeManagerIface) string {
	l.Logger.Warn("CCI gap detected — switching to PLAN mode", "subsystem", subsystem)

	if modeManager != nil {
		modeManager.SetMode("PLAN")
	}

	return "⚠ CCI Gap Detected: No Tier 3 specification found for subsystem \"" + subsystem + "\".\n" +
		"Switching to PLAN mode. Please map the subsystem architecture using read-only tools\n" +
		"and create the specification before proceeding with implementation.\n\n" +
		"Suggested command: mcp_context_update_subsystem {\"name\": \"" + subsystem + "\", \"content\": \"...\"}\n" +
		"Then call: /mode normal to resume coding."
}

// NotifyGapResolved is called after the AI generates a Tier 3 spec for a gap.
// It returns the ModeManager to NORMAL mode.
func (l *CCILayer) NotifyGapResolved(modeManager ModeManagerIface) {
	if modeManager != nil && modeManager.Name() == "PLAN" {
		modeManager.SetMode("NORMAL")
		l.Logger.Info("CCI gap resolved — returning to NORMAL mode")
	}
}

// AnnotatePromptWithHints adds lightweight CCI hints to a prompt if the
// Trigger Table identifies a relevant specialist domain.
// Returns the (possibly augmented) prompt for the AI.
func (l *CCILayer) AnnotatePromptWithHints(prompt string) string {
	if l.TriggerFn == nil {
		return prompt
	}
	domain := l.TriggerFn(prompt)
	if domain == "" {
		return prompt
	}

	spec := l.Specialists.Load(domain)
	if spec == nil {
		// Domain exists in trigger table but no specialist file yet.
		cold := l.ColdStore.GetSubsystem(domain)
		if cold == "" {
			return prompt // gap — caller should handle via HandleGap
		}
		return prompt // Tier 3 available, Tier 2 not yet synthesized — that's fine
	}

	// Specialist exists — the prompt is not modified (it was injected at system level).
	return prompt
}

// SynthesizeSpecialist creates a new Tier 2 specialist from raw content.
// Called by MEL BifurcationAnalyzer when a domain repeatedly causes loops.
func (l *CCILayer) SynthesizeSpecialist(domain, content string) error {
	return l.Specialists.Synthesize(domain, content)
}

// GetStatus returns a diagnostic string about the CCI system state.
func (l *CCILayer) GetStatus() string {
	var sb strings.Builder
	sb.WriteString("## CCI System Status\n\n")

	specialists := l.Specialists.List()
	sb.WriteString("**Tier 2 Specialists**: ")
	if len(specialists) == 0 {
		sb.WriteString("none\n")
	} else {
		sb.WriteString(strings.Join(specialists, ", "))
		sb.WriteString("\n")
	}

	subsystems := l.ColdStore.ListSubsystems()
	sb.WriteString("**Tier 3 Docs**: ")
	if len(subsystems) == 0 {
		sb.WriteString("none\n")
	} else {
		sb.WriteString(strings.Join(subsystems, ", "))
		sb.WriteString("\n")
	}

	sb.WriteString("**Hot memory dir**: ")
	sb.WriteString(l.Hot.conventionsPath)
	sb.WriteString("\n")

	return sb.String()
}

// loadSpecialistForPrompt uses the TriggerFn (or cold store suggestion as fallback)
// to find and load the appropriate Tier 2 specialist.
func (l *CCILayer) loadSpecialistForPrompt(prompt string) *Specialist {
	var domain string

	if l.TriggerFn != nil {
		domain = l.TriggerFn(prompt)
	}
	if domain == "" {
		// Fallback: use cold store keyword scoring.
		domain = l.ColdStore.SuggestSpecialist(prompt)
	}
	if domain == "" {
		return nil
	}

	return l.Specialists.Load(domain)
}

// buildTriggerTableSummary generates a compact markdown summary of the
// current trigger table for injection into the Tier 1 hot block.
func (l *CCILayer) buildTriggerTableSummary() string {
	var sb strings.Builder
	sb.WriteString("| File Path Pattern          | → Specialist Domain  |\n")
	sb.WriteString("|----------------------------|----------------------|\n")

	entries := []struct{ path, domain string }{
		{"internal/tui/", "tui"},
		{"internal/engine/", "orchestrator"},
		{"internal/arc/", "arc-mel"},
		{"internal/mel/", "arc-mel"},
		{"pkg/tools/", "tool-system"},
		{"pkg/ai/", "ai-providers"},
		{"pkg/mcp/", "mcp-integration"},
		{"pkg/sense/", "sense"},
		{"pkg/cci/", "cci"},
		{"pkg/memory/", "memory"},
		{"pkg/subagents/", "subagents"},
		{"pkg/session/", "session"},
		{"pkg/security/", "security"},
		{"pkg/commands/", "commands"},
		{"pkg/providers/", "providers"},
		{"cmd/gorkbot/", "orchestrator"},
	}

	for _, e := range entries {
		sb.WriteString("| `")
		sb.WriteString(e.path)
		sb.WriteString("`")
		// Pad to column width.
		pad := 28 - len(e.path) - 2
		if pad < 0 {
			pad = 0
		}
		sb.WriteString(strings.Repeat(" ", pad))
		sb.WriteString("| ")
		sb.WriteString(e.domain)
		pad2 := 20 - len(e.domain)
		if pad2 < 0 {
			pad2 = 0
		}
		sb.WriteString(strings.Repeat(" ", pad2))
		sb.WriteString(" |\n")
	}

	return sb.String()
}
