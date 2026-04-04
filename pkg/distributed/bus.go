package distributed

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/velariumai/gorkbot/internal/events"
)

// ObservabilityReporter is a narrow interface for distributed metrics reporting.
// Avoids import cycles by not depending on internal/observability.
type ObservabilityReporter interface {
	RecordEventPublished(remote bool, latencyMS int64, failed bool)
	RecordNodeDiscovered()
	RecordNodeLost()
}

// DistributedBus extends the local Bus with remote event delivery.
// It maintains the local-first semantics while enabling cross-node coordination.
type DistributedBus struct {
	local      *events.Bus
	transport  Transport
	eventSrc   EventSource
	logger     *slog.Logger
	nodeID     string
	mu         sync.RWMutex
	isReady    bool
	remoteOnly map[string]bool // Event types that only route remotely

	// Distributed routing and observability
	selector     NodeSelector
	nodeStats    map[string]*NodeStats
	nodeStatsMu  sync.RWMutex
	capabilities []string
	obs          ObservabilityReporter
}

// NewDistributedBus creates a bus with distributed capabilities.
func NewDistributedBus(
	local *events.Bus,
	transport Transport,
	eventSrc EventSource,
	nodeID string,
	logger *slog.Logger,
) *DistributedBus {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(nil, nil))
	}

	db := &DistributedBus{
		local:      local,
		transport:  transport,
		eventSrc:   eventSrc,
		logger:     logger,
		nodeID:     nodeID,
		remoteOnly: make(map[string]bool),
		nodeStats:  make(map[string]*NodeStats),
	}

	// Register transport handler to process incoming remote events
	if transport != nil {
		_ = transport.Subscribe(db.handleRemoteEvent)
	}

	return db
}

// Start initializes the distributed bus.
func (db *DistributedBus) Start(ctx context.Context) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.isReady {
		return nil
	}

	// Start transport
	if db.transport != nil {
		if err := db.transport.Start(ctx); err != nil {
			db.logger.Error("transport start failed", "error", err)
			return err
		}

		// Get capabilities (use default if not set)
		caps := db.capabilities
		if len(caps) == 0 {
			caps = []string{"provider_coordinator", "tool_coordinator"}
		}

		// Register this node
		entry := &ServiceDiscoveryEntry{
			NodeId:       db.nodeID,
			NodeAddress:  db.localNodeAddress(),
			StartedAtMs:  time.Now().UnixMilli(),
			Capabilities: caps,
			HeartbeatMs:  time.Now().UnixMilli(),
			Status:       "healthy",
		}

		if err := db.transport.RegisterNode(ctx, entry); err != nil {
			db.logger.Error("register node failed", "error", err)
			return err
		}
	}

	db.isReady = true
	db.logger.Info("DistributedBus started", "node_id", db.nodeID)
	return nil
}

func (db *DistributedBus) localNodeAddress() string {
	if addr := os.Getenv("GORKBOT_DISTRIBUTED_ADDR"); addr != "" {
		return addr
	}
	return "localhost:9090"
}

// Stop gracefully stops the distributed bus.
func (db *DistributedBus) Stop(ctx context.Context) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if !db.isReady {
		return nil
	}

	if db.transport != nil {
		if err := db.transport.Stop(ctx); err != nil {
			db.logger.Warn("transport stop failed", "error", err)
		}
	}

	if db.eventSrc != nil {
		if err := db.eventSrc.Close(); err != nil {
			db.logger.Warn("event source close failed", "error", err)
		}
	}

	db.isReady = false
	return nil
}

// Publish dispatches an event to local and optionally remote handlers.
// If remoteOnly is set for the event type, it only goes to remote nodes.
func (db *DistributedBus) Publish(ctx context.Context, event events.BusEvent) events.BusEvent {
	eventType := events.EventTypeName(event)
	corrID := event.CorrelationID()
	requestID := uuid.New().String()

	// Store event (if event source available)
	if db.eventSrc != nil {
		entry := &EventStoreEntry{
			EventId:       requestID,
			Sequence:      0,
			TimestampMs:   time.Now().UnixMilli(),
			EventType:     eventType,
			CorrelationId: corrID,
			Payload:       db.serializeEvent(event),
			SourceNode:    db.nodeID,
		}
		_, _ = db.eventSrc.Store(ctx, entry)
	}

	// Check if event is local-only
	db.mu.RLock()
	remoteOnly := db.remoteOnly[eventType]
	db.mu.RUnlock()

	var localResponse events.BusEvent

	// Publish locally
	if !remoteOnly && db.local != nil {
		localResponse = db.local.Publish(ctx, event)
	}

	// Publish remotely if transport available
	if db.transport != nil && db.transport.IsReady() {
		go db.publishRemote(ctx, eventType, event, requestID, corrID)
	}

	return localResponse
}

