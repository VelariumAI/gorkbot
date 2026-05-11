package governance

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

const defaultMaxInflightApprovals = 4

// InflightApproval represents a single in-flight approval worker.
type InflightApproval struct {
	Key       string
	StartedAt time.Time

	done   chan struct{}
	result ApprovalResult
	err    error
}

// ApprovalRuntime bounds and coordinates human approval workers.
type ApprovalRuntime struct {
	mu           sync.Mutex
	shuttingDown bool
	shutdownCh   chan struct{}
	maxInflight  int
	sem          chan struct{}
	inflight     map[string]*InflightApproval
}

// NewApprovalRuntime builds an approval runtime with bounded worker capacity.
func NewApprovalRuntime(maxInflight int) *ApprovalRuntime {
	normalized := normalizeMaxInflight(maxInflight)
	return &ApprovalRuntime{
		shutdownCh:  make(chan struct{}),
		maxInflight: normalized,
		sem:         make(chan struct{}, normalized),
		inflight:    make(map[string]*InflightApproval),
	}
}

// Shutdown marks runtime as shutting down and releases waiters.
func (r *ApprovalRuntime) Shutdown() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.shuttingDown {
		r.mu.Unlock()
		return
	}
	r.shuttingDown = true
	close(r.shutdownCh)
	r.mu.Unlock()
}

// IsShutdown reports whether shutdown has been requested.
func (r *ApprovalRuntime) IsShutdown() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.shuttingDown
}

// MaxInflight returns the configured in-flight approval limit.
func (r *ApprovalRuntime) MaxInflight() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.maxInflight
}

// InflightCount returns the current number of in-flight approval keys.
func (r *ApprovalRuntime) InflightCount() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.inflight)
}

// Run executes fn in a bounded worker, deduplicated by key.
func (r *ApprovalRuntime) Run(ctx context.Context, key string, fn func() (ApprovalResult, error)) (ApprovalResult, error) {
	if r == nil {
		return ApprovalResult{
			Decision: APPROVAL_UNAVAILABLE,
			Scope:    APPROVAL_ONCE,
			Reason:   REASON_APPROVAL_RUNTIME_SHUTDOWN,
		}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if fn == nil {
		return ApprovalResult{
			Decision: APPROVAL_UNAVAILABLE,
			Scope:    APPROVAL_ONCE,
			Reason:   "approval function unavailable",
		}, nil
	}
	if key == "" {
		key = "__approval__"
	}

	for {
		r.mu.Lock()
		if r.shuttingDown {
			r.mu.Unlock()
			return ApprovalResult{
				Decision: APPROVAL_CANCELLED,
				Scope:    APPROVAL_ONCE,
				Reason:   REASON_APPROVAL_RUNTIME_SHUTDOWN,
			}, nil
		}
		if existing := r.inflight[key]; existing != nil {
			shutdownCh := r.shutdownCh
			r.mu.Unlock()
			return r.waitForResult(ctx, shutdownCh, existing, true)
		}
		shutdownCh := r.shutdownCh
		r.mu.Unlock()

		select {
		case r.sem <- struct{}{}:
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return ApprovalResult{
					Decision: APPROVAL_TIMEOUT,
					Scope:    APPROVAL_ONCE,
					Reason:   REASON_APPROVAL_INFLIGHT_LIMIT,
				}, nil
			}
			return ApprovalResult{
				Decision: APPROVAL_CANCELLED,
				Scope:    APPROVAL_ONCE,
				Reason:   REASON_HUMAN_APPROVAL_CANCELLED,
			}, nil
		case <-shutdownCh:
			return ApprovalResult{
				Decision: APPROVAL_CANCELLED,
				Scope:    APPROVAL_ONCE,
				Reason:   REASON_APPROVAL_RUNTIME_SHUTDOWN,
			}, nil
		}

		r.mu.Lock()
		if r.shuttingDown {
			r.mu.Unlock()
			r.releaseSlot()
			return ApprovalResult{
				Decision: APPROVAL_CANCELLED,
				Scope:    APPROVAL_ONCE,
				Reason:   REASON_APPROVAL_RUNTIME_SHUTDOWN,
			}, nil
		}
		if existing := r.inflight[key]; existing != nil {
			r.mu.Unlock()
			r.releaseSlot()
			return r.waitForResult(ctx, shutdownCh, existing, true)
		}

		inflight := &InflightApproval{
			Key:       key,
			StartedAt: time.Now(),
			done:      make(chan struct{}),
		}
		r.inflight[key] = inflight
		r.mu.Unlock()

		go r.runInflight(inflight, fn)
		return r.waitForResult(ctx, shutdownCh, inflight, false)
	}
}

func (r *ApprovalRuntime) waitForResult(ctx context.Context, shutdownCh <-chan struct{}, inflight *InflightApproval, joined bool) (ApprovalResult, error) {
	select {
	case <-inflight.done:
		result := inflight.result
		if joined && result.Reason == "" {
			result.Reason = REASON_APPROVAL_DUPLICATE_JOINED
		}
		return result, inflight.err
	case <-ctx.Done():
		return approvalFromContext(ctx)
	case <-shutdownCh:
		return ApprovalResult{
			Decision: APPROVAL_CANCELLED,
			Scope:    APPROVAL_ONCE,
			Reason:   REASON_APPROVAL_RUNTIME_SHUTDOWN,
		}, nil
	}
}

func (r *ApprovalRuntime) runInflight(inflight *InflightApproval, fn func() (ApprovalResult, error)) {
	defer func() {
		if rec := recover(); rec != nil {
			inflight.result = ApprovalResult{
				Decision: APPROVAL_UNAVAILABLE,
				Scope:    APPROVAL_ONCE,
				Reason:   fmt.Sprintf("approval runtime panic: %v", rec),
			}
			inflight.err = nil
		}
		close(inflight.done)

		r.mu.Lock()
		delete(r.inflight, inflight.Key)
		r.mu.Unlock()
		r.releaseSlot()
	}()

	inflight.result, inflight.err = fn()
}

func (r *ApprovalRuntime) releaseSlot() {
	// Defensive no-op when the semaphore is already empty.
	// Normal paths acquire once and release exactly once.
	select {
	case <-r.sem:
	default:
	}
}

func normalizeMaxInflight(maxInflight int) int {
	if maxInflight <= 0 {
		return defaultMaxInflightApprovals
	}
	return maxInflight
}

func approvalFromContext(ctx context.Context) (ApprovalResult, error) {
	err := ctx.Err()
	if errors.Is(err, context.DeadlineExceeded) {
		return ApprovalResult{
			Decision: APPROVAL_TIMEOUT,
			Scope:    APPROVAL_ONCE,
			Reason:   REASON_HUMAN_APPROVAL_TIMEOUT,
		}, nil
	}
	return ApprovalResult{
		Decision: APPROVAL_CANCELLED,
		Scope:    APPROVAL_ONCE,
		Reason:   REASON_HUMAN_APPROVAL_CANCELLED,
	}, nil
}
