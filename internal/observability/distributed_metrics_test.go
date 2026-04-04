package observability

import (
	"log/slog"
	"strings"
	"testing"
)

// TestDistributedMetrics_RecordAndCount verifies all counter recordings.
func TestDistributedMetrics_RecordAndCount(t *testing.T) {
	dm := &DistributedMetrics{}

	// Record node events
	dm.RecordNodeDiscovered()
	dm.RecordNodeDiscovered()
	dm.RecordNodeLost()

	if dm.NodesDiscovered != 2 {
		t.Errorf("expected NodesDiscovered=2, got %d", dm.NodesDiscovered)
	}

	if dm.NodesLost != 1 {
		t.Errorf("expected NodesLost=1, got %d", dm.NodesLost)
	}

	// Record event publications
	dm.RecordEventPublished(false, 0, false) // local
	dm.RecordEventPublished(true, 50, false) // remote success
	dm.RecordEventPublished(true, 100, true) // remote failure

	if dm.EventsPublishedLocal != 1 {
		t.Errorf("expected EventsPublishedLocal=1, got %d", dm.EventsPublishedLocal)
	}

	if dm.EventsPublishedRemote != 2 {
		t.Errorf("expected EventsPublishedRemote=2, got %d", dm.EventsPublishedRemote)
	}

	if dm.RemotePublishErrors != 1 {
		t.Errorf("expected RemotePublishErrors=1, got %d", dm.RemotePublishErrors)
	}

	// Record provider failover
	dm.RecordProviderFailover()
	if dm.ProviderFailovers != 1 {
		t.Errorf("expected ProviderFailovers=1, got %d", dm.ProviderFailovers)
	}

	// Record memory queries
	dm.RecordMemoryQuery(100)
	dm.RecordMemoryQuery(50)
	if dm.MemoryQueries != 2 {
		t.Errorf("expected MemoryQueries=2, got %d", dm.MemoryQueries)
	}
}

// TestDistributedMetrics_AvgLatency verifies average latency calculation.
func TestDistributedMetrics_AvgLatency(t *testing.T) {
	dm := &DistributedMetrics{}

	// Record remote events with latencies
	dm.RecordEventPublished(true, 100, false)
	dm.RecordEventPublished(true, 200, false)

	avgRemote := dm.AvgRemoteLatencyMS()
	expected := int64(150) // (100+200)/2
	if avgRemote != expected {
		t.Errorf("expected avg remote latency %d, got %d", expected, avgRemote)
	}

	// Record memory queries
	dm.RecordMemoryQuery(50)
	dm.RecordMemoryQuery(150)

	avgMem := dm.AvgMemoryLatencyMS()
	expected = int64(100) // (50+150)/2
	if avgMem != expected {
		t.Errorf("expected avg memory latency %d, got %d", expected, avgMem)
	}
}

// TestDistributedMetrics_ExportFormat verifies Prometheus export includes distributed metrics.
func TestDistributedMetrics_ExportFormat(t *testing.T) {
	logger := slog.Default()
	hub := NewObservabilityHub(logger)

	// Record some distributed metrics
	hub.RecordNodeDiscovered()
	hub.RecordEventPublished(true, 100, false)
	hub.RecordProviderFailover()
	hub.RecordMemoryQuery(50)

	export := hub.ExportMetrics()

	// Check for key metric lines
	expectedMetrics := []string{
		"gorkbot_distributed_nodes_discovered_total",
		"gorkbot_distributed_events_remote_total",
		"gorkbot_distributed_provider_failovers_total",
		"gorkbot_distributed_memory_queries_total",
		"gorkbot_distributed_remote_avg_latency_ms",
		"gorkbot_distributed_memory_avg_latency_ms",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(export, metric) {
			t.Errorf("expected metric %s not found in export", metric)
		}
	}
}

// TestDistributedMetrics_NilSafe verifies NewObservabilityHub initializes distributed field.
func TestDistributedMetrics_NilSafe(t *testing.T) {
	logger := slog.Default()
	hub := NewObservabilityHub(logger)

	// distributed field should be initialized
	if hub.distributed == nil {
		t.Errorf("expected distributed field to be initialized, got nil")
	}

	// Methods should be callable without panic
	hub.RecordNodeDiscovered()
	hub.RecordNodeLost()
	hub.RecordEventPublished(true, 100, false)
	hub.RecordProviderFailover()
	hub.RecordMemoryQuery(50)

	// GetDistributedMetrics should return the metrics
	metrics := hub.GetDistributedMetrics()
	if metrics == nil {
		t.Errorf("expected non-nil distributed metrics")
	}

	if metrics.NodesDiscovered != 1 {
		t.Errorf("expected NodesDiscovered=1, got %d", metrics.NodesDiscovered)
	}
}
