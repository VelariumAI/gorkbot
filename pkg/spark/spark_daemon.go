package spark

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/sense"
)

// SPARK is the Self-Propelling Autonomous Reasoning Kernel.
// It runs an 8-step self-improvement cycle triggered after each task
// completion, integrating with every existing SENSE subsystem.
type SPARK struct {
	cfg          *Config
	tii          *EfficiencyEngine
	idl          *ImprovementDebtLedger
	introspector *Introspector
	diagnosis    *DiagnosisKernel
	mc           *MotivationalCore
	rm           *ResearchModule
	metrics      *Metrics
	hitl         HITLFacade           // nil-safe
	lie          *sense.LIEEvaluator  // nil-safe
	analyzer     *sense.TraceAnalyzer // nil-safe
	ageMem       ageMemReader         // nil-safe
	callbacks    DirectiveCallbacks
	triggerCh    chan struct{} // buffered(1)
	stopCh       chan struct{}
	mu           sync.Mutex
	running      atomic.Bool

	// State guarded by stateMu, read by PrepareContext.
	stateMu     sync.RWMutex
	lastState   *SPARKState
	lastCycleAt time.Time
	cycleCount  int64

	logger *slog.Logger
}

// New creates a SPARK daemon.
// lie, analyzer, ageMem, and aiProv are all nil-safe optional components.
func New(cfg *Config, lie *sense.LIEEvaluator, analyzer *sense.TraceAnalyzer,
	ageMem ageMemReader, aiProv ai.AIProvider, logger *slog.Logger) *SPARK {

	if logger == nil {
		logger = slog.Default()
	}

	dataDir := filepath.Join(cfg.ConfigDir, "spark")
	traceDir := filepath.Join(dataDir, "traces")

	tii := NewEfficiencyEngine(cfg.TIIAlpha, dataDir)
	idl := NewImprovementDebtLedger(cfg.MaxIDLEntries, dataDir)

	var mc *MotivationalCore
	if lie != nil {
		mc = NewMotivationalCore(lie, cfg.DriveAlpha)
	}

	rm := NewResearchModule(cfg.ResearchObjectiveMax)
	if aiProv != nil && cfg.LLMObjectiveEnabled {
		rm.SetLLMProvider(aiProv)
	}

	diag := NewDiagnosisKernel(analyzer, tii, idl)
	intro := NewIntrospector(tii, idl, ageMem, mc, rm)
	metrics := NewMetrics(traceDir)

	return &SPARK{
		cfg:          cfg,
		tii:          tii,
		idl:          idl,
		introspector: intro,
		diagnosis:    diag,
		mc:           mc,
		rm:           rm,
		metrics:      metrics,
		lie:          lie,
		analyzer:     analyzer,
		ageMem:       ageMem,
		triggerCh:    make(chan struct{}, 1),
		stopCh:       make(chan struct{}),
		logger:       logger,
	}
}

// SetHITL wires the HITL approval facade (optional).
func (s *SPARK) SetHITL(h HITLFacade) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hitl = h
}

// SetCallbacks wires directive application callbacks.
func (s *SPARK) SetCallbacks(cb DirectiveCallbacks) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callbacks = cb
}

// Start launches the SPARK run loop as a goroutine.
// No-op if already running.
func (s *SPARK) Start(ctx context.Context) {
	if !s.running.CompareAndSwap(false, true) {
		return
	}
	go s.runLoop(ctx)
}

// Stop shuts down the run loop.
func (s *SPARK) Stop() {
	select {
	case <-s.stopCh:
	default:
		close(s.stopCh)
	}
}

// TriggerCycle requests a new cycle.  Non-blocking; drops if a trigger is
// already pending.
func (s *SPARK) TriggerCycle() {
	select {
	case s.triggerCh <- struct{}{}:
	default:
	}
}

