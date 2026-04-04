package tools

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/velariumai/gorkbot/internal/events"
	"github.com/velariumai/gorkbot/pkg/distributed"
)

// ToolCoordinator manages tool execution, caching, and scheduling across local and remote nodes.
// It communicates exclusively via the event bus to coordinate multi-node tool execution.
type ToolCoordinator struct {
	// Tool registry
	registry *Registry

	// Execution tracking
	execMu     sync.RWMutex
	executions map[string]*ToolExecution // keyed by request ID
	cache      map[string]*CachedResult  // simple LRU cache of tool results
	cacheTTL   time.Duration

	// Event bus for coordination
	bus    *distributed.DistributedBus
	logger *slog.Logger

	// Configuration
	maxConcurrent int
	semaphore     chan struct{}

	// Metrics
	mu               sync.RWMutex
	totalExecutions  int64
	successCount     int64
	errorCount       int64
	cacheHits        int64
	avgLatencyMS     int64
	lastExecutedTool string
}

// ToolExecution represents an in-flight tool execution.
type ToolExecution struct {
	RequestID   string
	ToolName    string
	Parameters  map[string]interface{}
	StartedAt   time.Time
	CompletedAt time.Time
	Result      string
	Error       string
	Success     bool
	SourceNode  string // Which node executed this
}

// CachedResult stores a cached tool result with TTL.
type CachedResult struct {
	Result    string
	ExpiresAt time.Time
	Count     int // Hit count
}

// NewToolCoordinator creates a new tool coordinator.
func NewToolCoordinator(
	registry *Registry,
	bus *distributed.DistributedBus,
	logger *slog.Logger,
) *ToolCoordinator {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(nil, nil))
	}

	tc := &ToolCoordinator{
		registry:      registry,
		bus:           bus,
		logger:        logger,
		executions:    make(map[string]*ToolExecution),
		cache:         make(map[string]*CachedResult),
		cacheTTL:      5 * time.Minute,
		maxConcurrent: 10,
		semaphore:     make(chan struct{}, 10),
	}

	// Register event handlers on the bus
	if bus != nil {
		bus.Register("ToolRequestEvent", tc.handleToolRequest)
		bus.Register("ToolResultEvent", tc.handleToolResult)
	}

	return tc
}

// Execute executes a tool with the given parameters.
// Returns the execution request ID for tracking.
func (tc *ToolCoordinator) Execute(ctx context.Context, toolName string, params map[string]interface{}) (string, error) {
	// Validate tool exists
	tool, ok := tc.registry.Get(toolName)
	if !ok {
		return "", fmt.Errorf("tool not found: %s", toolName)
	}

	requestID := uuid.New().String()

	// Check cache first
	cacheKey := fmt.Sprintf("%s:%v", toolName, params)
	if cached, ok := tc.cache[cacheKey]; ok && time.Now().Before(cached.ExpiresAt) {
		tc.mu.Lock()
		tc.cacheHits++
		tc.mu.Unlock()

		tc.logger.Debug("cache hit", "tool", toolName, "request_id", requestID)
		return requestID, nil
	}

	// Create execution record
	exec := &ToolExecution{
		RequestID:  requestID,
		ToolName:   toolName,
		Parameters: params,
		StartedAt:  time.Now(),
	}

	tc.execMu.Lock()
	tc.executions[requestID] = exec
	tc.execMu.Unlock()

	// Update metrics
	tc.mu.Lock()
	tc.totalExecutions++
	tc.mu.Unlock()

	// Emit ToolRequestEvent
	event := &events.ToolRequestEvent{
		BaseEvent:  events.NewBaseEventWithID(requestID),
		ToolName:   toolName,
		Parameters: params,
		RequestID:  requestID,
	}

	tc.bus.Publish(ctx, event)

	// Execute tool in background with semaphore to limit concurrency
	go tc.executeAsync(ctx, tool, exec, cacheKey)

	return requestID, nil
}

// executeAsync performs the actual tool execution.
func (tc *ToolCoordinator) executeAsync(ctx context.Context, tool Tool, exec *ToolExecution, cacheKey string) {
	// Acquire semaphore slot
	tc.semaphore <- struct{}{}
	defer func() { <-tc.semaphore }()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Execute tool
	result, err := tool.Execute(ctx, exec.Parameters)
	exec.CompletedAt = time.Now()

	if err != nil {
		exec.Error = err.Error()
		exec.Success = false

		tc.mu.Lock()
		tc.errorCount++
		tc.mu.Unlock()
	} else {
		exec.Result = result.Output
		exec.Success = result.Success
		if result.Error != "" {
			exec.Error = result.Error
		}

		tc.mu.Lock()
		tc.successCount++
		tc.mu.Unlock()

		// Cache result
		tc.cache[cacheKey] = &CachedResult{
			Result:    result.Output,
			ExpiresAt: time.Now().Add(tc.cacheTTL),
		}
	}

	// Record latency
	latency := exec.CompletedAt.Sub(exec.StartedAt).Milliseconds()
	tc.mu.Lock()
	if tc.avgLatencyMS == 0 {
		tc.avgLatencyMS = latency
	} else {
		tc.avgLatencyMS = (tc.avgLatencyMS + latency) / 2
	}
	tc.lastExecutedTool = exec.ToolName
	tc.mu.Unlock()

	// Emit ToolResultEvent
	resultEvent := &events.ToolResultEvent{
		BaseEvent:  events.NewBaseEventWithID(exec.RequestID),
		ToolName:   exec.ToolName,
		RequestID:  exec.RequestID,
		Success:    exec.Success,
		Output:     exec.Result,
		Error:      exec.Error,
		DurationMS: latency,
	}

	tc.bus.Publish(ctx, resultEvent)

	tc.logger.Debug("tool execution completed",
		"tool", exec.ToolName,
		"request_id", exec.RequestID,
		"success", exec.Success,
		"latency_ms", latency,
	)
}

