package memory

import (
	"context"
	"log/slog"
	"testing"

	"github.com/velariumai/gorkbot/internal/events"
)

// TestMemoryCoordinator_HandlesQuery verifies query handling and result generation.
func TestMemoryCoordinator_HandlesQuery(t *testing.T) {
	// Create a minimal mock unified memory
	unified := &UnifiedMemory{}
	bus := events.NewBus()
	logger := slog.Default()

	mc := NewMemoryCoordinator(unified, nil, bus, logger)

	// Create a query event
	queryEvent := &events.MemoryQueryEvent{
		BaseEvent: events.NewBaseEvent(),
		QueryType: "search",
		Query: "test query",
		RequestID: "req-123",
	}

	// Handle the query
	ctx := context.Background()
	result := mc.handleMemoryQuery(ctx, queryEvent)

	// Verify response event
	resultEvent, ok := result.(*events.MemoryResultEvent)
	if !ok {
		t.Fatalf("expected MemoryResultEvent, got %T", result)
	}

	if resultEvent.RequestID != "req-123" {
		t.Errorf("expected RequestID req-123, got %s", resultEvent.RequestID)
	}

	if resultEvent.QueryType != "search" {
		t.Errorf("expected QueryType search, got %s", resultEvent.QueryType)
	}

	if resultEvent.LatencyMS < 0 {
		t.Errorf("expected non-negative latency, got %d", resultEvent.LatencyMS)
	}
}

// TestMemoryCoordinator_NilUnifiedReturnsError verifies behavior when unified memory is nil.
func TestMemoryCoordinator_NilUnifiedReturnsError(t *testing.T) {
	bus := events.NewBus()
	logger := slog.Default()

	mc := NewMemoryCoordinator(nil, nil, bus, logger)

	queryEvent := &events.MemoryQueryEvent{
		BaseEvent: events.NewBaseEvent(),
		QueryType: "search",
		Query: "test",
	}

	ctx := context.Background()
	result := mc.handleMemoryQuery(ctx, queryEvent)

	resultEvent, ok := result.(*events.MemoryResultEvent)
	if !ok {
		t.Fatalf("expected MemoryResultEvent, got %T", result)
	}

	if resultEvent.Error == "" {
		t.Errorf("expected error message, got empty")
	}
}

// TestMemoryCoordinator_CompressionNilTemporal verifies no panic with nil temporal.
func TestMemoryCoordinator_CompressionNilTemporal(t *testing.T) {
	bus := events.NewBus()
	logger := slog.Default()

	mc := NewMemoryCoordinator(nil, nil, bus, logger)

	compressionEvent := &events.CompressionEvent{
		BaseEvent: events.NewBaseEvent(),
		Source: "conversation",
		TargetSize: 1000,
	}

	ctx := context.Background()

	// Should not panic
	result := mc.handleCompression(ctx, compressionEvent)
	if result != nil {
		t.Errorf("expected nil result when temporal is nil, got %v", result)
	}
}

// TestMemoryCoordinator_StatsAccuracy verifies metric accuracy.
func TestMemoryCoordinator_StatsAccuracy(t *testing.T) {
	bus := events.NewBus()
	logger := slog.Default()

	mc := NewMemoryCoordinator(&UnifiedMemory{}, nil, bus, logger)

	// Process 3 queries
	for i := 0; i < 3; i++ {
		queryEvent := &events.MemoryQueryEvent{
			BaseEvent: events.NewBaseEvent(),
			QueryType: "search",
			Query: "test",
		}

		ctx := context.Background()
		_ = mc.handleMemoryQuery(ctx, queryEvent)
	}

	// Check stats
	stats := mc.Stats()
	if stats.QueryCount != 3 {
		t.Errorf("expected QueryCount=3, got %d", stats.QueryCount)
	}

	if stats.AvgLatencyMS < 0 {
		t.Errorf("expected non-negative average latency, got %d", stats.AvgLatencyMS)
	}
}

// TestMemoryCoordinator_NilBusConstruction verifies nil bus doesn't panic.
func TestMemoryCoordinator_NilBusConstruction(t *testing.T) {
	logger := slog.Default()

	// Should not panic
	mc := NewMemoryCoordinator(&UnifiedMemory{}, nil, nil, logger)
	if mc == nil {
		t.Errorf("expected non-nil coordinator")
	}

	// Should handle queries even with nil bus
	queryEvent := &events.MemoryQueryEvent{
		BaseEvent: events.NewBaseEvent(),
		QueryType: "search",
		Query: "test",
	}

	ctx := context.Background()
	result := mc.handleMemoryQuery(ctx, queryEvent)
	if result == nil {
		t.Errorf("expected result even with nil bus")
	}
}