// AppendToolEvent records a tool execution outcome into TII and (on repeated
// failure) pushes an IDL debt entry.
func (s *SPARK) AppendToolEvent(toolName string, success bool, latencyMS int64, errMsg string) {
	if success {
		s.tii.RecordSuccess(toolName, latencyMS)
	} else {
		s.tii.RecordFailure(toolName, latencyMS, errMsg)

		// Push to IDL after 3+ failures with low success rate.
		entry := s.tii.GetEntry(toolName)
		if entry != nil && entry.Invocations > 3 && entry.SuccessRate < 0.6 {
			idlEntry := IDLEntry{
				ID:          fmt.Sprintf("%s:tii_threshold:%d", toolName, entry.Invocations),
				ToolName:    toolName,
				Category:    sense.CatToolFailure,
				Severity:    clamp01(1.0 - entry.SuccessRate),
				Description: fmt.Sprintf("Tool %q has degraded success rate (%.2f)", toolName, entry.SuccessRate),
			}
			s.idl.Push(idlEntry)
			s.metrics.Emit(SPARKEvent{
				Time:     time.Now(),
				Kind:     EventIDLAdded,
				ToolName: toolName,
				Payload:  map[string]interface{}{"severity": idlEntry.Severity},
			})
		}
	}
	s.metrics.Emit(SPARKEvent{
		Time:     time.Now(),
		Kind:     EventTIIUpdate,
		ToolName: toolName,
		Payload:  map[string]interface{}{"success": success, "latency_ms": latencyMS},
	})
}

// ObserveResponse feeds a completed AI response to the MotivationalCore.
func (s *SPARK) ObserveResponse(response string) {
	if s.mc != nil {
		s.mc.Observe(response)
	}
}

// PrepareContext builds the [SPARK_CONTEXT] block for injection.
// Returns "" if no meaningful data is available yet.
func (s *SPARK) PrepareContext() string {
	s.stateMu.RLock()
	state := s.lastState
	s.stateMu.RUnlock()
	if state == nil {
		return ""
	}

	var parts []string

	// TII block — top 8 tools.
	tiiBlock := s.tii.GetContextBlock(8)
	if tiiBlock != "" {
		parts = append(parts, "[SPARK: Tool Intelligence Index]\n"+tiiBlock)
	}

	// IDL block — top 5 entries.
	idlItems := s.idl.Top(5)
	if len(idlItems) > 0 {
		var idlSb strings.Builder
		idlSb.WriteString("[SPARK: Improvement Debt Ledger]\n")
		for _, item := range idlItems {
			idlSb.WriteString(fmt.Sprintf("  [%.2f] %s: %s\n", item.Severity, item.ToolName, item.Description))
		}
		parts = append(parts, idlSb.String())
	}

	// MotivationalCore block.
	if s.mc != nil {
		parts = append(parts, s.mc.FormatDriveBlock())
	}

	// Research objectives block — top 3.
	if s.rm != nil {
		objBlock := s.rm.FormatObjectivesBlock(3)
		if objBlock != "" {
			parts = append(parts, objBlock)
		}
	}

	if len(parts) == 0 {
		return ""
	}

	result := strings.Join(parts, "\n\n")
	s.metrics.InjectionsTotal.Add(1)
	s.metrics.Emit(SPARKEvent{
		Time:    time.Now(),
		Kind:    EventContextInject,
		Payload: map[string]interface{}{"bytes": len(result)},
	})
	return result
}

// GetStatus returns a one-line summary of SPARK state.
func (s *SPARK) GetStatus() string {
	s.stateMu.RLock()
	cycles := s.cycleCount
	lastAt := s.lastCycleAt
	s.stateMu.RUnlock()

	driveScore := 0.0
	if s.mc != nil {
		driveScore = s.mc.DriveScore()
	}
	rmLen := 0
	if s.rm != nil {
		rmLen = s.rm.Len()
	}
	return fmt.Sprintf("SPARK cycles=%d last=%s drive=%.3f tii=%d idl=%d objectives=%d",
		cycles,
		formatDuration(time.Since(lastAt)),
		driveScore,
		len(s.tii.Snapshot()),
		s.idl.Len(),
		rmLen,
	)
}

