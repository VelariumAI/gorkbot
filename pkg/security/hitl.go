package security

// hitl.go — Synchronous Human-In-the-Loop (HITL) with buffered channel patterns.
//
// This module provides a synchronous blocking HITL mechanism that prevents goroutine
// leaks by using select statements with context cancellation awareness. When an
// agent triggers a restricted action, it halts via a select statement waiting
// for user approval through the TUI.
//
// Key features:
//   - Buffered channels to prevent goroutine leaks
//   - Context-aware cancellation support
//   - FIFO queue for multiple pending requests
//   - Clean timeout handling

import (
	"context"
	"fmt"
	"time"
)

// HITLRequest represents a pending human approval request.
type HITLRequest struct {
	ID         string                 // Unique identifier for this request
	Action     string                 // Action type (e.g., "bash", "delete_file")
	Details    string                 // Human-readable details
	Params     map[string]interface{} // Parameters for the action
	Timestamp  time.Time              // When the request was created
	ResultChan chan HITLResult        // Channel to send the result back
}

// HITLResult represents the result of a HITL approval decision.
type HITLResult struct {
	Approved bool   // Whether the action was approved
	Notes    string // Optional notes (for amended approvals)
	Error    error  // Error if something went wrong
}

// HITLManager manages the synchronous HITL approval flow.
type HITLManager struct {
	// Channel for sending requests to the UI
	RequestChan chan HITLRequest

	// Queue for pending requests when UI is busy
	requestQueue []HITLRequest

	// Channel to signal UI shutdown
	shutdownChan chan struct{}

	// Maximum queue size before rejecting new requests
	maxQueueSize int

	// Default timeout for approval
	defaultTimeout time.Duration
}

// NewHITLManager creates a new HITL manager with buffered channels.
func NewHITLManager() *HITLManager {
	return &HITLManager{
		RequestChan:    make(chan HITLRequest, 1), // Buffered to prevent leaks
		requestQueue:   make([]HITLRequest, 0),
		shutdownChan:   make(chan struct{}),
		maxQueueSize:   10,
		defaultTimeout: 5 * time.Minute,
	}
}

// RequestApproval submits an approval request and blocks until a decision is received.
// This is the synchronous API that agents use to request human approval.
//
// The function uses a select statement to handle:
//   - Context cancellation
//   - Channel receiving
//   - Timeout expiration
func (hm *HITLManager) RequestApproval(ctx context.Context, action, details string, params map[string]interface{}) (bool, string, error) {
	resultCh := make(chan HITLResult, 1) // Buffered to prevent goroutine leak

	req := HITLRequest{
		ID:         fmt.Sprintf("hitl_%d", time.Now().UnixNano()),
		Action:     action,
		Details:    details,
		Params:     params,
		Timestamp:  time.Now(),
		ResultChan: resultCh,
	}

	// Send request to UI channel with context-aware select
	select {
	case hm.RequestChan <- req:
		// Request successfully queued
	case <-ctx.Done():
		return false, "", ctx.Err()
	case <-hm.shutdownChan:
		return false, "", fmt.Errorf("HITL manager shutdown")
	}

	// Wait for response with timeout
	select {
	case result := <-resultCh:
		return result.Approved, result.Notes, result.Error
	case <-ctx.Done():
		// Context cancelled - try to clean up
		go func() {
			select {
			case _, ok := <-resultCh:
				if ok {
					close(resultCh)
				}
			default:
			}
		}()
		return false, "", ctx.Err()
	}
}

// RequestApprovalWithTimeout submits an approval request with a specific timeout.
func (hm *HITLManager) RequestApprovalWithTimeout(action, details string, params map[string]interface{}, timeout time.Duration) (bool, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return hm.RequestApproval(ctx, action, details, params)
}

// QueueRequest adds a request to the queue when UI is busy processing another request.
func (hm *HITLManager) QueueRequest(req HITLRequest) error {
	if len(hm.requestQueue) >= hm.maxQueueSize {
		return fmt.Errorf("HITL queue full, rejecting request: %s", req.Action)
	}
	hm.requestQueue = append(hm.requestQueue, req)
	return nil
}

// GetNextQueuedRequest returns the next request from the queue.
func (hm *HITLManager) GetNextQueuedRequest() *HITLRequest {
	if len(hm.requestQueue) == 0 {
		return nil
	}
	req := hm.requestQueue[0]
	hm.requestQueue = hm.requestQueue[1:]
	return &req
}

// QueueLength returns the current queue length.
func (hm *HITLManager) QueueLength() int {
	return len(hm.requestQueue)
}

// Shutdown signals the HITL manager to shut down and cleans up pending requests.
func (hm *HITLManager) Shutdown() error {
	close(hm.shutdownChan)

	// Drain the queue
	for _, req := range hm.requestQueue {
		req.ResultChan <- HITLResult{
			Approved: false,
			Notes:    "",
			Error:    fmt.Errorf("HITL manager shutdown"),
		}
	}
	hm.requestQueue = nil

	return nil
}

// IsQueueFull returns true if the queue is at capacity.
func (hm *HITLManager) IsQueueFull() bool {
	return len(hm.requestQueue) >= hm.maxQueueSize
}

// SetMaxQueueSize sets the maximum queue size.
func (hm *HITLManager) SetMaxQueueSize(size int) {
	if size > 0 {
		hm.maxQueueSize = size
	}
}

// SetDefaultTimeout sets the default timeout for approval requests.
func (hm *HITLManager) SetDefaultTimeout(timeout time.Duration) {
	if timeout > 0 {
		hm.defaultTimeout = timeout
	}
}
