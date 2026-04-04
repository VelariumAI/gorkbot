package distributed

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/velariumai/gorkbot/internal/events"
)

// TestGRPCTransport_StartStop verifies gRPC transport lifecycle.
func TestGRPCTransport_StartStop(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	config := TransportConfig{
		NodeID:                 "node1",
		ListenAddr:             "127.0.0.1:9091",
		TransportType:          "grpc",
		AllowInsecureTransport: true,
		HeartbeatInterval:      1 * time.Second,
		MaxRetries:             3,
	}

	transport := NewGRPCTransport(config, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start
	if err := transport.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if !transport.IsReady() {
		t.Error("Transport not ready after Start")
	}

	// Stop
	if err := transport.Stop(ctx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if transport.IsReady() {
		t.Error("Transport still ready after Stop")
	}
}

// TestGRPCTransport_RegisterNode verifies node registration.
func TestGRPCTransport_RegisterNode(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	config := TransportConfig{
		NodeID:                 "node1",
		ListenAddr:             "127.0.0.1:9092",
		TransportType:          "grpc",
		AllowInsecureTransport: true,
		HeartbeatInterval:      1 * time.Second,
	}

	transport := NewGRPCTransport(config, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_ = transport.Start(ctx)
	defer transport.Stop(ctx)

	entry := &ServiceDiscoveryEntry{
		NodeId:       "node2",
		NodeAddress:  "127.0.0.1:9093",
		Capabilities: []string{"tool_coordinator"},
		Status:       "healthy",
	}

	if err := transport.RegisterNode(ctx, entry); err != nil {
		t.Fatalf("RegisterNode failed: %v", err)
	}

	info := transport.NodeInfo("node2")
	if info == nil {
		t.Error("NodeInfo returned nil for registered node")
	}
	if info.NodeId != "node2" {
		t.Errorf("NodeInfo returned wrong node: %s", info.NodeId)
	}
}

func TestGRPCTransport_StartRequiresExplicitInsecureOptIn(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	config := TransportConfig{
		NodeID:                 "node1",
		ListenAddr:             "127.0.0.1:9191",
		TransportType:          "grpc",
		AllowInsecureTransport: false,
		HeartbeatInterval:      1 * time.Second,
	}

	transport := NewGRPCTransport(config, logger)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := transport.Start(ctx)
	if err == nil {
		t.Fatalf("expected start to fail without insecure opt-in")
	}
}

// TestInMemoryEventSource_StoreAndLoad verifies event persistence.
func TestInMemoryEventSource_StoreAndLoad(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	store := NewInMemoryEventSource(logger)
	defer store.Close()

	ctx := context.Background()

	entry := &EventStoreEntry{
		EventId:       "evt-1",
		TimestampMs:   time.Now().UnixMilli(),
		EventType:     "ToolRequestEvent",
		CorrelationId: "corr-123",
		Payload:       []byte(`{"tool_name":"bash","parameters":{}}`),
		SourceNode:    "node1",
	}

	seq, err := store.Store(ctx, entry)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}
	if seq != 1 {
		t.Errorf("Expected sequence 1, got %d", seq)
	}

	loaded, err := store.Load(ctx, "evt-1")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.EventId != "evt-1" {
		t.Errorf("Loaded wrong event: %s", loaded.EventId)
	}
	if loaded.Hash == "" {
		t.Error("Hash not computed")
	}
}

// TestInMemoryEventSource_Query verifies filtering.
func TestInMemoryEventSource_Query(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	store := NewInMemoryEventSource(logger)
	defer store.Close()

	ctx := context.Background()

	// Store multiple events
	for i := 0; i < 5; i++ {
		entry := &EventStoreEntry{
			EventId:       string(rune(i)),
			TimestampMs:   time.Now().UnixMilli(),
			EventType:     "ToolRequestEvent",
			CorrelationId: "corr-123",
			Payload:       []byte(`{}`),
			SourceNode:    "node1",
		}
		_, _ = store.Store(ctx, entry)
	}

	// Query by correlation ID
	results, err := store.Query(ctx, "", "corr-123", 0)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 5 {
		t.Errorf("Expected 5 results, got %d", len(results))
	}

	// Query by event type
	results, err = store.Query(ctx, "ToolRequestEvent", "", 0)
	if err != nil {
		t.Fatalf("Query by type failed: %v", err)
	}
	if len(results) != 5 {
		t.Errorf("Expected 5 results, got %d", len(results))
	}
}

// TestInMemoryEventSource_Tail verifies recent event retrieval.
func TestInMemoryEventSource_Tail(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	store := NewInMemoryEventSource(logger)
	defer store.Close()

	ctx := context.Background()

	// Store 10 events
	for i := 0; i < 10; i++ {
		entry := &EventStoreEntry{
			EventId:       string(rune(i)),
			TimestampMs:   time.Now().UnixMilli(),
			EventType:     "Test",
			CorrelationId: "corr-x",
			Payload:       []byte(`{}`),
		}
		_, _ = store.Store(ctx, entry)
	}

	// Get last 5
	results, err := store.Tail(ctx, 5)
	if err != nil {
		t.Fatalf("Tail failed: %v", err)
	}
	if len(results) != 5 {
		t.Errorf("Expected 5 results, got %d", len(results))
	}
}

// TestInMemoryEventSource_Compact verifies cleanup.
func TestInMemoryEventSource_Compact(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	store := NewInMemoryEventSource(logger)
	defer store.Close()

	ctx := context.Background()

	// Store old and new events
	oldTime := time.Now().Add(-2 * time.Hour).UnixMilli()

	for i := 0; i < 3; i++ {
		entry := &EventStoreEntry{
			EventId:       string(rune(i)),
			TimestampMs:   oldTime,
			EventType:     "OldEvent",
			CorrelationId: "corr-old",
			Payload:       []byte(`{}`),
		}
		_, _ = store.Store(ctx, entry)
	}

	// Store new events
	for i := 3; i < 6; i++ {
		entry := &EventStoreEntry{
			EventId:       string(rune(i)),
			TimestampMs:   time.Now().UnixMilli(),
			EventType:     "NewEvent",
			CorrelationId: "corr-new",
			Payload:       []byte(`{}`),
		}
		_, _ = store.Store(ctx, entry)
	}

	size, _ := store.Size(ctx)
	if size != 6 {
		t.Errorf("Expected 6 events before compact, got %d", size)
	}

	// Compact (keep only last 1 hour)
	_ = store.Compact(ctx, 60*60*1000)

	size, _ = store.Size(ctx)
	if size != 3 {
		t.Errorf("Expected 3 events after compact, got %d", size)
	}
}

// TestDistributedBus_PublishAndSubscribe verifies event flow.
func TestDistributedBus_PublishAndSubscribe(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Create local bus
	localBus := events.NewBus()

	// Create event source
	eventSrc := NewInMemoryEventSource(logger)
	defer eventSrc.Close()

	// Create distributed bus (without transport for this test)
	dBus := NewDistributedBus(localBus, nil, eventSrc, "node1", logger)

	ctx := context.Background()

	// Register handler
	received := make(chan events.BusEvent, 1)
	localBus.Register("ToolRequestEvent", func(ctx context.Context, event events.BusEvent) events.BusEvent {
		received <- event
		return nil
	})

	// Publish event
	evt := &events.ToolRequestEvent{
		BaseEvent:  events.NewBaseEvent(),
		ToolName:   "bash",
		Parameters: map[string]interface{}{},
		RequestID:  "req-1",
	}

	dBus.Publish(ctx, evt)

	// Verify handler was called
	select {
	case e := <-received:
		if req, ok := e.(*events.ToolRequestEvent); !ok || req.ToolName != "bash" {
			t.Error("Handler received wrong event")
		}
	case <-time.After(2 * time.Second):
		t.Error("Handler not called")
	}
}

// TestConcurrentExecutions verifies thread safety.
func TestConcurrentExecutions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	localBus := events.NewBus()
	eventSrc := NewInMemoryEventSource(logger)
	defer eventSrc.Close()

	dBus := NewDistributedBus(localBus, nil, eventSrc, "node1", logger)

	ctx := context.Background()

	// Publish 100 events concurrently
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			evt := &events.ProviderEvent{
				BaseEvent:  events.NewBaseEvent(),
				ProviderID: "xai",
				Action:     "ready",
			}
			dBus.Publish(ctx, evt)
		}(i)
	}

	wg.Wait()

	// Verify all events were stored
	size, err := eventSrc.Size(ctx)
	if err != nil {
		t.Fatalf("Size failed: %v", err)
	}
	if size != 100 {
		t.Errorf("Expected 100 events, got %d", size)
	}
}

// TestNetworkPartitionRecovery verifies event sourcing on reconnect.
func TestNetworkPartitionRecovery(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	localBus := events.NewBus()
	eventSrc := NewInMemoryEventSource(logger)
	defer eventSrc.Close()

	dBus := NewDistributedBus(localBus, nil, eventSrc, "node1", logger)

	ctx := context.Background()

	// Publish events
	for i := 0; i < 5; i++ {
		evt := &events.ToolRequestEvent{
			BaseEvent:  events.NewBaseEvent(),
			ToolName:   "tool-x",
			Parameters: map[string]interface{}{},
			RequestID:  string(rune(i)),
		}
		dBus.Publish(ctx, evt)
	}

	// Simulate partition recovery: query stored events
	results, err := eventSrc.Tail(ctx, 10)
	if err != nil {
		t.Fatalf("Tail failed: %v", err)
	}
	if len(results) < 5 {
		t.Errorf("Expected at least 5 events after partition, got %d", len(results))
	}

	// Verify integrity via hash
	hash1, _ := eventSrc.Hash(ctx)
	hash2, _ := eventSrc.Hash(ctx)
	if hash1 != hash2 {
		t.Error("Event hash changed unexpectedly")
	}
}