// ─── internal run loop ─────────────────────────────────────────────────────

func (s *SPARK) runLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			s.metrics.Close()
			s.running.Store(false)
			return
		case <-s.stopCh:
			s.metrics.Close()
			s.running.Store(false)
			return
		case <-s.triggerCh:
			s.stateMu.RLock()
			lastAt := s.lastCycleAt
			s.stateMu.RUnlock()
			if time.Since(lastAt) < s.cfg.MinCycleInterval {
				// Rate-limited — skip.
				continue
			}
			s.runOneCycle(ctx)
		}
	}
}

// runOneCycle executes the full 8-step SPARK improvement cycle.
func (s *SPARK) runOneCycle(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("SPARK: cycle panic recovered", "recover", r)
			s.metrics.CyclesFailed.Add(1)
		}
	}()

	start := time.Now()
	s.metrics.CyclesTotal.Add(1)
	s.metrics.Emit(SPARKEvent{Time: start, Kind: EventCycleStart})

	// ── Step 1: Introspection ─────────────────────────────────────────────
	state := s.introspector.Snapshot(ctx)

	// ── Step 2: Efficiency Audit ──────────────────────────────────────────
	// Push degraded TII entries to IDL.
	for _, entry := range state.TIISnapshot {
		if entry.Invocations >= 5 && entry.SuccessRate < 0.5 {
			idlEntry := IDLEntry{
				ID:          fmt.Sprintf("%s:tii_audit:%d", entry.ToolName, entry.Invocations),
				ToolName:    entry.ToolName,
				Category:    sense.CatToolFailure,
				Severity:    clamp01(1.0 - entry.SuccessRate),
				Description: fmt.Sprintf("Efficiency audit: sr=%.2f over %d calls", entry.SuccessRate, entry.Invocations),
			}
			s.idl.Push(idlEntry)
		}
	}

	// ── Step 3: FRC Diagnosis ─────────────────────────────────────────────
	frcResults := s.diagnosis.AnalyzeFRC(ctx)

	// ── Step 4: Directive Application ────────────────────────────────────
	s.mu.Lock()
	callbacks := s.callbacks
	hitl := s.hitl
	s.mu.Unlock()

	applied := 0
	for _, frc := range frcResults {
		for i := range frc.Directives {
			dir := &frc.Directives[i]
			// HITL gate for high-stakes directives.
			if hitl != nil && (dir.Kind == DirectiveToolBan || dir.Kind == DirectivePromptFix) && dir.Magnitude > 0.8 {
				approved, err := hitl.RequestApproval(ctx,
					fmt.Sprintf("SPARK directive: %s", directiveKindString(dir.Kind)),
					dir.Rationale)
				if err != nil || !approved {
					s.logger.Info("SPARK: directive rejected by HITL", "kind", directiveKindString(dir.Kind), "tool", dir.ToolName)
					continue
				}
			}
			if s.diagnosis.ApplyDirective(ctx, dir, callbacks) {
				applied++
				s.metrics.DirectivesApplied.Add(1)
				s.metrics.Emit(SPARKEvent{
					Time:     time.Now(),
					Kind:     EventDirectiveApply,
					ToolName: dir.ToolName,
					Payload: map[string]interface{}{
						"kind":      directiveKindString(dir.Kind),
						"rationale": dir.Rationale,
						"magnitude": dir.Magnitude,
					},
				})
			}
		}
	}

	// ── Step 5: Research Objectives ───────────────────────────────────────
	if s.rm != nil {
		newObjs := s.rm.GenerateObjectives(ctx, state)
		for _, obj := range newObjs {
			s.rm.Push(obj)
			s.metrics.Emit(SPARKEvent{
				Time:    time.Now(),
				Kind:    EventObjectiveAdd,
				Payload: map[string]interface{}{"topic": obj.Topic, "priority": obj.Priority},
			})
		}
	}

	// ── Step 6: Motivational Calibration ─────────────────────────────────
	s.stateMu.RLock()
	cycles := s.cycleCount
	s.stateMu.RUnlock()
	if s.mc != nil && cycles%5 == 0 {
		s.mc.CalibrateWeights()
	}

	// ── Step 7: Persist ───────────────────────────────────────────────────
	if err := s.tii.Persist(); err != nil {
		s.logger.Warn("SPARK: TII persist failed", "error", err)
	}
	if err := s.idl.Persist(); err != nil {
		s.logger.Warn("SPARK: IDL persist failed", "error", err)
	}

	// ── Step 8: Update lastState ──────────────────────────────────────────
	dur := time.Since(start)
	s.stateMu.Lock()
	s.lastState = state
	s.lastCycleAt = time.Now()
	s.cycleCount++
	newCycles := s.cycleCount
	s.stateMu.Unlock()

	driveScore := 0.0
	if s.mc != nil {
		driveScore = s.mc.DriveScore()
	}
	s.logger.Info("SPARK: cycle complete",
		"cycle", newCycles,
		"duration_ms", dur.Milliseconds(),
		"applied", applied,
		"idl_size", s.idl.Len(),
		"drive_score", fmt.Sprintf("%.3f", driveScore),
	)
	s.metrics.Emit(SPARKEvent{
		Time: time.Now(),
		Kind: EventCycleEnd,
		Payload: map[string]interface{}{
			"cycle":       newCycles,
			"duration_ms": dur.Milliseconds(),
			"applied":     applied,
			"idl_size":    s.idl.Len(),
			"drive_score": driveScore,
		},
	})
}

