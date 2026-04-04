package engine

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// InterruptReason explains why an operation was interrupted
type InterruptReason int

const (
	ReasonNone InterruptReason = iota
	ReasonUserInterrupt         // User requested stop (Ctrl+C, /stop command)
	ReasonTimeout               // Operation exceeded time limit
	ReasonResourceLimit         // Token limit or cost limit exceeded
	ReasonSystemShutdown        // System shutdown initiated
	ReasonError                 // Unrecoverable error
)

func (ir InterruptReason) String() string {
	switch ir {
	case ReasonUserInterrupt:
		return "user_interrupt"
	case ReasonTimeout:
		return "timeout"
	case ReasonResourceLimit:
		return "resource_limit"
	case ReasonSystemShutdown:
		return "system_shutdown"
	case ReasonError:
		return "error"
	default:
		return "none"
	}
}

// InterruptHandler manages graceful operation interruption with clean shutdown
type InterruptHandler struct {
	mu sync.RWMutex

	// Interrupt state
	isInterrupted atomic.Bool    // Whether interrupt is requested
	interruptTime time.Time      // When interrupt was requested
	reason        InterruptReason // Why it was interrupted
	message       string          // Human-readable reason

	// Shutdown context
	cancel context.CancelFunc
	ctx    context.Context

	// Cleanup handlers (executed in reverse order)
	cleanupHandlers []func() error
	cleanupMu       sync.Mutex

	// Observability
	onInterrupt func(reason InterruptReason, message string)
	onCleanup   func(handler string, err error)
}

// NewInterruptHandler creates a new interrupt handler with timeout support
func NewInterruptHandler(timeout time.Duration) *InterruptHandler {
	ctx, cancel := context.WithCancel(context.Background())
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
	}

	ih := &InterruptHandler{
		cancel:          cancel,
		ctx:             ctx,
		cleanupHandlers: make([]func() error, 0, 10),
	}

	// Monitor context for timeout/cancellation
	go func() {
		<-ih.ctx.Done()
		// Only set as interrupted if not already set by explicit request
		if !ih.isInterrupted.Load() {
			ih.RequestInterrupt(ReasonTimeout, "context deadline exceeded")
		}
	}()

	return ih
}

// Context returns the underlying context for cancellation propagation
func (ih *InterruptHandler) Context() context.Context {
	return ih.ctx
}

// RequestInterrupt signals that an operation should stop gracefully
func (ih *InterruptHandler) RequestInterrupt(reason InterruptReason, message string) {
	if ih.isInterrupted.Load() {
		return // Already interrupted
	}

	ih.isInterrupted.Store(true)

	ih.mu.Lock()
	ih.interruptTime = time.Now()
	ih.reason = reason
	ih.message = message
	onInterrupt := ih.onInterrupt
	ih.mu.Unlock()

	// Cancel context to trigger any waiting operations
	ih.cancel()

	// Fire observability hook
	if onInterrupt != nil {
		onInterrupt(reason, message)
	}
}

// IsInterrupted returns true if an interrupt has been requested
func (ih *InterruptHandler) IsInterrupted() bool {
	return ih.isInterrupted.Load()
}

// AwaitInterrupt blocks until interrupt is requested or context is cancelled
func (ih *InterruptHandler) AwaitInterrupt() (InterruptReason, string) {
	<-ih.ctx.Done()

	ih.mu.RLock()
	defer ih.mu.RUnlock()
	return ih.reason, ih.message
}

// RegisterCleanup registers a cleanup function to be called on shutdown
// Cleanup functions are called in reverse registration order (LIFO)
func (ih *InterruptHandler) RegisterCleanup(name string, fn func() error) {
	if fn == nil {
		return
	}

	ih.cleanupMu.Lock()
	defer ih.cleanupMu.Unlock()

	// Wrap with observability
	wrapped := func() error {
		err := fn()
		if ih.onCleanup != nil {
			ih.onCleanup(name, err)
		}
		return err
	}

	ih.cleanupHandlers = append(ih.cleanupHandlers, wrapped)
}

// Shutdown executes all registered cleanup handlers and stops operations
// Returns slice of errors encountered during cleanup
func (ih *InterruptHandler) Shutdown() []error {
	// Request interrupt if not already requested
	if !ih.isInterrupted.Load() {
		ih.RequestInterrupt(ReasonSystemShutdown, "graceful shutdown")
	}

	ih.cleanupMu.Lock()
	handlers := make([]func() error, len(ih.cleanupHandlers))
	copy(handlers, ih.cleanupHandlers)
	ih.cleanupMu.Unlock()

	// Execute handlers in reverse order (LIFO)
	var errors []error
	for i := len(handlers) - 1; i >= 0; i-- {
		if err := handlers[i](); err != nil {
			errors = append(errors, err)
		}
	}

	return errors
}

// InterruptRequest encapsulates a request to stop execution
type InterruptRequest struct {
	Reason    InterruptReason
	Message   string
	Timestamp time.Time
	StackAt   error // Optional: stack trace for debugging
}

// StreamingInterruptAware wraps streaming operations with interrupt checks
func (ih *InterruptHandler) StreamingInterruptAware(
	onToken func(token string) error,
) func(token string) error {
	if onToken == nil {
		return nil
	}

	return func(token string) error {
		// Check for interrupt before processing token
		if ih.IsInterrupted() {
			return context.Canceled
		}

		// Process token
		if err := onToken(token); err != nil {
			return err
		}

		// Check again after processing
		select {
		case <-ih.ctx.Done():
			return context.Canceled
		default:
			return nil
		}
	}
}

// SetInterruptCallback sets the observability hook for interrupt events
func (ih *InterruptHandler) SetInterruptCallback(fn func(reason InterruptReason, message string)) {
	ih.mu.Lock()
	defer ih.mu.Unlock()
	ih.onInterrupt = fn
}

// SetCleanupCallback sets the observability hook for cleanup events
func (ih *InterruptHandler) SetCleanupCallback(fn func(handler string, err error)) {
	ih.cleanupMu.Lock()
	defer ih.cleanupMu.Unlock()
	ih.onCleanup = fn
}

// StatusReport returns information about interrupt state for diagnostics
func (ih *InterruptHandler) StatusReport() string {
	ih.mu.RLock()
	defer ih.mu.RUnlock()

	if !ih.isInterrupted.Load() {
		return "No interrupt requested"
	}

	elapsed := time.Since(ih.interruptTime)
	return "Interrupt: " + ih.reason.String() + " | " + ih.message + " (" + formatTimeSince(elapsed) + " ago)"
}

