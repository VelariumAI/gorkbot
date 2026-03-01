package tools

import (
	"context"
	"sync"
	"time"
)

// DispatchResult pairs a ToolRequest with its ToolResult for ordered collection.
type DispatchResult struct {
	Request ToolRequest
	Result  *ToolResult
	Err     error
	Elapsed time.Duration
}

// Dispatcher runs independent ToolRequests concurrently and returns results
// in the same order as the input slice.
type Dispatcher struct {
	registry   *Registry
	maxWorkers int
}

// NewDispatcher creates a Dispatcher backed by the given registry.
// maxWorkers caps concurrent executions; 0 means unbounded.
func NewDispatcher(registry *Registry, maxWorkers int) *Dispatcher {
	if maxWorkers <= 0 {
		maxWorkers = 8
	}
	return &Dispatcher{registry: registry, maxWorkers: maxWorkers}
}

// Dispatch executes all requests concurrently (up to maxWorkers at a time)
// and returns results in the original request order.
func (d *Dispatcher) Dispatch(ctx context.Context, reqs []ToolRequest) []DispatchResult {
	n := len(reqs)
	results := make([]DispatchResult, n)

	sem := make(chan struct{}, d.maxWorkers)
	var wg sync.WaitGroup

	for i, req := range reqs {
		wg.Add(1)
		go func(idx int, r ToolRequest) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			start := time.Now()
			result, err := d.registry.Execute(ctx, &r)
			elapsed := time.Since(start)

			results[idx] = DispatchResult{
				Request: r,
				Result:  result,
				Err:     err,
				Elapsed: elapsed,
			}
		}(i, req)
	}

	wg.Wait()
	return results
}

// DispatchWithProgress executes requests concurrently and calls onProgress
// after each tool completes (useful for live TUI updates).
func (d *Dispatcher) DispatchWithProgress(
	ctx context.Context,
	reqs []ToolRequest,
	onProgress func(idx int, dr DispatchResult),
) []DispatchResult {
	n := len(reqs)
	results := make([]DispatchResult, n)

	sem := make(chan struct{}, d.maxWorkers)
	var wg sync.WaitGroup

	for i, req := range reqs {
		wg.Add(1)
		go func(idx int, r ToolRequest) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			start := time.Now()
			result, err := d.registry.Execute(ctx, &r)
			elapsed := time.Since(start)

			dr := DispatchResult{
				Request: r,
				Result:  result,
				Err:     err,
				Elapsed: elapsed,
			}
			results[idx] = dr

			if onProgress != nil {
				onProgress(idx, dr)
			}
		}(i, req)
	}

	wg.Wait()
	return results
}