// DriveScore returns the current MotivationalCore EWMA quality score.
// Returns 0.5 (neutral) when mc is nil.
func (s *SPARK) DriveScore() float64 {
	if s.mc == nil {
		return 0.5
	}
	return s.mc.DriveScore()
}

// GetLastState returns a read-only snapshot of the last SPARK state.
// Used by the self-improvement driver for autonomous decision-making.
// Returns nil if no cycle has completed yet.
func (s *SPARK) GetLastState() *SPARKState {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.lastState
}

// RecordPhaseDeviation pushes a CatPhaseDeviation IDL debt entry.
// Called by SRE CorrectionEngine on backtrack. Nil-safe.
func (s *SPARK) RecordPhaseDeviation(phase, reason string, severity float64) {
	if s.idl == nil {
		return
	}
	s.idl.Push(IDLEntry{
		ID:          fmt.Sprintf("sre:phase_deviation:%d:%s", time.Now().UnixMilli(), phase),
		Category:    sense.CatPhaseDeviation,
		Severity:    clamp01(severity),
		Description: fmt.Sprintf("SRE phase deviation [%s]: %s", phase, reason),
		FirstSeen:   time.Now(),
		LastSeen:    time.Now(),
		Occurrences: 1,
	})
}

// ObserveEnsemble scores multiple trajectory outputs via MotivationalCore.
// Returns nil when mc is nil (ensemble proceeds unscored).
func (s *SPARK) ObserveEnsemble(outputs []string) []float64 {
	if s.mc == nil {
		return nil
	}
	return s.mc.ObserveEnsemble(outputs)
}

// ─── helpers ───────────────────────────────────────────────────────────────

func directiveKindString(k DirectiveKind) string {
	switch k {
	case DirectiveRetry:
		return "retry"
	case DirectiveFallback:
		return "fallback"
	case DirectivePromptFix:
		return "prompt_fix"
	case DirectiveToolBan:
		return "tool_ban"
	case DirectiveResearch:
		return "research"
	default:
		return fmt.Sprintf("unknown(%d)", k)
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs ago", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.0fm ago", d.Minutes())
	}
	return fmt.Sprintf("%.0fh ago", d.Hours())
}