// PublishRemote publishes an event to remote nodes using the node selector.
func (db *DistributedBus) publishRemote(ctx context.Context, eventType string, event events.BusEvent, requestID, corrID string) {
	remoteEvent := &RemoteEvent{
		Type:          eventType,
		CorrelationId: corrID,
		TimestampMs:   time.Now().UnixMilli(),
		RequestId:     requestID,
		SourceNode:    db.nodeID,
	}

	// Marshal event to JSON
	data, err := json.Marshal(event)
	if err != nil {
		db.logger.Error("marshal event failed", "error", err)
		return
	}
	remoteEvent.Payload = data

	// Get all known nodes
	allNodes := db.transport.ListNodes()

	// Use node selector if available, otherwise filter self
	var targetNodes []string
	if db.selector != nil {
		targetNodes = db.selector.Select(eventType, allNodes)
	} else {
		for _, nodeID := range allNodes {
			if nodeID != db.nodeID {
				targetNodes = append(targetNodes, nodeID)
			}
		}
	}

	// Send to selected nodes
	for _, nodeID := range targetNodes {
		go func(target string) {
			startTime := time.Now()
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			_, err := db.transport.Publish(ctx, target, remoteEvent)
			latencyMS := time.Since(startTime).Milliseconds()
			failed := err != nil

			if failed {
				db.logger.Debug("remote publish failed", "target", target, "error", err)
			}

			// Record latency
			db.RecordNodeLatency(target, latencyMS, failed)

			// Record to observability if available
			if db.obs != nil {
				db.obs.RecordEventPublished(true, latencyMS, failed)
			}
		}(nodeID)
	}
}

// handleRemoteEvent processes incoming events from remote nodes.
func (db *DistributedBus) handleRemoteEvent(ctx context.Context, remoteEvent *RemoteEvent) (*RemoteEvent, error) {
	// Deserialize event
	var event events.BusEvent

	switch remoteEvent.Type {
	case "ProviderEvent":
		event = &events.ProviderEvent{}
	case "ProviderHealthEvent":
		event = &events.ProviderHealthEvent{}
	case "ProviderFailoverEvent":
		event = &events.ProviderFailoverEvent{}
	case "ToolRequestEvent":
		event = &events.ToolRequestEvent{}
	case "ToolResultEvent":
		event = &events.ToolResultEvent{}
	case "MemoryQueryEvent":
		event = &events.MemoryQueryEvent{}
	case "MemoryResultEvent":
		event = &events.MemoryResultEvent{}
	case "CompressionEvent":
		event = &events.CompressionEvent{}
	case "ImprovementProposalEvent":
		event = &events.ImprovementProposalEvent{}
	case "RollbackEvent":
		event = &events.RollbackEvent{}
	default:
		db.logger.Warn("unknown event type", "type", remoteEvent.Type)
		return nil, fmt.Errorf("unknown event type: %s", remoteEvent.Type)
	}

	if err := json.Unmarshal(remoteEvent.Payload, event); err != nil {
		db.logger.Error("unmarshal event failed", "error", err)
		return nil, err
	}

	// Publish locally
	response := db.local.Publish(ctx, event)

	// Prepare response
	respEvent := &RemoteEvent{
		Type:          "response",
		CorrelationId: remoteEvent.CorrelationId,
		RequestId:     remoteEvent.RequestId,
		SourceNode:    db.nodeID,
		TimestampMs:   time.Now().UnixMilli(),
	}

	if response != nil {
		data, _ := json.Marshal(response)
		respEvent.Payload = data
	}

	return respEvent, nil
}

// Register registers a handler on the local bus.
func (db *DistributedBus) Register(eventType string, handler events.Handler) {
	if db.local != nil {
		db.local.Register(eventType, handler)
	}
}

// SetRemoteOnly marks an event type as remote-only (not processed locally).
func (db *DistributedBus) SetRemoteOnly(eventType string, remoteOnly bool) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.remoteOnly[eventType] = remoteOnly
}

// IsReady returns true if the distributed bus is operational.
func (db *DistributedBus) IsReady() bool {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.isReady
}

// Transport returns the underlying transport (for testing).
func (db *DistributedBus) Transport() Transport {
	return db.transport
}

// EventSource returns the underlying event source (for testing).
func (db *DistributedBus) EventSource() EventSource {
	return db.eventSrc
}

// SetNodeSelector sets the node selector for remote event routing.
func (db *DistributedBus) SetNodeSelector(selector NodeSelector) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.selector = selector
}

// SetObservability sets the observability reporter for distributed metrics.
func (db *DistributedBus) SetObservability(obs ObservabilityReporter) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.obs = obs
}

// SetCapabilities sets the advertised capabilities for this node.
func (db *DistributedBus) SetCapabilities(caps []string) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.capabilities = caps
}

// RecordNodeLatency records latency for a node (and delegates to selector if applicable).
func (db *DistributedBus) RecordNodeLatency(nodeID string, latencyMS int64, failed bool) {
	db.nodeStatsMu.Lock()
	stats, ok := db.nodeStats[nodeID]
	if !ok {
		stats = &NodeStats{NodeID: nodeID}
		db.nodeStats[nodeID] = stats
	}
	db.nodeStatsMu.Unlock()

	stats.Record(latencyMS, failed)

	// Delegate to selector if it's a least-loaded selector
	if llSelector, ok := db.selector.(*LeastLoadedSelector); ok {
		llSelector.RecordLatency(nodeID, latencyMS, failed)
	}
}

// serializeEvent converts a BusEvent to JSON.
func (db *DistributedBus) serializeEvent(event events.BusEvent) []byte {
	data, err := json.Marshal(event)
	if err != nil {
		db.logger.Error("serialize failed", "error", err)
		return []byte("{}")
	}
	return data
}
