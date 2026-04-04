package distributed

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// NATSTransport implements Transport using NATS JetStream for event delivery.
type NATSTransport struct {
	config       TransportConfig
	logger       *slog.Logger
	conn         *nats.Conn
	js           jetstream.JetStream
	mu           sync.RWMutex
	isReady      bool
	nodes        map[string]*ServiceDiscoveryEntry
	nodesMu      sync.RWMutex
	eventHandler func(ctx context.Context, event *RemoteEvent) (*RemoteEvent, error)
	rpcHandlers  map[string]chan *RemoteEvent
	rpcMu        sync.RWMutex
}

// NewNATSTransport creates a new NATS JetStream transport.
func NewNATSTransport(config TransportConfig, logger *slog.Logger) *NATSTransport {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(nil, nil))
	}

	return &NATSTransport{
		config:      config,
		logger:      logger,
		nodes:       make(map[string]*ServiceDiscoveryEntry),
		rpcHandlers: make(map[string]chan *RemoteEvent),
	}
}

// Start initializes the NATS connection and JetStream.
func (t *NATSTransport) Start(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.isReady {
		return nil
	}

	// Connect to NATS
	nc, err := nats.Connect(t.config.NATSURL,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(10),
		nats.ReconnectWait(1*time.Second),
	)
	if err != nil {
		t.logger.Error("failed to connect to NATS", "url", t.config.NATSURL, "error", err)
		return fmt.Errorf("nats connect: %w", err)
	}
	t.conn = nc

	// Create JetStream context
	js, err := jetstream.New(nc)
	if err != nil {
		t.logger.Error("failed to create JetStream", "error", err)
		nc.Close()
		return fmt.Errorf("jetstream: %w", err)
	}
	t.js = js

	// Create streams for events and RPC
	if err := t.createStreams(ctx); err != nil {
		t.logger.Error("failed to create streams", "error", err)
		nc.Close()
		return fmt.Errorf("create streams: %w", err)
	}

	t.isReady = true
	t.logger.Info("NATS transport connected", "url", t.config.NATSURL)

	// Start discovery subscription
	go t.subscribeToDiscovery(context.Background())

	// Start RPC subscription
	go t.subscribeToRPC(context.Background())

	// Start heartbeat
	go t.heartbeat(context.Background())

	return nil
}

// Stop gracefully closes the NATS connection.
func (t *NATSTransport) Stop(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.isReady {
		return nil
	}

	if t.conn != nil {
		t.conn.Close()
	}

	t.isReady = false
	t.logger.Info("NATS transport disconnected")
	return nil
}

// IsReady returns true if the transport is operational.
func (t *NATSTransport) IsReady() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.isReady && t.conn != nil && t.conn.IsConnected()
}

// Publish sends an event via NATS JetStream.
func (t *NATSTransport) Publish(ctx context.Context, nodeID string, event *RemoteEvent) (*RemoteEvent, error) {
	if !t.IsReady() {
		return nil, fmt.Errorf("transport not ready")
	}

	// Create RPC response channel
	respChan := make(chan *RemoteEvent, 1)
	requestID := event.RequestId

	t.rpcMu.Lock()
	t.rpcHandlers[requestID] = respChan
	t.rpcMu.Unlock()

	defer func() {
		t.rpcMu.Lock()
		delete(t.rpcHandlers, requestID)
		t.rpcMu.Unlock()
	}()

	// Publish to target node's event stream
	subj := fmt.Sprintf("events.%s", nodeID)
	data, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	_, err = t.js.Publish(ctx, subj, data)
	if err != nil {
		t.logger.Error("publish failed", "subject", subj, "error", err)
		return nil, fmt.Errorf("publish: %w", err)
	}

	// Wait for response or timeout
	select {
	case resp := <-respChan:
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(10 * time.Second):
		return nil, fmt.Errorf("rpc timeout")
	}
}

// Subscribe registers an event handler.
func (t *NATSTransport) Subscribe(handler func(ctx context.Context, event *RemoteEvent) (*RemoteEvent, error)) error {
	t.mu.Lock()
	t.eventHandler = handler
	t.mu.Unlock()
	return nil
}

// GetServiceDirectory returns all known nodes.
func (t *NATSTransport) GetServiceDirectory(ctx context.Context) ([]*ServiceDiscoveryEntry, error) {
	t.nodesMu.RLock()
	defer t.nodesMu.RUnlock()

	entries := make([]*ServiceDiscoveryEntry, 0, len(t.nodes))
	for _, entry := range t.nodes {
		entries = append(entries, entry)
	}
	return entries, nil
}

// RegisterNode advertises this node.
func (t *NATSTransport) RegisterNode(ctx context.Context, entry *ServiceDiscoveryEntry) error {
	t.nodesMu.Lock()
	defer t.nodesMu.Unlock()

	if entry == nil {
		return fmt.Errorf("nil entry")
	}

	t.nodes[entry.NodeId] = entry
	t.logger.Debug("registered node", "id", entry.NodeId)
	return nil
}

// Health checks a remote node's health via NATS.
func (t *NATSTransport) Health(ctx context.Context, nodeID string) (bool, error) {
	if !t.IsReady() {
		return false, fmt.Errorf("transport not ready")
	}

	subj := fmt.Sprintf("health.%s", nodeID)
	resp, err := t.conn.Request(subj, []byte("ping"), 5*time.Second)
	if err != nil {
		return false, err
	}

	return string(resp.Data) == "pong", nil
}

