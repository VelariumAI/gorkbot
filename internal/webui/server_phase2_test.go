package webui

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/velariumai/gorkbot/internal/designsystem"
)

// TestServer_ThemeTokensEndpoint tests /api/theme/tokens.css endpoint.
func TestServer_ThemeTokensEndpoint(t *testing.T) {
	// Initialize design system
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := designsystem.Init(logger); err != nil {
		t.Fatalf("Failed to initialize design system: %v", err)
	}

	// Create test server
	gin.SetMode(gin.TestMode)
	router := gin.New()

	server := &Server{logger: logger}
	router.GET("/api/theme/tokens.css", server.handleThemeTokens)

	// Test request
	req := httptest.NewRequest("GET", "/api/theme/tokens.css", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/css; charset=utf-8" {
		t.Errorf("expected Content-Type text/css, got %s", contentType)
	}

	body := w.Body.String()
	if body == "" {
		t.Errorf("theme endpoint returned empty body")
	}

	// Verify CSS variables are present
	expectedVars := []string{
		"--color-bg-canvas",
		"--color-accent-primary",
		"--space-base",
	}
	for _, v := range expectedVars {
		if !containsString(body, v) {
			t.Errorf("theme endpoint missing CSS variable: %s", v)
		}
	}
}

// TestServer_WorkspacesEndpoint tests /api/workspaces endpoint.
func TestServer_WorkspacesEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	server := &Server{}
	router.GET("/api/workspaces", server.handleWorkspaces)

	req := httptest.NewRequest("GET", "/api/workspaces", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var workspaces []WorkspaceInfo
	if err := json.Unmarshal(w.Body.Bytes(), &workspaces); err != nil {
		t.Errorf("failed to parse workspaces JSON: %v", err)
	}

	expectedWorkspaces := 7
	if len(workspaces) != expectedWorkspaces {
		t.Errorf("expected %d workspaces, got %d", expectedWorkspaces, len(workspaces))
	}

	workspaceIds := []string{"chat", "tasks", "tools", "agents", "memory", "analytics", "settings"}
	for i, id := range workspaceIds {
		if i < len(workspaces) && workspaces[i].ID != id {
			t.Errorf("expected workspace[%d].ID=%s, got %s", i, id, workspaces[i].ID)
		}
	}
}

// TestServer_RunsEndpoint tests /api/runs endpoint.
func TestServer_RunsEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	server := &Server{}
	router.GET("/api/runs", server.handleRuns)

	req := httptest.NewRequest("GET", "/api/runs", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var runs []RunInfo
	if err := json.Unmarshal(w.Body.Bytes(), &runs); err != nil {
		t.Errorf("failed to parse runs JSON: %v", err)
	}

	// Initially empty
	if len(runs) != 0 {
		t.Errorf("expected empty runs list initially, got %d", len(runs))
	}
}

// TestServer_RunDetailsEndpoint tests /api/entities/runs/:id endpoint.
func TestServer_RunDetailsEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	server := &Server{}
	router.GET("/api/entities/runs/:id", server.handleRunDetails)

	req := httptest.NewRequest("GET", "/api/entities/runs/test-123", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Errorf("failed to parse response JSON: %v", err)
	}

	if response["id"] != "test-123" {
		t.Errorf("expected id=test-123, got %v", response["id"])
	}
}

// TestServer_AnalyticsMetricsEndpoint tests /api/analytics/metrics endpoint.
func TestServer_AnalyticsMetricsEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	server := &Server{}
	router.GET("/api/analytics/metrics", server.handleAnalyticsMetrics)

	req := httptest.NewRequest("GET", "/api/analytics/metrics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var metrics map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &metrics); err != nil {
		t.Errorf("failed to parse metrics JSON: %v", err)
	}

	expectedKeys := []string{"provider_latency", "tool_success_rate", "token_usage"}
	for _, key := range expectedKeys {
		if _, ok := metrics[key]; !ok {
			t.Errorf("metrics missing expected key: %s", key)
		}
	}
}

// Helper function to check if string contains substring
func containsString(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr))
}
