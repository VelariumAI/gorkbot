package selfimprove

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// Driver is the central coordinator for self-improvement autonomous operations.
type Driver struct {
	// Facades
	spark    SPARKFacade
	freeWill FreeWillFacade
	harness  HarnessFacade
	research ResearchFacade
	registry ToolRegistryFacade
	notify   NotifyFacade

	// Components
	motivator *Motivator
	heartbeat *AdaptiveHeartbeat
	planner   *ImprovementPlanner

	// State
	mu            sync.RWMutex
	enabled       atomic.Bool
	lastCycle     *ImproveCycle
	cycleCount    int64
	cancelFn      context.CancelFunc
	isRunning     atomic.Bool
	activePhase   atomic.Value   // stores string
	lastCandidate *CandidateInfo // protected by mu
	cycleHistory  []ImproveCycle // ring buffer, last 5; protected by mu
	lastSignals   SignalSnapshot // last computed signals; protected by mu
	evolvePhaseCB func()         // called on any phase change (for fast TUI refresh)
	pipeline      *SIPipeline    // if set, executeCycle() delegates to unified pipeline

	// Tracking
	logger *slog.Logger
}

// NewDriver creates a new self-improvement driver.
func NewDriver(spark SPARKFacade, fw FreeWillFacade, h HarnessFacade,
	res ResearchFacade, reg ToolRegistryFacade, n NotifyFacade, logger *slog.Logger) *Driver {

	if logger == nil {
		logger = slog.Default()
	}

	return &Driver{
		spark:     spark,
		freeWill:  fw,
		harness:   h,
		research:  res,
		registry:  reg,
		notify:    n,
		motivator: NewMotivator(0.15), // alpha = 0.15 (slow-moving)
		heartbeat: NewAdaptiveHeartbeat(),
		planner:   NewImprovementPlanner(),
		logger:    logger,
	}
}

// Start begins the self-improvement autonomous loop.
func (d *Driver) Start(ctx context.Context) error {
	if d.enabled.Load() {
		return fmt.Errorf("driver already running")
	}

	d.enabled.Store(true)
	d.heartbeat.Adapt(ModeCalm)
	d.notify.Notify("✨ Self-improvement drive started (CALM mode)")

	// Create cancelable context
	cancelCtx, cancel := context.WithCancel(ctx)
	d.cancelFn = cancel

	// Start the main loop
	go d.runLoop(cancelCtx)
	return nil
}

// Stop halts the self-improvement loop.
func (d *Driver) Stop() {
	if !d.enabled.Load() {
		return
	}

	d.enabled.Store(false)
	if d.cancelFn != nil {
		d.cancelFn()
	}
	d.heartbeat.Stop()
	d.notify.Notify("⏸ Self-improvement drive paused")
}

// Toggle switches the drive on/off. Returns the new enabled state.
func (d *Driver) Toggle(ctx context.Context) bool {
	if d.enabled.Load() {
		d.Stop()
		return false
	}
	_ = d.Start(ctx)
	return true
}

// Snapshot returns a point-in-time snapshot of SI state.
func (d *Driver) Snapshot() SISnapshot {
	d.mu.RLock()
	cand := d.lastCandidate
	hist := append([]ImproveCycle{}, d.cycleHistory...)
	sigs := d.lastSignals
	d.mu.RUnlock()

	mode := ModeCalm
	driveScore := 0.0
	rawScore := 0.0

	// Compute current mode and score from latest signals
	sparkState := d.spark.GetLastState()
	if sparkState != nil {
		signals := &SignalSnapshot{
			SPARKDriveScore:       sparkState.DriveScore,
			SPARKActiveDirectives: sparkState.ActiveDirectives,
			SPARKIDLDebt:          sparkState.IDLDebt,
		}
		mode = d.motivator.Update(signals)
		driveScore = d.motivator.GetScore()
		rawScore = d.motivator.GetLastRaw()
	}

	nextHeartbeat := d.heartbeat.NextTickTime()

	// Safely load activePhase (atomic.Value returns nil if not yet initialized)
	activePhaseVal := d.activePhase.Load()
	activePhase := ""
	if activePhaseVal != nil {
		activePhase = activePhaseVal.(string)
	}

	return SISnapshot{
		Enabled:        d.enabled.Load(),
		Mode:           mode,
		DriveScore:     driveScore,
		RawScore:       rawScore,
		LastCycle:      d.lastCycle,
		NextHeartbeat:  nextHeartbeat,
		PendingSignals: d.countPendingSignals(),
		CycleCount:     d.cycleCount,
		Signals:        sigs,
		IsRunning:      d.isRunning.Load(),
		ActivePhase:    activePhase,
		LastCandidate:  cand,
		CycleHistory:   hist,
	}
}

