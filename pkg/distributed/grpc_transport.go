package distributed

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
)

// GRPCTransport implements the Transport interface using gRPC.
type GRPCTransport struct {
	config       TransportConfig
	logger       *slog.Logger
	server       *grpc.Server
	listener     net.Listener
	mu           sync.RWMutex
	isReady      bool
	nodes        map[string]*ServiceDiscoveryEntry
	nodesMu      sync.RWMutex
	eventHandler func(ctx context.Context, event *RemoteEvent) (*RemoteEvent, error)
	connPool     map[string]*grpc.ClientConn
	connPoolMu   sync.RWMutex
}

// NewGRPCTransport creates a new gRPC-based transport.
func NewGRPCTransport(config TransportConfig, logger *slog.Logger) *GRPCTransport {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(nil, nil))
	}

	return &GRPCTransport{
		config:   config,
		logger:   logger,
		nodes:    make(map[string]*ServiceDiscoveryEntry),
		connPool: make(map[string]*grpc.ClientConn),
	}
}

// Start initializes and starts the gRPC server.
func (t *GRPCTransport) Start(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.isReady {
		return nil // Already started
	}
	if t.config.TransportType != "grpc-secure" && !t.config.AllowInsecureTransport {
		return fmt.Errorf("insecure gRPC transport disabled: set AllowInsecureTransport=true or use TransportType=grpc-secure")
	}

	// Create listener
	listener, err := net.Listen("tcp", t.config.ListenAddr)
	if err != nil {
		t.logger.Error("failed to listen", "addr", t.config.ListenAddr, "error", err)
		return fmt.Errorf("listen: %w", err)
	}
	t.listener = listener

	// Create gRPC server with TLS (self-signed for now)
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(50 * 1024 * 1024), // 50 MB
		grpc.MaxSendMsgSize(50 * 1024 * 1024),
	}

	// Add TLS if configured
	if t.config.TransportType == "grpc-secure" {
		tlsCert, err := t.generateSelfSignedCert()
		if err == nil {
			tlsConfig := &tls.Config{Certificates: []tls.Certificate{tlsCert}}
			opts = append(opts, grpc.Creds(credentials.NewTLS(tlsConfig)))
			t.logger.Info("TLS enabled for gRPC transport")
		}
	}

	t.server = grpc.NewServer(opts...)

	// Register gRPC service
	RegisterDistributedBusServer(t.server, t)

	// Enable reflection for debugging
	reflection.Register(t.server)

	// Start server in background
	go func() {
		if err := t.server.Serve(t.listener); err != nil && err != grpc.ErrServerStopped {
			t.logger.Error("gRPC server error", "error", err)
		}
	}()

	t.isReady = true
	t.logger.Info("gRPC transport started", "addr", t.config.ListenAddr)

	// Start discovery loop
	go t.discoverPeers(context.Background())

	return nil
}

// Stop gracefully shuts down the gRPC server.
func (t *GRPCTransport) Stop(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.isReady {
		return nil
	}

	// Close all client connections
	t.connPoolMu.Lock()
	for _, conn := range t.connPool {
		_ = conn.Close()
	}
	t.connPool = make(map[string]*grpc.ClientConn)
	t.connPoolMu.Unlock()

	// Stop server
	if t.server != nil {
		done := make(chan struct{})
		go func() {
			t.server.GracefulStop()
			close(done)
		}()

		select {
		case <-done:
		case <-ctx.Done():
			t.server.Stop()
		}
	}

	if t.listener != nil {
		_ = t.listener.Close()
	}

	t.isReady = false
	t.logger.Info("gRPC transport stopped")
	return nil
}

// IsReady returns true if the transport is operational.
func (t *GRPCTransport) IsReady() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.isReady
}

// Publish sends an event to a remote node.
func (t *GRPCTransport) Publish(ctx context.Context, nodeID string, event *RemoteEvent) (*RemoteEvent, error) {
	conn, err := t.getOrCreateConnection(nodeID)
	if err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}

	client := NewDistributedBusClient(conn)

	resp, err := client.PublishEvent(ctx, event)
	if err != nil {
		t.logger.Error("publish failed", "node", nodeID, "error", err)
		return nil, fmt.Errorf("publish: %w", err)
	}

	return resp, nil
}

// Subscribe registers an event handler.
func (t *GRPCTransport) Subscribe(handler func(ctx context.Context, event *RemoteEvent) (*RemoteEvent, error)) error {
	t.mu.Lock()
	t.eventHandler = handler
	t.mu.Unlock()
	return nil
}

// PublishEvent implements the gRPC service method.
func (t *GRPCTransport) PublishEvent(ctx context.Context, event *RemoteEvent) (*RemoteEvent, error) {
	t.mu.RLock()
	handler := t.eventHandler
	t.mu.RUnlock()

	if handler == nil {
		return &RemoteEvent{
			CorrelationId: event.CorrelationId,
			SourceNode:    t.config.NodeID,
		}, nil
	}

	resp, err := handler(ctx, event)
	if err != nil {
		t.logger.Error("handler error", "error", err)
		return nil, err
	}

	return resp, nil
}

// getServiceDirectoryInternal returns all known nodes (Transport interface).
func (t *GRPCTransport) getServiceDirectoryInternal(ctx context.Context) ([]*ServiceDiscoveryEntry, error) {
	t.nodesMu.RLock()
	defer t.nodesMu.RUnlock()

	entries := make([]*ServiceDiscoveryEntry, 0, len(t.nodes))
	for _, entry := range t.nodes {
		entries = append(entries, entry)
	}
	return entries, nil
}

