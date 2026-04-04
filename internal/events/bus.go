package events

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// BusEvent is the base interface for all internal coordinator bus events.
// Each event carries a correlation ID for tracing across domains.
// This is distinct from the audit Event struct used for event sourcing.
type BusEvent interface {
	// CorrelationID returns the unique ID linking related events across the system.
	CorrelationID() string
	// Timestamp returns when the event was created.
	Timestamp() time.Time
}

// BaseEvent provides common fields for all events.
type BaseEvent struct {
	correlationID string
	timestamp     time.Time
}

// NewBaseEvent creates a base event with a new correlation ID.
func NewBaseEvent() BaseEvent {
	return BaseEvent{
		correlationID: uuid.New().String(),
		timestamp:     time.Now(),
	}
}

// NewBaseEventWithID creates a base event with an explicit correlation ID.
func NewBaseEventWithID(corrID string) BaseEvent {
	return BaseEvent{
		correlationID: corrID,
		timestamp:     time.Now(),
	}
}

func (e BaseEvent) CorrelationID() string {
	return e.correlationID
}

func (e BaseEvent) Timestamp() time.Time {
	return e.timestamp
}

// Handler processes a bus event and may return a response event (or nil).
type Handler func(ctx context.Context, event BusEvent) BusEvent

// BusPublisher is a generic interface for event bus implementations.
// Both *events.Bus and *distributed.DistributedBus implement this interface.
type BusPublisher interface {
	Publish(ctx context.Context, event BusEvent) BusEvent
	Register(eventType string, handler Handler)
}

// Bus is a typed internal event bus for coordinator communication.
// All inter-coordinator messages flow through the bus instead of shared-state mutation.
// The bus is synchronous: handlers execute in order and may return response events.
type Bus struct {
	handlers map[string][]Handler
	mu       sync.RWMutex
}

// NewBus creates a new event bus.
func NewBus() *Bus {
	return &Bus{
		handlers: make(map[string][]Handler),
	}
}

// Register registers a handler for a specific event type (by type name).
// Multiple handlers can be registered for the same event type.
func (b *Bus) Register(eventType string, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

// Publish synchronously dispatches an event to all registered handlers.
// Returns the response from the last handler (or nil if no handlers matched).
// Handlers execute in registration order.
func (b *Bus) Publish(ctx context.Context, event BusEvent) BusEvent {
	eventType := EventTypeName(event)

	b.mu.RLock()
	handlers := b.handlers[eventType]
	b.mu.RUnlock()

	var response BusEvent
	for _, h := range handlers {
		response = h(ctx, event)
	}

	return response
}

// EventTypeName returns the string key for an event type.
// Uses type assertion to determine the concrete type name.
func EventTypeName(event BusEvent) string {
	// This is a simple implementation; could be enhanced with custom naming.
	t := EventTypeOf(event)
	return t
}

// EventTypeOf returns the type name of an event (simplified).
// In a real implementation, this could use reflection or explicit type registration.
func EventTypeOf(event BusEvent) string {
	switch event.(type) {
	case *ProviderEvent:
		return "ProviderEvent"
	case *ProviderHealthEvent:
		return "ProviderHealthEvent"
	case *ProviderFailoverEvent:
		return "ProviderFailoverEvent"
	case *ToolRequestEvent:
		return "ToolRequestEvent"
	case *ToolResultEvent:
		return "ToolResultEvent"
	case *MemoryQueryEvent:
		return "MemoryQueryEvent"
	case *MemoryResultEvent:
		return "MemoryResultEvent"
	case *CompressionEvent:
		return "CompressionEvent"
	case *ImprovementProposalEvent:
		return "ImprovementProposalEvent"
	case *RollbackEvent:
		return "RollbackEvent"
	default:
		return "UnknownEvent"
	}
}

// ─── Provider Coordinator Events ───────────────────────────────────────────

// ProviderEvent is raised when a provider is initialized or status changes.
type ProviderEvent struct {
	BaseEvent
	ProviderID string // "xai", "google", etc.
	Action     string // "init", "ready", "error"
	Error      string // non-empty if action is "error"
}

// ProviderHealthEvent is raised periodically to report provider health.
type ProviderHealthEvent struct {
	BaseEvent
	ProviderID string
	Healthy    bool
	Latency    time.Duration
	Error      string
}

// ProviderFailoverEvent is raised when the system switches to a backup provider.
type ProviderFailoverEvent struct {
	BaseEvent
	FromProvider string
	ToProvider   string
	Reason       string
}

// ─── Tool Coordinator Events ───────────────────────────────────────────────

// ToolRequestEvent is raised when a tool is about to be executed.
type ToolRequestEvent struct {
	BaseEvent
	ToolName   string
	Parameters map[string]interface{}
	RequestID  string // unique ID for this tool invocation
}

// ToolResultEvent is raised when a tool execution completes.
type ToolResultEvent struct {
	BaseEvent
	ToolName   string
	RequestID  string
	Success    bool
	Output     string
	Error      string
	DurationMS int64
}

// ─── Memory Coordinator Events ─────────────────────────────────────────────

// MemoryQueryEvent is raised when a component needs to query the memory system.
type MemoryQueryEvent struct {
	BaseEvent
	QueryType string // "search", "retrieve_fact", "list_engrams", etc.
	Query     string
	RequestID string // RPC correlation ID (empty → auto-generated on publish)
}

// MemoryResultEvent is returned by MemoryCoordinator in response to MemoryQueryEvent.
type MemoryResultEvent struct {
	BaseEvent
	RequestID string // matches MemoryQueryEvent.RequestID
	QueryType string
	Result    string
	LatencyMS int64
	Error     string
}

// CompressionEvent is raised when the memory system needs to compress context.
type CompressionEvent struct {
	BaseEvent
	Source     string // "conversation_history", "audit_log", etc.
	TargetSize int    // desired max size in tokens
}

// ─── Self-Improve Coordinator Events ───────────────────────────────────────

// ImprovementProposalEvent is raised by the SI driver with a proposed optimization.
type ImprovementProposalEvent struct {
	BaseEvent
	ProposalID string
	Target     string // tool name, feature name, or system component
	Type       string // "code_change", "parameter_tune", "workflow_optimization"
	Confidence int    // 0-100
	RiskLevel  string // "low", "medium", "high"
	Details    string // human-readable description
}

// RollbackEvent is raised when an improvement must be reverted.
type RollbackEvent struct {
	BaseEvent
	ProposalID string
	Reason     string
}