// ListNodes returns all known node IDs.
func (t *NATSTransport) ListNodes() []string {
	t.nodesMu.RLock()
	defer t.nodesMu.RUnlock()

	nodes := make([]string, 0, len(t.nodes))
	for id := range t.nodes {
		nodes = append(nodes, id)
	}
	return nodes
}

// NodeInfo returns info about a specific node.
func (t *NATSTransport) NodeInfo(nodeID string) *ServiceDiscoveryEntry {
	t.nodesMu.RLock()
	defer t.nodesMu.RUnlock()
	return t.nodes[nodeID]
}

// ─── Private Methods ───────────────────────────────────────────────

// createStreams creates necessary JetStream streams.
func (t *NATSTransport) createStreams(ctx context.Context) error {
	// Event stream for all events
	config := jetstream.StreamConfig{
		Name:     "EVENTS",
		Subjects: []string{"events.>"},
		MaxAge:   24 * time.Hour,
		Storage:  jetstream.FileStorage,
		Replicas: 1,
	}

	_, err := t.js.CreateStream(ctx, config)
	if err != nil && err.Error() != "stream already exists" {
		return fmt.Errorf("create event stream: %w", err)
	}

	// Discovery stream
	config = jetstream.StreamConfig{
		Name:     "DISCOVERY",
		Subjects: []string{"discovery.>"},
		MaxAge:   1 * time.Hour,
		Storage:  jetstream.FileStorage,
		Replicas: 1,
	}

	_, err = t.js.CreateStream(ctx, config)
	if err != nil && err.Error() != "stream already exists" {
		return fmt.Errorf("create discovery stream: %w", err)
	}

	return nil
}

// subscribeToDiscovery subscribes to service discovery announcements.
func (t *NATSTransport) subscribeToDiscovery(ctx context.Context) {
	subj := "discovery.>"

	// Create consumer for discovery stream
	cons, err := t.js.CreateOrUpdateConsumer(ctx, "DISCOVERY", jetstream.ConsumerConfig{
		FilterSubject: subj,
		DeliverPolicy: jetstream.DeliverAllPolicy,
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil {
		t.logger.Error("discovery consumer creation failed", "subject", subj, "error", err)
		return
	}

	t.logger.Debug("subscribed to discovery", "subject", subj)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msgs, err := cons.Fetch(1, jetstream.FetchMaxWait(5*time.Second))
		if err != nil {
			t.logger.Debug("discovery fetch failed", "error", err)
			continue
		}

		for msg := range msgs.Messages() {
			var entry ServiceDiscoveryEntry
			if err := json.Unmarshal(msg.Data(), &entry); err != nil {
				t.logger.Debug("unmarshal discovery failed", "error", err)
				_ = msg.Ack()
				continue
			}

			_ = t.RegisterNode(ctx, &entry)
			_ = msg.Ack()
		}
	}
}

// subscribeToRPC subscribes to event delivery for this node.
func (t *NATSTransport) subscribeToRPC(ctx context.Context) {
	subj := fmt.Sprintf("events.%s", t.config.NodeID)

	// Create consumer for RPC events
	cons, err := t.js.CreateOrUpdateConsumer(ctx, "EVENTS", jetstream.ConsumerConfig{
		FilterSubject: subj,
		DeliverPolicy: jetstream.DeliverNewPolicy,
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil {
		t.logger.Error("rpc consumer creation failed", "subject", subj, "error", err)
		return
	}

	t.logger.Debug("subscribed to events", "subject", subj)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msgs, err := cons.Fetch(1, jetstream.FetchMaxWait(5*time.Second))
		if err != nil {
			t.logger.Debug("event fetch failed", "error", err)
			continue
		}

		for msg := range msgs.Messages() {
			var event RemoteEvent
			if err := json.Unmarshal(msg.Data(), &event); err != nil {
				t.logger.Debug("unmarshal event failed", "error", err)
				_ = msg.Ack()
				continue
			}

			// Call handler
			t.mu.RLock()
			handler := t.eventHandler
			t.mu.RUnlock()

			if handler != nil {
				resp, err := handler(ctx, &event)
				if err != nil {
					t.logger.Debug("handler error", "error", err)
				} else if resp != nil {
					// Send response via RPC handler channel
					t.rpcMu.RLock()
					respChan, ok := t.rpcHandlers[event.RequestId]
					t.rpcMu.RUnlock()

					if ok {
						select {
						case respChan <- resp:
						default:
							t.logger.Debug("response channel full")
						}
					}
				}
			}

			_ = msg.Ack()
		}
	}
}

// heartbeat publishes periodic service discovery announcements.
func (t *NATSTransport) heartbeat(ctx context.Context) {
	ticker := time.NewTicker(t.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.publishHeartbeat(ctx)
		}
	}
}

// publishHeartbeat announces this node's availability.
func (t *NATSTransport) publishHeartbeat(ctx context.Context) {
	if !t.IsReady() {
		return
	}

	// Use configured capabilities or default fallback
	caps := t.config.Capabilities
	if len(caps) == 0 {
		caps = []string{"provider_coordinator", "tool_coordinator"}
	}

	entry := &ServiceDiscoveryEntry{
		NodeId:       t.config.NodeID,
		NodeAddress:  t.config.ListenAddr,
		StartedAtMs:  time.Now().UnixMilli(),
		Capabilities: caps,
		HeartbeatMs:  time.Now().UnixMilli(),
		Status:       "healthy",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.logger.Debug("marshal heartbeat failed", "error", err)
		return
	}

	subj := fmt.Sprintf("discovery.%s", t.config.NodeID)
	_, err = t.js.Publish(ctx, subj, data)
	if err != nil {
		t.logger.Debug("publish heartbeat failed", "error", err)
	}
}