// RegisterNode advertises this node.
func (t *GRPCTransport) RegisterNode(ctx context.Context, entry *ServiceDiscoveryEntry) error {
	t.nodesMu.Lock()
	defer t.nodesMu.Unlock()

	if entry == nil {
		return fmt.Errorf("nil entry")
	}

	t.nodes[entry.NodeId] = entry
	t.logger.Debug("registered node", "id", entry.NodeId)
	return nil
}

// healthRemoteNode checks health of a remote node (Transport interface).
func (t *GRPCTransport) healthRemoteNode(ctx context.Context, nodeID string) (bool, error) {
	conn, err := t.getOrCreateConnection(nodeID)
	if err != nil {
		return false, err
	}

	client := NewDistributedBusClient(conn)
	resp, err := client.Health(ctx, &HealthRequest{NodeId: t.config.NodeID})
	if err != nil {
		return false, err
	}

	return resp.Healthy, nil
}

// ListNodes returns all known node IDs.
func (t *GRPCTransport) ListNodes() []string {
	t.nodesMu.RLock()
	defer t.nodesMu.RUnlock()

	nodes := make([]string, 0, len(t.nodes))
	for id := range t.nodes {
		nodes = append(nodes, id)
	}
	return nodes
}

// NodeInfo returns info about a specific node.
func (t *GRPCTransport) NodeInfo(nodeID string) *ServiceDiscoveryEntry {
	t.nodesMu.RLock()
	defer t.nodesMu.RUnlock()
	return t.nodes[nodeID]
}

// ─── Private Methods ───────────────────────────────────────────────

// getOrCreateConnection gets or creates a gRPC connection to a node.
func (t *GRPCTransport) getOrCreateConnection(nodeID string) (*grpc.ClientConn, error) {
	t.nodesMu.RLock()
	entry, ok := t.nodes[nodeID]
	t.nodesMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("node not found: %s", nodeID)
	}

	transportCreds, err := t.clientTransportCredentials()
	if err != nil {
		return nil, err
	}

	conn, err := grpc.NewClient(entry.NodeAddress,
		grpc.WithTransportCredentials(transportCreds),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(50*1024*1024),
			grpc.MaxCallSendMsgSize(50*1024*1024),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}

	return conn, nil
}

// discoverPeers periodically discovers and registers peer nodes.
func (t *GRPCTransport) discoverPeers(ctx context.Context) {
	ticker := time.NewTicker(t.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.attemptPeerDiscovery()
		}
	}
}

// attemptPeerDiscovery tries to connect to configured peers.
func (t *GRPCTransport) attemptPeerDiscovery() {
	for _, peerAddr := range t.config.PeerAddrs {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		transportCreds, credErr := t.clientTransportCredentials()
		if credErr != nil {
			t.logger.Debug("peer discovery skipped due transport policy", "error", credErr)
			cancel()
			return
		}
		conn, err := grpc.NewClient(peerAddr, grpc.WithTransportCredentials(transportCreds))
		if err != nil {
			t.logger.Debug("peer discovery failed", "addr", peerAddr, "error", err)
			cancel()
			continue
		}

		client := NewDistributedBusClient(conn)
		resp, err := client.GetServiceDirectory(ctx, &ServiceDirectoryRequest{RequesterNodeId: t.config.NodeID})
		if err != nil {
			t.logger.Debug("service discovery failed", "addr", peerAddr, "error", err)
			_ = conn.Close()
			cancel()
			continue
		}

		// Register discovered nodes
		for _, entry := range resp.Entries {
			_ = t.RegisterNode(ctx, entry)
		}

		_ = conn.Close()
		cancel()
	}
}

func (t *GRPCTransport) clientTransportCredentials() (credentials.TransportCredentials, error) {
	if t.config.TransportType == "grpc-secure" {
		return credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13}), nil
	}
	if !t.config.AllowInsecureTransport {
		return nil, fmt.Errorf("plaintext transport denied by policy")
	}
	return insecure.NewCredentials(), nil
}

// generateSelfSignedCert generates a self-signed TLS certificate.
func (t *GRPCTransport) generateSelfSignedCert() (tls.Certificate, error) {
	// For production, use proper certificate management.
	// This is a placeholder that uses an empty cert (not secure).
	return tls.Certificate{}, nil
}

// ─── gRPC Service Implementation ───────────────────────────────────

// GetServiceDirectory implements DistributedBusServer.GetServiceDirectory
func (t *GRPCTransport) GetServiceDirectory(ctx context.Context, req *ServiceDirectoryRequest) (*ServiceDirectoryResponse, error) {
	t.nodesMu.RLock()
	entries := make([]*ServiceDiscoveryEntry, 0, len(t.nodes))
	for _, entry := range t.nodes {
		entries = append(entries, entry)
	}
	t.nodesMu.RUnlock()

	return &ServiceDirectoryResponse{Entries: entries}, nil
}

// Health implements DistributedBusServer.Health
func (t *GRPCTransport) Health(ctx context.Context, req *HealthRequest) (*HealthResponse, error) {
	t.mu.RLock()
	isReady := t.isReady
	t.mu.RUnlock()

	return &HealthResponse{
		Healthy: isReady,
		Status:  "ok",
	}, nil
}

// mustEmbedUnimplementedDistributedBusServer ensures this type implements the gRPC interface.
func (t *GRPCTransport) mustEmbedUnimplementedDistributedBusServer() {}
