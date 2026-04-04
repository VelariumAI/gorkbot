package webui

import (
	"encoding/json"
	"testing"
	"time"
)

func TestWSHub_BroadcastReachesClient(t *testing.T) {
	// Note: Full WebSocket integration tests require live HTTP servers
	// This test is simplified to test the Broadcast mechanism
	hub := NewWSHub()
	go hub.Run()

	// Create a mock client
	client := &WSClient{hub: hub, send: make(chan []byte, 64)}
	hub.register <- client
	time.Sleep(50 * time.Millisecond)

	// Broadcast a message
	msg := map[string]interface{}{
		"type": "test",
		"data": "hello",
	}
	err := hub.Broadcast(msg)
	if err != nil {
		t.Fatalf("Failed to broadcast: %v", err)
	}

	// Check that message was queued to broadcast channel
	select {
	case data := <-client.send:
		var received map[string]interface{}
		json.Unmarshal(data, &received)
		if received["type"] != "test" {
			t.Errorf("Expected test message, got %v", received)
		}
	case <-time.After(1 * time.Second):
		t.Error("Timeout waiting for message")
	}
}

func TestWSHub_ClientCount(t *testing.T) {
	hub := NewWSHub()
	go hub.Run()

	if hub.ClientCount() != 0 {
		t.Errorf("Expected 0 clients, got %d", hub.ClientCount())
	}

	// Simulate registering clients
	for i := 0; i < 3; i++ {
		client := &WSClient{hub: hub, send: make(chan []byte, 64)}
		hub.register <- client
		time.Sleep(50 * time.Millisecond)
	}

	if hub.ClientCount() != 3 {
		t.Errorf("Expected 3 clients, got %d", hub.ClientCount())
	}

	// Unregister one
	clients := make([]*WSClient, 0)
	hub.mu.RLock()
	for client := range hub.clients {
		clients = append(clients, client)
	}
	hub.mu.RUnlock()

	if len(clients) > 0 {
		hub.unregister <- clients[0]
		time.Sleep(50 * time.Millisecond)
	}

	if hub.ClientCount() != 2 {
		t.Errorf("Expected 2 clients, got %d", hub.ClientCount())
	}
}

func TestWSHub_BroadcastJSON(t *testing.T) {
	hub := NewWSHub()

	payload := map[string]interface{}{
		"type": "token",
		"payload": map[string]interface{}{
			"run_id":   "run_123",
			"token":    "hello",
			"sequence": 1,
		},
	}

	err := hub.Broadcast(payload)
	if err != nil {
		t.Fatalf("Failed to broadcast: %v", err)
	}

	// Verify it was queued
	select {
	case msg := <-hub.broadcast:
		var decoded map[string]interface{}
		json.Unmarshal(msg, &decoded)
		if decoded["type"] != "token" {
			t.Errorf("Expected type 'token', got %v", decoded["type"])
		}
	case <-time.After(1 * time.Second):
		t.Error("Broadcast timeout")
	}
}

func TestWSHub_NilBroadcast(t *testing.T) {
	hub := NewWSHub()
	go hub.Run()

	// Should not panic on valid JSON
	err := hub.Broadcast(nil)
	if err != nil {
		t.Fatalf("Failed to broadcast nil: %v", err)
	}
}