// runLoop is the main autonomous loop that fires on each heartbeat.
func (d *Driver) runLoop(ctx context.Context) {
	ticker := d.heartbeat.C()

	for d.enabled.Load() {
		select {
		case <-ctx.Done():
			d.enabled.Store(false)
			return
		case <-ticker:
			d.executeCycle(ctx)
		}
	}
}

// executeCycle runs one improvement cycle: gather signals, select, execute.
func (d *Driver) executeCycle(ctx context.Context) {
	if !d.isRunning.CompareAndSwap(false, true) {
		d.logger.Debug("SI cycle skipped: already running")
		return
	}
	cycleID := uuid.New().String()[:8]
	startTime := time.Now()

	// Mark as running
	d.setPhase("selecting")
	defer func() {
		d.isRunning.Store(false)
		d.setPhase("")
	}()

	// Delegate to unified pipeline if wired (Task 4.2)
	d.mu.RLock()
	pipeline := d.pipeline
	d.mu.RUnlock()
	if pipeline != nil {
		accepted, err := pipeline.Run(ctx)
		if err != nil {
			d.logger.Warn("SI pipeline error", "error", err)
			return
		}
		if accepted {
			d.mu.Lock()
			d.cycleCount++
			d.mu.Unlock()
		}
		return
	}

	// 1. Gather all signals
	sparkState := d.spark.GetLastState()
	if sparkState == nil {
		return // Not ready yet
	}

	signals := &SignalSnapshot{
		SPARKDriveScore:          sparkState.DriveScore,
		SPARKActiveDirectives:    sparkState.ActiveDirectives,
		SPARKIDLDebt:             sparkState.IDLDebt,
		HarnessFailing:           d.harness.FailingCount(),
		HarnessTotal:             d.harness.TotalCount(),
		FreeWillProposalsPending: len(d.freeWill.GetPendingProposals()),
		ResearchBufferedDocs:     d.research.BufferedCount(),
	}

	// 2. Update motivator, get new mode
	mode := d.motivator.Update(signals)
	d.heartbeat.Adapt(mode)

	// Store last signals
	d.mu.Lock()
	d.lastSignals = *signals
	d.mu.Unlock()

	// 3. Feed SPARK state as FreeWill observation
	d.feedFreeWillObservation(ctx, sparkState)

	// 4. Plan the best action
	candidate := d.selectCandidate(mode)
	if candidate == nil {
		return
	}

	// Update phase and store candidate
	d.setPhase("executing:" + candidate.Target)
	d.mu.Lock()
	d.lastCandidate = &CandidateInfo{
		Source:    candidate.Source,
		Target:    candidate.Target,
		BaseScore: signals.SPARKDriveScore,
	}
	d.mu.Unlock()

	// 5. Execute and record outcome
	outcome := "success"
	err := d.executeCandidate(ctx, candidate)
	if err != nil {
		outcome = "failed"
		d.logger.Warn("SI cycle execution failed", "cycle", cycleID, "error", err)
	}

	// Update phase to verifying
	d.setPhase("verifying")

	// 6. Record cycle
	cycle := &ImproveCycle{
		ID:        cycleID,
		StartedAt: startTime,
		Source:    candidate.Source,
		Target:    candidate.Target,
		Mode:      mode,
		Outcome:   outcome,
		Duration:  time.Since(startTime),
	}

	d.mu.Lock()
	d.lastCycle = cycle
	d.cycleCount++
	// Append to history (ring buffer, max 5)
	d.cycleHistory = append(d.cycleHistory, *cycle)
	if len(d.cycleHistory) > 5 {
		d.cycleHistory = d.cycleHistory[len(d.cycleHistory)-5:]
	}
	d.mu.Unlock()

	d.notify.Notify(fmt.Sprintf("🔄 %s: %s %s (%s)",
		cycleID, candidate.Source.String(), candidate.Target, outcome))
}

