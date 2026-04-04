package engine

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/velariumai/gorkbot/internal/events"
	"github.com/velariumai/gorkbot/internal/observability"
	"github.com/velariumai/gorkbot/pkg/dag"
	"github.com/velariumai/gorkbot/pkg/harness"
	"github.com/velariumai/gorkbot/pkg/selfimprove"
	"github.com/velariumai/gorkbot/pkg/spark"
	"github.com/velariumai/gorkbot/pkg/tools"
)

// ── Adapters ─────────────────────────────────────────────────────────────────

// sparkAdapter wraps *spark.SPARK to implement selfimprove.SPARKFacade.
type sparkAdapter struct {
	s *spark.SPARK
}

func (a *sparkAdapter) GetLastState() *selfimprove.SPARKStateSnapshot {
	if a.s == nil {
		return nil
	}
	state := a.s.GetLastState()
	if state == nil {
		return nil
	}
	return &selfimprove.SPARKStateSnapshot{
		DriveScore:       state.DriveScore,
		IDLDebt:          len(state.IDLSnapshot), // Use IDLSnapshot length
		ActiveDirectives: state.ActiveDirectives,
	}
}

// freeWillAdapter wraps *FreeWillEngine to implement selfimprove.FreeWillFacade.
type freeWillAdapter struct {
	fw *FreeWillEngine
}

func (a *freeWillAdapter) AddObservation(ctx context.Context, input selfimprove.FreeWillObsInput) error {
	if a.fw == nil {
		return nil
	}
	obs := FreeWillObservation{
		Domain:         "self_improve",
		MetricName:     input.ToolName,
		MetricValue:    float64(input.Latency),
		Confidence:     input.Confidence,
		ContextSummary: input.Context,
	}
	select {
	case a.fw.observationQueue <- obs:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("observation queue full")
	}
}

func (a *freeWillAdapter) GetPendingProposals() []selfimprove.FreeWillProposalSummary {
	if a.fw == nil {
		return []selfimprove.FreeWillProposalSummary{}
	}
	a.fw.mu.RLock()
	defer a.fw.mu.RUnlock()

	result := make([]selfimprove.FreeWillProposalSummary, 0, len(a.fw.proposalQueue))
	// Drain the queue (non-blocking, up to cap)
	for {
		select {
		case prop := <-a.fw.proposalQueue:
			result = append(result, selfimprove.FreeWillProposalSummary{
				Target:     prop.ProposedChange,
				Confidence: float64(prop.ConfidenceScore),
				Risk:       riskStringToFloat(prop.RiskLevel),
			})
		default:
			return result
		}
	}
}

func riskStringToFloat(riskStr string) float64 {
	switch riskStr {
	case "low":
		return 0.2
	case "medium":
		return 0.5
	case "high":
		return 0.8
	case "critical":
		return 1.0
	default:
		return 0.5
	}
}

// harnessAdapter reads from the harness feature store.
type harnessAdapter struct {
	cwd string
}

func (a *harnessAdapter) FailingCount() int {
	if a.cwd == "" {
		return 0
	}
	store := harness.NewStore(a.cwd)
	featureList, err := store.LoadFeatureList()
	if err != nil || featureList == nil {
		return 0
	}
	count := 0
	for _, f := range featureList.Features {
		if f.Status == harness.StatusFailing {
			count++
		}
	}
	return count
}

func (a *harnessAdapter) TotalCount() int {
	if a.cwd == "" {
		return 0
	}
	store := harness.NewStore(a.cwd)
	featureList, err := store.LoadFeatureList()
	if err != nil || featureList == nil {
		return 0
	}
	return len(featureList.Features)
}

func (a *harnessAdapter) ActiveFeatureID() string {
	if a.cwd == "" {
		return ""
	}
	store := harness.NewStore(a.cwd)
	state, err := store.LoadState()
	if err != nil || state == nil {
		return ""
	}
	return state.ActiveFeatureID
}

// researchAdapter wraps *research.Engine to implement selfimprove.ResearchFacade.
// research.Engine is accessed as interface{} on the orchestrator to avoid import cycles.
type researchAdapter struct {
	re interface{} // Actually *research.Engine
}

func (a *researchAdapter) BufferedCount() int {
	// Safely cast and access ListBuffered if possible
	// For now, return 0 if not available (nil-safe)
	// The orchestrator will wire a proper research.Engine when available
	return 0
}

