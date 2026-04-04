package distributed

import (
	"context"
	"encoding/json"
	"time"
)

// Transport defines the interface for network-based event delivery.
// Implementations can use gRPC, NATS, or other transports.
type Transport interface {
	// Start initializes and starts the transport.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the transport.
	Stop(ctx context.Context) error

	// IsReady returns true if the transport is initialized and operational.
	IsReady() bool

	// Publish sends an event to a remote coordinator.
	// Returns the response event or an error.
	Publish(ctx context.Context, nodeID string, event *RemoteEvent) (*RemoteEvent, error)

	// Subscribe registers a handler for incoming events.
	// The handler is called with any event delivered to this node.
	Subscribe(handler func(ctx context.Context, event *RemoteEvent) (*RemoteEvent, error)) error

	// GetServiceDirectory returns all known nodes and their capabilities.
	GetServiceDirectory(ctx context.Context) ([]*ServiceDiscoveryEntry, error)

	// RegisterNode advertises this node's capabilities to the network.
	RegisterNode(ctx context.Context, entry *ServiceDiscoveryEntry) error

	// Health performs a health check against a remote node.
	Health(ctx context.Context, nodeID string) (bool, error)

	// ListNodes returns all known node IDs.
	ListNodes() []string

	// NodeInfo returns the discovery entry for a specific node (or nil if not found).
	NodeInfo(nodeID string) *ServiceDiscoveryEntry
}

// RemoteEventWrapper adds metadata needed for network delivery.
type RemoteEventWrapper struct {
	Event     *RemoteEvent
	Retry     int
	CreatedAt time.Time
	Deadline  time.Time
}

// EventSource defines the interface for retrieving and storing events.
type EventSource interface {
	// Store persists an event in the event log.
	// Returns the assigned sequence number or an error.
	Store(ctx context.Context, entry *EventStoreEntry) (int64, error)

	// Load retrieves an event by ID.
	Load(ctx context.Context, eventID string) (*EventStoreEntry, error)

	// Query returns all events matching criteria.
	// Empty filters return all events.
	Query(ctx context.Context, eventType, correlationID string, sinceMS int64) ([]*EventStoreEntry, error)

	// Tail returns the last N events in order.
	Tail(ctx context.Context, limit int) ([]*EventStoreEntry, error)

	// Size returns the total number of events stored.
	Size(ctx context.Context) (int64, error)

	// Hash returns the hash of all events (for integrity verification).
	Hash(ctx context.Context) (string, error)

	// Compact removes old events (configurable by age or count).
	Compact(ctx context.Context, keepMS int64) error

	// Close releases resources.
	Close() error
}

// TransportConfig contains configuration for distributed deployments.
type TransportConfig struct {
	// NodeID is the unique identifier for this node in the cluster.
	NodeID string

	// ListenAddr is the local address to listen for incoming connections (host:port).
	ListenAddr string

	// PeerAddrs is a list of known peer addresses to connect to (for discovery).
	PeerAddrs []string

	// Transport selects the backend: "grpc", "nats", or "hybrid"
	TransportType string

	// AllowInsecureTransport permits plaintext gRPC transport when TransportType
	// is not "grpc-secure". Default false: callers must opt in explicitly.
	AllowInsecureTransport bool

	// NATS server URL (required if TransportType is "nats" or "hybrid")
	NATSURL string

	// Heartbeat interval for node discovery and health checks.
	HeartbeatInterval time.Duration

	// MaxRetries for failed event delivery.
	MaxRetries int

	// EventSourceType selects the backend: "memory" or "sqlite"
	EventSourceType string

	// SQLitePath is the path to the SQLite database (required if EventSourceType is "sqlite")
	SQLitePath string

	// Capabilities advertised by this node (empty → default to ["provider_coordinator", "tool_coordinator"])
	Capabilities []string
}

// ToProto converts RemoteEvent to its protobuf representation.
func (e *RemoteEvent) ToProto() *RemoteEvent {
	return e // Already in proto format
}

// FromJSON unmarshals JSON payload into a concrete event.
func (e *RemoteEvent) FromJSON(v interface{}) error {
	return json.Unmarshal(e.Payload, v)
}

// ToJSON marshals a concrete event into JSON payload.
func (e *RemoteEvent) ToJSON(v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	e.Payload = data
	return nil
}
