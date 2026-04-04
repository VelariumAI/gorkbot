package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

var logger = slog.New(slog.NewTextHandler(os.Stderr, nil))

// ────────────────────────────────────────────────────────────
// API Client Tests
// ────────────────────────────────────────────────────────────

// TestNewClient creates a client.
func TestNewClient(t *testing.T) {
	client := NewClient("http://localhost:8080", logger)

	if client == nil {
		t.Errorf("NewClient returned nil")
	}
	if client.baseURL != "http://localhost:8080" {
		t.Errorf("expected baseURL=http://localhost:8080, got %s", client.baseURL)
	}
}

// TestClient_SetHeader sets custom headers.
func TestClient_SetHeader(t *testing.T) {
	client := NewClient("http://localhost:8080", logger)
	client.SetHeader("X-Custom", "value")

	if client.headers["X-Custom"] != "value" {
		t.Errorf("header not set")
	}
}

// TestClient_SetAuthToken sets authorization.
func TestClient_SetAuthToken(t *testing.T) {
	client := NewClient("http://localhost:8080", logger)
	client.SetAuthToken("test-token")

	if client.headers["Authorization"] != "Bearer test-token" {
		t.Errorf("auth token not set correctly")
	}
}

// TestClient_SendChat sends a chat message.
func TestClient_SendChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("expected /api/chat, got %s", r.URL.Path)
		}

		response := ChatResponse{
			ID:      "resp-1",
			Message: "Test response",
			Tokens:  42,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, logger)
	resp, err := client.SendChat(context.Background(), &ChatRequest{
		Prompt: "Test prompt",
	})

	if err != nil {
		t.Errorf("SendChat failed: %v", err)
	}
	if resp.ID != "resp-1" {
		t.Errorf("expected ID=resp-1, got %s", resp.ID)
	}
	if resp.Message != "Test response" {
		t.Errorf("expected message='Test response', got %s", resp.Message)
	}
}

// TestClient_GetRuns retrieves runs.
func TestClient_GetRuns(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		runs := []map[string]interface{}{
			{"id": "run-1", "status": "complete"},
			{"id": "run-2", "status": "running"},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(runs)
	}))
	defer server.Close()

	client := NewClient(server.URL, logger)
	runs, err := client.GetRuns(context.Background(), 10)

	if err != nil {
		t.Errorf("GetRuns failed: %v", err)
	}
	if len(runs) != 2 {
		t.Errorf("expected 2 runs, got %d", len(runs))
	}
}

// TestClient_GetWorkspaces retrieves workspaces.
func TestClient_GetWorkspaces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		workspaces := []map[string]interface{}{
			{"id": "chat", "name": "Chat"},
			{"id": "tools", "name": "Tools"},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(workspaces)
	}))
	defer server.Close()

	client := NewClient(server.URL, logger)
	workspaces, err := client.GetWorkspaces(context.Background())

	if err != nil {
		t.Errorf("GetWorkspaces failed: %v", err)
	}
	if len(workspaces) != 2 {
		t.Errorf("expected 2 workspaces, got %d", len(workspaces))
	}
}

// TestClient_HealthCheck verifies connectivity.
func TestClient_HealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, logger)
	err := client.HealthCheck(context.Background())

	if err != nil {
		t.Errorf("HealthCheck failed: %v", err)
	}
}

// TestClient_HealthCheck_Failure detects connection errors.
func TestClient_HealthCheck_Failure(t *testing.T) {
	client := NewClient("http://invalid-host-that-does-not-exist:9999", logger)
	err := client.HealthCheck(context.Background())

	if err == nil {
		t.Errorf("expected error for invalid host")
	}
}

// ────────────────────────────────────────────────────────────
// WebSocket Message Tests
// ────────────────────────────────────────────────────────────

