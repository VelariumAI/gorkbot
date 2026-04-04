package memory

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/velariumai/gorkbot/internal/events"
)

// MemoryObserver is a narrow observability interface for memory operations.
// Avoids import cycles by not depending on internal/observability.
type MemoryObserver interface {
	RecordMemoryQuery(latencyMS int64)
	RecordMemoryQuality(cacheHit, compression, relevance float64)
}

// MemoryCoordinator manages memory queries across the distributed system via the event bus.
// It handles both local and cross-node memory operations.
type MemoryCoordinator struct {
	unified   *UnifiedMemory         // nil-safe
	temporal  *TemporalMemory        // nil-safe
	bus       events.BusPublisher    // nil-safe
	logger    *slog.Logger
	obs       MemoryObserver         // nil-safe
	mu        sync.RWMutex
	queryCount   int64
	compactCount int64
	errorCount   int64
	totalLatMS   int64
}

// MemoryCoordinatorStats reports coordinator metrics.
type MemoryCoordinatorStats struct {
	QueryCount   int64
	CompactCount int64
	ErrorCount   int64
	AvgLatencyMS int64
}

// NewMemoryCoordinator creates a new memory coordinator.
// Registers event handlers on the bus if available.
func NewMemoryCoordinator(
	unified *UnifiedMemory,
	temporal *TemporalMemory,
	bus events.BusPublisher,
	logger *slog.Logger,
) *MemoryCoordinator {
	if logger == nil {
		logger = slog.Default()
	}

	mc := &MemoryCoordinator{
		unified: unified,
		temporal: temporal,
		bus: bus,
		logger: logger,
	}

	// Register event handlers
	if bus != nil {
		bus.Register("MemoryQueryEvent", mc.handleMemoryQuery)
		bus.Register("CompressionEvent", mc.handleCompression)
	}

	return mc
}

// SetObservability sets the observability reporter for memory metrics.
func (mc *MemoryCoordinator) SetObservability(obs MemoryObserver) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.obs = obs
}

// Stats returns the current coordinator statistics.
func (mc *MemoryCoordinator) Stats() MemoryCoordinatorStats {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	avgLatency := int64(0)
	if mc.queryCount > 0 {
		avgLatency = mc.totalLatMS / mc.queryCount
	}

	return MemoryCoordinatorStats{
		QueryCount: mc.queryCount,
		CompactCount: mc.compactCount,
		ErrorCount: mc.errorCount,
		AvgLatencyMS: avgLatency,
	}
}

// handleMemoryQuery processes memory query events.
func (mc *MemoryCoordinator) handleMemoryQuery(ctx context.Context, event events.BusEvent) events.BusEvent {
	queryEvent, ok := event.(*events.MemoryQueryEvent)
	if !ok {
		return nil
	}

	startTime := time.Now()

	// Generate RequestID if empty
	requestID := queryEvent.RequestID
	if requestID == "" {
		requestID = uuid.New().String()
	}

	// Query unified memory (nil-safe)
	var result string
	var errStr string

	if mc.unified != nil {
		result = mc.unified.Query(queryEvent.Query, 600)
	} else {
		errStr = "unified memory unavailable"
	}

	// Create response event
	latencyMS := time.Since(startTime).Milliseconds()
	resultEvent := &events.MemoryResultEvent{
		BaseEvent: events.NewBaseEventWithID(event.CorrelationID()),
		RequestID: requestID,
		QueryType: queryEvent.QueryType,
		Result: result,
		LatencyMS: latencyMS,
		Error: errStr,
	}

	// Update metrics
	mc.mu.Lock()
	mc.queryCount++
	mc.totalLatMS += latencyMS
	if errStr != "" {
		mc.errorCount++
	}
	mc.mu.Unlock()

	// Record to observability (nil-safe, outside lock)
	if mc.obs != nil {
		mc.obs.RecordMemoryQuery(latencyMS)
	}

	mc.logger.Debug("processed memory query",
		slog.String("query_type", queryEvent.QueryType),
		slog.Int64("latency_ms", latencyMS),
		slog.String("error", errStr))

	return resultEvent
}

// handleCompression processes compression events.
func (mc *MemoryCoordinator) handleCompression(ctx context.Context, event events.BusEvent) events.BusEvent {
	compressionEvent, ok := event.(*events.CompressionEvent)
	if !ok {
		return nil
	}

	// No-op if temporal memory unavailable
	if mc.temporal == nil {
		mc.logger.Debug("compression skipped: temporal memory unavailable")
		return nil
	}

	startTime := time.Now()

	// Perform compression
	if err := mc.temporal.CompactFacts(ctx); err != nil {
		mc.logger.Error("compression failed", "error", err)
		mc.mu.Lock()
		mc.errorCount++
		mc.mu.Unlock()
		return nil
	}

	// Update metrics
	mc.mu.Lock()
	mc.compactCount++
	mc.mu.Unlock()

	mc.logger.Debug("compression completed",
		slog.String("source", compressionEvent.Source),
		slog.Duration("elapsed", time.Since(startTime)))

	return nil
}