// GetExecution retrieves an execution by request ID.
func (tc *ToolCoordinator) GetExecution(requestID string) *ToolExecution {
	tc.execMu.RLock()
	defer tc.execMu.RUnlock()
	return tc.executions[requestID]
}

// ListTools returns all available tools (for UI discovery).
func (tc *ToolCoordinator) ListTools() []string {
	tools := tc.registry.List()
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name())
	}
	return names
}

// GetToolInfo returns metadata about a tool.
func (tc *ToolCoordinator) GetToolInfo(toolName string) *Tool {
	tool, ok := tc.registry.Get(toolName)
	if !ok {
		return nil
	}
	return &tool
}

// ClearCache removes expired entries from the cache.
func (tc *ToolCoordinator) ClearCache() {
	tc.execMu.Lock()
	defer tc.execMu.Unlock()

	now := time.Now()
	removed := 0

	for key, cached := range tc.cache {
		if now.After(cached.ExpiresAt) {
			delete(tc.cache, key)
			removed++
		}
	}

	tc.logger.Debug("cache cleared", "removed", removed, "remaining", len(tc.cache))
}

// Stats returns execution statistics.
func (tc *ToolCoordinator) Stats() map[string]interface{} {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	successRate := float64(0)
	if tc.totalExecutions > 0 {
		successRate = float64(tc.successCount) / float64(tc.totalExecutions) * 100
	}

	return map[string]interface{}{
		"total_executions":   tc.totalExecutions,
		"success_count":      tc.successCount,
		"error_count":        tc.errorCount,
		"success_rate_pct":   successRate,
		"cache_hits":         tc.cacheHits,
		"average_latency_ms": tc.avgLatencyMS,
		"last_executed_tool": tc.lastExecutedTool,
		"cached_results":     len(tc.cache),
	}
}

// ─── Event Handlers ───────────────────────────────────────────────

// handleToolRequest processes incoming ToolRequestEvent from remote nodes.
func (tc *ToolCoordinator) handleToolRequest(ctx context.Context, event events.BusEvent) events.BusEvent {
	toolReq, ok := event.(*events.ToolRequestEvent)
	if !ok {
		return nil
	}

	// Get tool
	tool, exists := tc.registry.Get(toolReq.ToolName)
	if !exists {
		// Return error event
		return &events.ToolResultEvent{
			BaseEvent:  events.NewBaseEventWithID(toolReq.RequestID),
			ToolName:   toolReq.ToolName,
			RequestID:  toolReq.RequestID,
			Success:    false,
			Error:      fmt.Sprintf("tool not found: %s", toolReq.ToolName),
			DurationMS: 0,
		}
	}

	// Execute synchronously for RPC-style calls
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	result, err := tool.Execute(ctx, toolReq.Parameters)

	resultEvent := &events.ToolResultEvent{
		BaseEvent:  events.NewBaseEventWithID(toolReq.RequestID),
		ToolName:   toolReq.ToolName,
		RequestID:  toolReq.RequestID,
		Success:    err == nil && result.Success,
		Output:     result.Output,
		DurationMS: 0,
	}

	if err != nil {
		resultEvent.Error = err.Error()
	} else if result.Error != "" {
		resultEvent.Error = result.Error
	}

	return resultEvent
}

// handleToolResult processes completed tool executions.
func (tc *ToolCoordinator) handleToolResult(ctx context.Context, event events.BusEvent) events.BusEvent {
	toolResult, ok := event.(*events.ToolResultEvent)
	if !ok {
		return nil
	}

	// Update execution record if we're tracking it
	tc.execMu.Lock()
	if exec, ok := tc.executions[toolResult.RequestID]; ok {
		exec.Success = toolResult.Success
		exec.Error = toolResult.Error
		exec.Result = toolResult.Output
		exec.CompletedAt = time.Now()
	}
	tc.execMu.Unlock()

	tc.logger.Debug("tool result processed",
		"tool", toolResult.ToolName,
		"request_id", toolResult.RequestID,
		"success", toolResult.Success,
	)

	return nil
}