// TestMessage_MarshalJSON serializes messages.
func TestMessage_MarshalJSON(t *testing.T) {
	msg := Message{
		Type: "token",
		ID:   "msg-1",
		Payload: map[string]interface{}{
			"token": "hello",
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Errorf("MarshalJSON failed: %v", err)
	}

	if len(data) == 0 {
		t.Errorf("serialized message is empty")
	}
}

// TestTokenMessage represents token streaming.
func TestTokenMessage(t *testing.T) {
	msg := TokenMessage{
		RunID:    "run-1",
		Token:    "hello",
		Sequence: 1,
	}

	if msg.RunID != "run-1" {
		t.Errorf("expected RunID=run-1")
	}
	if msg.Token != "hello" {
		t.Errorf("expected Token=hello")
	}
}

// TestRunStatusMessage represents run updates.
func TestRunStatusMessage(t *testing.T) {
	msg := RunStatusMessage{
		RunID:  "run-1",
		Status: "complete",
	}

	if msg.Status != "complete" {
		t.Errorf("expected Status=complete")
	}
}

// ────────────────────────────────────────────────────────────
// State Manager Tests
// ────────────────────────────────────────────────────────────

// TestNewStateManager creates a state manager.
func TestNewStateManager(t *testing.T) {
	client := NewClient("http://localhost:8080", logger)
	sm := NewStateManager(client, nil)

	if sm == nil {
		t.Errorf("NewStateManager returned nil")
	}

	state := sm.GetState()
	if state == nil {
		t.Errorf("GetState returned nil")
	}
	if state.ActiveWorkspace != "chat" {
		t.Errorf("expected ActiveWorkspace=chat, got %s", state.ActiveWorkspace)
	}
}

// TestStateManager_UpdateState updates state.
func TestStateManager_UpdateState(t *testing.T) {
	client := NewClient("http://localhost:8080", logger)
	sm := NewStateManager(client, nil)

	sm.UpdateState(func(s *AppState) {
		s.ActiveWorkspace = "tools"
		s.CurrentModel = "Grok 3"
	})

	state := sm.GetState()
	if state.ActiveWorkspace != "tools" {
		t.Errorf("expected ActiveWorkspace=tools, got %s", state.ActiveWorkspace)
	}
	if state.CurrentModel != "Grok 3" {
		t.Errorf("expected CurrentModel=Grok 3, got %s", state.CurrentModel)
	}
}

// TestStateManager_OnStateChange notifies of changes.
func TestStateManager_OnStateChange(t *testing.T) {
	client := NewClient("http://localhost:8080", logger)
	sm := NewStateManager(client, nil)

	ch := sm.OnStateChange()

	go sm.UpdateState(func(s *AppState) {
		s.IsLoading = true
	})

	select {
	case state := <-ch:
		if !state.IsLoading {
			t.Errorf("expected IsLoading=true")
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timeout waiting for state change")
	}
}

// TestStateManager_SyncRuns syncs run data.
func TestStateManager_SyncRuns(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		runs := []map[string]interface{}{
			{"id": "run-1", "status": "complete"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(runs)
	}))
	defer server.Close()

	client := NewClient(server.URL, logger)
	sm := NewStateManager(client, nil)

	err := sm.SyncRuns(context.Background())
	if err != nil {
		t.Errorf("SyncRuns failed: %v", err)
	}

	state := sm.GetState()
	if len(state.Runs) != 1 {
		t.Errorf("expected 1 run, got %d", len(state.Runs))
	}
}

// TestStateManager_SessionID generates unique session IDs.
func TestStateManager_SessionID(t *testing.T) {
	client := NewClient("http://localhost:8080", logger)
	sm1 := NewStateManager(client, nil)
	sm2 := NewStateManager(client, nil)

	state1 := sm1.GetState()
	state2 := sm2.GetState()

	if state1.SessionID == state2.SessionID {
		t.Errorf("expected different session IDs")
	}

	if state1.SessionID == "" {
		t.Errorf("session ID is empty")
	}
}

// TestToBase62 encodes numbers.
func TestToBase62(t *testing.T) {
	result := toBase62(123456)

	if result == "" {
		t.Errorf("toBase62 returned empty string")
	}

	if result == "0" {
		t.Errorf("expected non-zero encoding, got 0")
	}
}

// TestGenerateSessionID creates unique IDs.
func TestGenerateSessionID(t *testing.T) {
	id1 := generateSessionID()
	id2 := generateSessionID()

	if id1 == id2 {
		t.Errorf("expected different session IDs")
	}

	if !contains(id1, "sess_") {
		t.Errorf("session ID missing sess_ prefix")
	}
}

// Helper function
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