// toolRegistryAdapter wraps *tools.Registry to implement selfimprove.ToolRegistryFacade.
type toolRegistryAdapter struct {
	reg *tools.Registry
}

func (a *toolRegistryAdapter) ExecuteTool(ctx context.Context, name string, params map[string]interface{}) (string, error) {
	if a.reg == nil {
		return "", fmt.Errorf("registry not available")
	}

	// Look up the tool
	tool, ok := a.reg.Get(name)
	if !ok {
		return "", fmt.Errorf("tool not found: %s", name)
	}

	// Execute the tool
	result, err := tool.Execute(ctx, params)
	if err != nil {
		return "", err
	}

	if result == nil {
		return "", fmt.Errorf("tool returned nil result")
	}

	if !result.Success && result.Error != "" {
		return result.Error, fmt.Errorf("tool failed: %s", result.Error)
	}

	return result.Output, nil
}

// notifyAdapter channels notifications to the TUI.
type notifyAdapter struct {
	callback func(string)
}

func (a *notifyAdapter) Notify(msg string) {
	if a.callback != nil {
		a.callback(msg)
	}
}

// obsAdapter wraps *observability.ObservabilityHub to implement selfimprove.ObservabilityFacade.
type obsAdapter struct {
	hub *observability.ObservabilityHub
}

// Core metrics (Task 4.3A)

func (a *obsAdapter) RecordSICycleStart() {
	if a.hub != nil {
		a.hub.RecordSICycleStart()
	}
}

func (a *obsAdapter) RecordSIProposal() {
	if a.hub != nil {
		a.hub.RecordSIProposal()
	}
}

func (a *obsAdapter) RecordSIAccepted() {
	if a.hub != nil {
		a.hub.RecordSIAccepted()
	}
}

func (a *obsAdapter) RecordSIRolledBack() {
	if a.hub != nil {
		a.hub.RecordSIRolledBack()
	}
}

func (a *obsAdapter) RecordSIFailed() {
	if a.hub != nil {
		a.hub.RecordSIFailed()
	}
}

// Extended metrics (Task 4.3B)

func (a *obsAdapter) RecordSIExecutionLatency(latency time.Duration) {
	if a.hub != nil {
		a.hub.RecordSIExecutionLatency(latency)
	}
}

func (a *obsAdapter) RecordSIScoreDelta(delta float64) {
	if a.hub != nil {
		a.hub.RecordSIScoreDelta(delta)
	}
}

func (a *obsAdapter) RecordSIGateRejectionReason(reason string) {
	if a.hub != nil {
		a.hub.RecordSIGateRejectionReason(reason)
	}
}

func (a *obsAdapter) RecordSIToolError(toolName string) {
	if a.hub != nil {
		a.hub.RecordSIToolError(toolName)
	}
}

func (a *obsAdapter) RecordSIRollbackLatency(latency time.Duration) {
	if a.hub != nil {
		a.hub.RecordSIRollbackLatency(latency)
	}
}

// ── Orchestrator integration ─────────────────────────────────────────────────

// InitSelfImprove initializes the self-improvement driver.
// Must be called after SPARK and other components are ready.
func (o *Orchestrator) InitSelfImprove() error {
	if o.SPARK == nil {
		return fmt.Errorf("SPARK not initialized")
	}
	if o.Registry == nil {
		return fmt.Errorf("tool registry not available")
	}

	cwd := o.configDir
	if cwd == "" {
		cwd = "."
	}

	// Create adapters
	sparkAdapt := &sparkAdapter{s: o.SPARK}
	fwAdapt := &freeWillAdapter{fw: o.FreeWillEngine}
	harnessAdapt := &harnessAdapter{cwd: cwd}
	researchAdapt := &researchAdapter{re: o.researchEngine}
	toolAdapt := &toolRegistryAdapter{reg: o.Registry}
	notifyAdapt := &notifyAdapter{callback: o.siNotifyCallback}

	// Create the driver
	logger := o.Logger
	if logger == nil {
		logger = slog.Default()
	}

	driver := selfimprove.NewDriver(sparkAdapt, fwAdapt, harnessAdapt,
		researchAdapt, toolAdapt, notifyAdapt, logger)

	o.SIDriver = driver

	// Wire unified pipeline (non-fatal if rollback dir unavailable)
	rbDir := filepath.Join(cwd, ".gorkbot", "tmp")
	if err := o.InitSIPipeline(rbDir); err != nil {
		logger.Warn("SI pipeline wiring skipped", "error", err)
	}

	return nil
}

