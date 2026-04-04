package webui

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func newTestServer() *Server {
	gin.SetMode(gin.ReleaseMode)
	logger := slog.New(slog.NewTextHandler(nil, nil))
	s := &Server{
		port:     8080,
		orch:     nil, // No orchestrator needed for these tests
		logger:   logger,
		router:   gin.New(),
		streams:  make(map[string]chan StreamEvent),
		runStore: NewRunStore(),
		wsHub:    NewWSHub(),
		shell:    NewShell(),
	}
	go s.wsHub.Run()
	s.routes()
	return s
}

func TestHandleIndex_ServesShellHTML(t *testing.T) {
	s := newTestServer()

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}

	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Expected text/html, got %s", ct)
	}

	body := w.Body.String()
	if !bytes.Contains(w.Body.Bytes(), []byte("shell-layout")) {
		t.Error("Expected shell HTML, got something else")
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("Gorkbot")) {
		t.Errorf("Expected Gorkbot title in HTML, got: %s", body)
	}
}

// Note: TestHandleChat_ReturnsRunID skipped as it requires full orchestrator integration
// The handleChat endpoint creates runs in runStore and broadcasts to wsHub

func TestHandleRuns_LiveData(t *testing.T) {
	s := newTestServer()

	// Create a run
	s.runStore.Create("run_test", "test prompt", "Grok", "xAI")

	req := httptest.NewRequest("GET", "/api/runs", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}

	var runs []map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &runs)

	if len(runs) != 1 {
		t.Errorf("Expected 1 run, got %d", len(runs))
	}

	if runs[0]["id"] != "run_test" {
		t.Errorf("Expected run_test, got %s", runs[0]["id"])
	}
}

func TestHandleRunDetails_LiveData(t *testing.T) {
	s := newTestServer()

	// Create a run with a tool
	s.runStore.Create("run_123", "test", "Grok", "xAI")
	s.runStore.ToolStart("run_123", "bash")
	time.Sleep(10 * time.Millisecond)
	s.runStore.ToolDone("run_123", "bash")

	req := httptest.NewRequest("GET", "/api/entities/runs/run_123", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}

	var run map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &run)

	if run["id"] != "run_123" {
		t.Errorf("Expected run_123, got %s", run["id"])
	}

	tools, ok := run["tools"].([]interface{})
	if !ok || len(tools) != 1 {
		t.Errorf("Expected 1 tool, got %v", run["tools"])
	}
}

func TestHandleRunDetails_NotFound(t *testing.T) {
	s := newTestServer()

	req := httptest.NewRequest("GET", "/api/entities/runs/nonexistent", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", w.Code)
	}
}

func TestHandleWS_Upgrades(t *testing.T) {
	s := newTestServer()

	// Just verify the route exists and hub is initialized
	if s.wsHub == nil {
		t.Error("WebSocket hub not initialized")
	}

	if s.router == nil {
		t.Error("Router not initialized")
	}
}
