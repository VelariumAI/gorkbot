package dag

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// DefaultConcurrency is the maximum number of tasks that run in parallel.
	// Chosen to saturate a typical mobile SoC (S23 Ultra = 8 cores) without
	// overwhelming it with context-switch overhead.
	DefaultConcurrency = 4

	// DefaultRCATimeout is how long we wait for the AI to analyze a failure.
	// Kept short so a broken API key doesn't stall the entire pipeline.
	DefaultRCATimeout = 15 * time.Second
)

// Resolver drives the parallel execution of a Graph.
// It continuously finds tasks whose dependencies are satisfied and dispatches
// them to goroutines up to the concurrency limit.
type Resolver struct {
	graph       *Graph
	env         *Environment
	concurrency int

	// running tracks how many goroutines are currently executing tasks.
	running int32
	// sem is the goroutine pool semaphore (buffered channel pattern).
	sem chan struct{}

	mu      sync.Mutex
	waiters []chan struct{} // goroutines blocked on "wait for a slot"
}

// ResolverOption configures the Resolver.
type ResolverOption func(*Resolver)

// WithConcurrency sets the maximum number of parallel task goroutines.
func WithConcurrency(n int) ResolverOption {
	return func(r *Resolver) {
		if n > 0 {
			r.concurrency = n
			r.sem = make(chan struct{}, n)
		}
	}
}

// NewResolver creates a Resolver for the given Graph and Environment.
func NewResolver(g *Graph, env *Environment, opts ...ResolverOption) *Resolver {
	r := &Resolver{
		graph:       g,
		env:         env,
		concurrency: DefaultConcurrency,
	}
	for _, opt := range opts {
		opt(r)
	}
	r.sem = make(chan struct{}, r.concurrency)
	return r
}

// Run executes the graph to completion and blocks until all tasks have
// reached a terminal state (Completed, Failed, or Skipped).
//
// Execution strategy:
//  1. Validate the graph (cycle check, unknown deps).
//  2. Each scheduling round finds all tasks whose dependencies are fully
//     satisfied (all Completed) and dispatches them in parallel.
//  3. On failure: run RCA → retry with exponential back-off → skip dependents
//     if max retries exceeded.
//  4. The context's Done channel is respected at every yield point so Ctrl+C
//     causes a graceful shutdown within one scheduling cycle.
func (r *Resolver) Run(ctx context.Context) error {
	// Validate topology before starting any work.
	if _, err := r.graph.TopologicalOrder(); err != nil {
		return err
	}

	var wg sync.WaitGroup

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Find tasks that are ready to run (all deps completed, not yet started).
		ready := r.readyTasks()
		if len(ready) == 0 {
			// No ready tasks — check if we are done or deadlocked.
			if r.allTerminal() {
				break
			}
			// Tasks still running; yield and re-evaluate.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(50 * time.Millisecond):
				continue
			}
		}

		// Dispatch each ready task into its own goroutine.
		for _, t := range ready {
			r.graph.setStatus(t.ID, StatusQueued, "queued", "")

			wg.Add(1)
			atomic.AddInt32(&r.running, 1)

			go func(task *Task) {
				defer wg.Done()
				defer atomic.AddInt32(&r.running, -1)

				// Acquire semaphore slot (blocks if at max concurrency).
				select {
				case r.sem <- struct{}{}:
				case <-ctx.Done():
					r.graph.setStatus(task.ID, StatusSkipped, "context cancelled", "")
					return
				}
				defer func() { <-r.sem }()

				r.executeTask(ctx, task)
			}(t)
		}
	}

	wg.Wait()

	// Collect any terminal failures.
	return r.summariseFailures()
}

// executeTask runs a single task, applying retry + RCA on failure.
func (r *Resolver) executeTask(ctx context.Context, t *Task) {
	for attempt := 0; ; attempt++ {
		if ctx.Err() != nil {
			r.graph.setStatus(t.ID, StatusSkipped, "context cancelled", "")
			return
		}

		r.graph.setStatus(t.ID, StatusRunning,
			fmt.Sprintf("attempt %d", attempt+1), "")

		result, err := r.runAction(ctx, t)

		if err == nil {
			// Success.
			r.graph.mu.Lock()
			t.Result = result
			t.Progress = 1.0
			r.graph.mu.Unlock()

			r.graph.setStatus(t.ID, StatusCompleted, "completed", "")
			return
		}

		// ── Failure path ──────────────────────────────────────────────────
		errMsg := err.Error()

		// Context pruning: compress the error before feeding it to RCA.
		compressed := r.env.Pruner.Compress(errMsg)

		// AI Root Cause Analysis — run in a time-bounded child context.
		rcaReport := ""
		if r.env.AI != nil {
			rcaCtx, cancel := context.WithTimeout(ctx, DefaultRCATimeout)
			report, rcaErr := r.env.AI.Analyze(rcaCtx, t.Description, compressed)
			cancel()
			if rcaErr == nil {
				rcaReport = report
			}
		}

		r.graph.mu.Lock()
		t.RetryCount = attempt + 1
		t.RCAReport = rcaReport
		t.ErrMsg = errMsg
		r.graph.mu.Unlock()

		// Decide whether to retry.
		maxR := t.MaxRetries
		if maxR == 0 {
			break // No retries configured.
		}
		if maxR > 0 && attempt >= maxR {
			break // Retry budget exhausted.
		}

		// Exponential back-off: base * 2^attempt, capped at 30s.
		base := t.RetryDelay
		if base <= 0 {
			base = time.Second
		}
		delay := time.Duration(float64(base) * math.Pow(2, float64(attempt)))
		if delay > 30*time.Second {
			delay = 30 * time.Second
		}

		r.graph.emit(Event{
			GraphID: r.graph.ID,
			TaskID:  t.ID,
			Status:  StatusFailed,
			Message: fmt.Sprintf("retry %d/%d in %s — RCA: %s", attempt+1, maxR, delay.Round(time.Millisecond), shortRCA(rcaReport)),
			ErrMsg:  errMsg,
		})

		select {
		case <-ctx.Done():
			r.graph.setStatus(t.ID, StatusSkipped, "context cancelled during retry delay", "")
			return
		case <-time.After(delay):
		}
	}

	// All retries exhausted (or no retries configured) — mark failed.
	r.graph.setStatus(t.ID, StatusFailed, "all retries exhausted", t.ErrMsg)

	// Mark every task that depends on this one as Skipped.
	r.skipDependents(t.ID)
}