// InitSIPipeline initializes and wires the unified 7-stage SI pipeline.
func (o *Orchestrator) InitSIPipeline(rollbackCacheDir string) error {
	if o.SIDriver == nil {
		return fmt.Errorf("SI driver not initialized")
	}

	// Create event bus if needed
	if o.eventBus == nil {
		o.eventBus = events.NewBus()
	}

	// Create rollback store
	rb, err := dag.NewRollbackStore(rollbackCacheDir)
	if err != nil {
		return fmt.Errorf("rollback store: %w", err)
	}

	cwd := o.configDir
	if cwd == "" {
		cwd = "."
	}

	// Create adapters for pipeline
	sparkAdapt := &sparkAdapter{s: o.SPARK}
	fwAdapt := &freeWillAdapter{fw: o.FreeWillEngine}
	harnessAdapt := &harnessAdapter{cwd: cwd}
	researchAdapt := &researchAdapter{re: o.researchEngine}
	toolAdapt := &toolRegistryAdapter{reg: o.Registry}
	notifyAdapt := &notifyAdapter{callback: o.siNotifyCallback}
	obsAdapt := &obsAdapter{hub: o.Observability} // o.Observability is *observability.ObservabilityHub

	logger := o.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Create and wire pipeline
	pipeline := selfimprove.NewSIPipeline(
		sparkAdapt,
		fwAdapt,
		harnessAdapt,
		researchAdapt,
		toolAdapt,
		notifyAdapt,
		obsAdapt,
		o.eventBus,
		rb,
		logger,
	)

	o.SIDriver.SetPipeline(pipeline)
	logger.Info("SI unified pipeline initialized")
	return nil
}

// StartSelfImprove begins the autonomous self-improvement loop.
func (o *Orchestrator) StartSelfImprove() error {
	if o.SIDriver == nil {
		if err := o.InitSelfImprove(); err != nil {
			return fmt.Errorf("failed to init SI driver: %w", err)
		}
	}
	return o.SIDriver.Start(o.rootCtx)
}

// StopSelfImprove halts the autonomous self-improvement loop.
func (o *Orchestrator) StopSelfImprove() {
	if o.SIDriver != nil {
		o.SIDriver.Stop()
	}
}

// ToggleSelfImprove switches the self-improvement drive on/off.
// Returns the new enabled state.
func (o *Orchestrator) ToggleSelfImprove() bool {
	if o.SIDriver == nil {
		if err := o.InitSelfImprove(); err != nil {
			return false
		}
	}
	return o.SIDriver.Toggle(o.rootCtx)
}

// SISnapshot returns a point-in-time snapshot of self-improve state.
func (o *Orchestrator) SISnapshot() selfimprove.SISnapshot {
	if o.SIDriver == nil {
		return selfimprove.SISnapshot{Enabled: false}
	}
	return o.SIDriver.Snapshot()
}

// TriggerSICycle forces one immediate self-improvement cycle and returns
// the post-run snapshot. If SI is not initialized, returns disabled snapshot.
func (o *Orchestrator) TriggerSICycle(ctx context.Context) selfimprove.SISnapshot {
	if o.SIDriver == nil {
		return selfimprove.SISnapshot{Enabled: false}
	}
	return o.SIDriver.RunCycleNow(ctx)
}

// SetSINotifyCallback wires the notification callback for SI messages.
func (o *Orchestrator) SetSINotifyCallback(fn func(string)) {
	o.siNotifyCallback = fn
}

// SetEvolvePhaseCallback wires a zero-arg callback called on every evolution phase change.
func (o *Orchestrator) SetEvolvePhaseCallback(cb func()) {
	if o.SIDriver != nil {
		o.SIDriver.SetPhaseCallback(cb)
	}
}

// launchSIPostTask feeds the last assistant response to the SI driver as an observation.
// Called from streaming.go after a successful response.
func (o *Orchestrator) launchSIPostTask(taskSummary string) {
	if o.SIDriver == nil || o.FreeWillEngine == nil {
		return
	}

	// Feed a post-task observation to the Free Will Engine
	obs := FreeWillObservation{
		Domain:         "task_completion",
		MetricName:     "response_generated",
		MetricValue:    float64(len(taskSummary)),
		Confidence:     0.9,
		ContextSummary: fmt.Sprintf("Completed task (response length: %d)", len(taskSummary)),
	}

	select {
	case o.FreeWillEngine.observationQueue <- obs:
	default:
		// Queue full, drop the observation
	}
}
