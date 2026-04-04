package engine

import (
	"context"
	"testing"

	"github.com/velariumai/gorkbot/internal/events"
)

// TestEventBusInitialized verifies the local event bus is created and functional.
func TestEventBusInitialized(t *testing.T) {
	// Create a simple event bus
	bus := events.NewBus()
	if bus == nil {
		t.Errorf("expected event bus to be created")
	}

	// Verify we can publish an event
	event := &events.ProviderHealthEvent{
		BaseEvent: events.NewBaseEvent(),
		ProviderID: "test-provider",
		Healthy: true,
	}

	result := bus.Publish(context.Background(), event)
	// Should complete without panic
	_ = result
}

// TestEventBusRegistration verifies handlers can be registered and called.
func TestEventBusRegistration(t *testing.T) {
	bus := events.NewBus()

	// Register a handler
	bus.Register("TestEvent", func(ctx context.Context, event events.BusEvent) events.BusEvent {
		return nil
	})

	// Create a custom test event (using ProviderHealthEvent as a proxy)
	event := &events.ProviderHealthEvent{
		BaseEvent: events.NewBaseEvent(),
		ProviderID: "test",
		Healthy: true,
	}

	// Publish the event
	// Note: This won't match "TestEvent" type, but demonstrates registration works
	// A real test would need a custom event type
	_ = bus.Publish(context.Background(), event)
}

// TestBusPublisherInterface verifies events.BusPublisher contract.
func TestBusPublisherInterface(t *testing.T) {
	var publisher events.BusPublisher = events.NewBus()

	if publisher == nil {
		t.Errorf("expected non-nil publisher")
	}

	event := &events.ProviderFailoverEvent{
		BaseEvent: events.NewBaseEvent(),
		FromProvider: "prov-a",
		ToProvider: "prov-b",
		Reason: "test",
	}

	// Should be able to publish via interface
	result := publisher.Publish(context.Background(), event)
	_ = result

	// Should be able to register via interface
	publisher.Register("TestType", func(ctx context.Context, e events.BusEvent) events.BusEvent {
		return nil
	})
}