// runAction calls t.Action with a panic recovery guard.
func (r *Resolver) runAction(ctx context.Context, t *Task) (result interface{}, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("panic in task %q: %v", t.ID, rec)
		}
	}()

	if t.Action == nil {
		return nil, fmt.Errorf("task %q has no Action defined", t.ID)
	}
	return t.Action(ctx, r.env)
}

// readyTasks returns all tasks that are Pending and whose every dependency
// has reached StatusCompleted.
func (r *Resolver) readyTasks() []*Task {
	r.graph.mu.RLock()
	defer r.graph.mu.RUnlock()

	var ready []*Task
	for _, t := range r.graph.tasks {
		if t.Status != StatusPending {
			continue
		}
		if r.depsCompleted(t) {
			ready = append(ready, t)
		}
	}
	return ready
}

// depsCompleted reports whether all of t's dependencies are Completed.
// Must be called with g.mu at least read-locked.
func (r *Resolver) depsCompleted(t *Task) bool {
	for _, depID := range t.Dependencies {
		dep, ok := r.graph.tasks[depID]
		if !ok || dep.Status != StatusCompleted {
			return false
		}
	}
	return true
}

// allTerminal reports whether every task has reached a terminal state.
func (r *Resolver) allTerminal() bool {
	r.graph.mu.RLock()
	defer r.graph.mu.RUnlock()
	for _, t := range r.graph.tasks {
		switch t.Status {
		case StatusCompleted, StatusFailed, StatusSkipped, StatusRolledBack:
		default:
			return false
		}
	}
	return true
}

// skipDependents marks every task that transitively depends on failedID as Skipped.
func (r *Resolver) skipDependents(failedID string) {
	r.graph.mu.RLock()
	var toSkip []string
	for _, t := range r.graph.tasks {
		if t.Status == StatusPending {
			for _, dep := range t.Dependencies {
				if dep == failedID {
					toSkip = append(toSkip, t.ID)
					break
				}
			}
		}
	}
	r.graph.mu.RUnlock()

	for _, id := range toSkip {
		r.graph.setStatus(id, StatusSkipped,
			fmt.Sprintf("dependency %q failed", failedID), "")
		// Recurse so transitive dependents are also skipped.
		r.skipDependents(id)
	}
}

// summariseFailures returns an aggregated error if any tasks failed.
func (r *Resolver) summariseFailures() error {
	r.graph.mu.RLock()
	defer r.graph.mu.RUnlock()

	var failed []string
	for _, t := range r.graph.tasks {
		if t.Status == StatusFailed {
			failed = append(failed, fmt.Sprintf("%s: %s", t.ID, t.ErrMsg))
		}
	}
	if len(failed) == 0 {
		return nil
	}
	msg := fmt.Sprintf("dag: %d task(s) failed", len(failed))
	for _, f := range failed {
		msg += "\n  • " + f
	}
	return fmt.Errorf("%s", msg)
}

// RollbackAll calls each task's Rollback function in reverse topological order.
// Useful for undoing partial progress after a graph failure.
func (r *Resolver) RollbackAll(ctx context.Context) []error {
	order, err := r.graph.TopologicalOrder()
	if err != nil {
		return []error{err}
	}

	var errs []error
	// Iterate in reverse order: undo leaf tasks first.
	for i := len(order) - 1; i >= 0; i-- {
		t := order[i]
		if t.Status != StatusCompleted || t.Rollback == nil {
			continue
		}
		if rbErr := t.Rollback(ctx, r.env); rbErr != nil {
			errs = append(errs, fmt.Errorf("rollback %q: %w", t.ID, rbErr))
		} else {
			r.graph.setStatus(t.ID, StatusRolledBack, "rolled back", "")
		}
	}
	return errs
}

// shortRCA trims an RCA report to a single line for inline display.
func shortRCA(report string) string {
	if report == "" {
		return "no AI analysis available"
	}
	if len(report) > 80 {
		return report[:77] + "..."
	}
	return report
}