// RunCycleNow triggers one immediate self-improvement cycle and returns
// the latest snapshot after the attempt.
func (d *Driver) RunCycleNow(ctx context.Context) SISnapshot {
	d.executeCycle(ctx)
	return d.Snapshot()
}

// setPhase updates the active phase and calls the callback if set.
func (d *Driver) setPhase(phase string) {
	d.activePhase.Store(phase)
	d.mu.RLock()
	cb := d.evolvePhaseCB
	d.mu.RUnlock()
	if cb != nil {
		cb()
	}
}

// selectCandidate gathers candidates and returns the best scoring one.
func (d *Driver) selectCandidate(mode EmotionalMode) *ImprovementCandidate {
	// Gather candidates from all sources
	var sparkDirs []string
	if d.spark.GetLastState() != nil {
		// For now, just use empty (real integration would parse directives)
		sparkDirs = make([]string, 0)
	}

	fwProps := d.freeWill.GetPendingProposals()

	harnessInfo := &HarnessFailureInfo{
		FailingCount: d.harness.FailingCount(),
		TotalCount:   d.harness.TotalCount(),
	}

	researchDocs := d.research.BufferedCount()

	// Plan and return best candidate
	return d.planner.Select(mode, sparkDirs, fwProps, harnessInfo, researchDocs)
}

// executeCandidate runs the improvement action.
func (d *Driver) executeCandidate(ctx context.Context, cand *ImprovementCandidate) error {
	params := make(map[string]interface{})

	// Route to appropriate tool based on source
	var toolName string
	switch cand.Source {
	case SourceSPARK:
		toolName = "harness_boot"
		params["feature"] = cand.Target
	case SourceFreeWill:
		toolName = "harness_select"
		params["feature_id"] = cand.Target
	case SourceHarness:
		toolName = "harness_complete"
		params["feature_id"] = d.harness.ActiveFeatureID()
	case SourceResearch:
		toolName = "browser_search"
		params["query"] = cand.Target
	default:
		return fmt.Errorf("unknown source: %v", cand.Source)
	}

	_, err := d.registry.ExecuteTool(ctx, toolName, params)
	return err
}

// feedFreeWillObservation sends SPARK state as an observation to FreeWill.
func (d *Driver) feedFreeWillObservation(ctx context.Context, sparkState *SPARKStateSnapshot) {
	obs := FreeWillObsInput{
		Context:    fmt.Sprintf("SPARK drive_score=%.2f directives=%d idl_debt=%d", sparkState.DriveScore, sparkState.ActiveDirectives, sparkState.IDLDebt),
		ToolName:   "spark_daemon",
		Outcome:    "observation",
		Latency:    0,
		Confidence: 0.85,
	}
	_ = d.freeWill.AddObservation(ctx, obs)
}

// SetPhaseCallback registers a callback to be called on every phase change.
func (d *Driver) SetPhaseCallback(cb func()) {
	d.mu.Lock()
	d.evolvePhaseCB = cb
	d.mu.Unlock()
}

// SetPipeline wires a unified 7-stage pipeline. When set, executeCycle()
// delegates to pipeline.Run() instead of the built-in 6-step logic.
func (d *Driver) SetPipeline(p *SIPipeline) {
	p.SetPhaseFunc(d.setPhase)
	d.mu.Lock()
	d.pipeline = p
	d.mu.Unlock()
}

// countPendingSignals returns the total number of pending improvement signals.
func (d *Driver) countPendingSignals() int {
	count := 0

	sparkState := d.spark.GetLastState()
	if sparkState != nil && sparkState.ActiveDirectives > 0 {
		count += sparkState.ActiveDirectives
	}

	count += len(d.freeWill.GetPendingProposals())
	count += d.harness.FailingCount()
	// Don't count research docs as urgent signals

	return count
}
