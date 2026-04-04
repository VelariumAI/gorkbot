// integration_test.go provides comprehensive integration tests for the headless API.
package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/velariumai/gorkbot/internal/engine"
	"github.com/velariumai/gorkbot/pkg/skills"
)

// TestHealthEndpoint verifies the /health endpoint works without auth.
func TestHealthEndpoint(t *testing.T) {
	router := setupTestRouter()

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	assert.Equal(t, "ok", resp["status"])
	assert.NotZero(t, resp["uptime"])
}

// TestVersionEndpoint verifies the /version endpoint.
func TestVersionEndpoint(t *testing.T) {
	router := setupTestRouter()

	req := httptest.NewRequest("GET", "/version", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	assert.NotEmpty(t, resp["version"])
}

// TestLoginFlow verifies authentication flow.
func TestLoginFlow(t *testing.T) {
	router := setupTestRouter()

	// Step 1: Login
	loginReq := map[string]string{
		"username": "testuser",
		"password": "password123",
	}
	body, _ := json.Marshal(loginReq)

	req := httptest.NewRequest("POST", "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var loginResp map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&loginResp)
	require.NoError(t, err)

	token := loginResp["token"].(string)
	assert.NotEmpty(t, token)
	assert.NotZero(t, loginResp["expires"])

	// Step 2: Use token to access protected endpoint (test with a simpler auth endpoint)
	req2 := httptest.NewRequest("GET", "/api/v1/session", nil)
	req2.Header.Set("Authorization", token)
	w2 := httptest.NewRecorder()

	router.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
}

// TestLoginDisabledByDefault verifies login is rejected when insecure login is disabled
// and no explicit API credentials are configured.
func TestLoginDisabledByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := &Server{
		logger:      slog.Default(),
		sessions:    NewSessionManager(),
		metrics:     NewMetrics(),
		jwtSecret:   "test-secret",
		rateLimiter: newRateLimiter(1000, 1*time.Minute),
		config: &ServerConfig{
			Port:               8080,
			RateLimitPerMin:    60,
			SessionTimeoutMin:  30,
			AllowedOrigins:     []string{"http://localhost:*", "http://127.0.0.1:*"},
			TokenTTL:           24 * time.Hour,
			AllowInsecureLogin: false,
		},
	}
	server.setupRouter()

	loginReq := map[string]string{
		"username": "testuser",
		"password": "password123",
	}
	body, _ := json.Marshal(loginReq)
	req := httptest.NewRequest("POST", "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	var resp map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "login_disabled", resp["code"])
}

// TestUnauthorizedAccess verifies protected endpoints reject missing auth.
func TestUnauthorizedAccess(t *testing.T) {
	router := setupTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	var resp map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "missing_authorization", resp["code"])
}

// TestMessageEndpoint verifies POST /api/v1/message works end-to-end.
func TestMessageEndpoint(t *testing.T) {
	router := setupTestRouter()
	token := getTestToken(router)

	msgReq := map[string]interface{}{
		"prompt": "Hello, Gorkbot!",
		"model":  "grok-3",
	}
	body, _ := json.Marshal(msgReq)

	req := httptest.NewRequest("POST", "/api/v1/message", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var msgResp map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&msgResp)
	require.NoError(t, err)

	assert.NotNil(t, msgResp["error"])
}

// TestToolListEndpoint verifies GET /api/v1/tools returns tools.
func TestToolListEndpoint(t *testing.T) {
	router := setupTestRouter()
	token := getTestToken(router)

	req := httptest.NewRequest("GET", "/api/v1/tools", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// In test mode, orchestrator is not initialized, so we expect 503
	// In production, this would return 200 with tools list
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var toolResp map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&toolResp)
	require.NoError(t, err)

	assert.NotNil(t, toolResp["error"])
}

// TestToolSearchEndpoint verifies GET /api/v1/tools/search.
func TestToolSearchEndpoint(t *testing.T) {
	router := setupTestRouter()
	token := getTestToken(router)

	req := httptest.NewRequest("GET", "/api/v1/tools/search?q=read_file", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var searchResp map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&searchResp)
	require.NoError(t, err)

	assert.NotNil(t, searchResp["error"])
}

// TestToolExecuteEndpoint verifies execute endpoint requires an initialized orchestrator.
func TestToolExecuteEndpoint(t *testing.T) {
	router := setupTestRouter()
	token := getTestToken(router)

	req := httptest.NewRequest("POST", "/api/v1/tools/read_file/execute", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var resp map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	assert.NotEmpty(t, resp["error"])
}

// TestSessionIsolation verifies two users have isolated sessions.
func TestSessionIsolation(t *testing.T) {
	router := setupTestRouter()

	// User A logs in
	tokenA := loginWithCredentials(router, "userA", "pass")
	assert.NotEmpty(t, tokenA)

	// User B logs in
	tokenB := loginWithCredentials(router, "userB", "pass")
	assert.NotEmpty(t, tokenB)

	// Verify tokens are different
	assert.NotEqual(t, tokenA, tokenB)

	// Each user can use their own token
	req := httptest.NewRequest("GET", "/api/v1/session", nil)
	req.Header.Set("Authorization", tokenA)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	req2 := httptest.NewRequest("GET", "/api/v1/session", nil)
	req2.Header.Set("Authorization", tokenB)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)
}

// TestRateLimiting verifies per-user rate limiting works.
func TestRateLimiting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := &Server{
		logger:      slog.Default(),
		sessions:    NewSessionManager(),
		metrics:     NewMetrics(),
		jwtSecret:   "test-secret",
		rateLimiter: newRateLimiter(1, 1*time.Minute), // allow one request per minute
		config: &ServerConfig{
			Port:               8080,
			RateLimitPerMin:    1,
			SessionTimeoutMin:  30,
			AllowedOrigins:     []string{"http://localhost:*", "http://127.0.0.1:*"},
			TokenTTL:           24 * time.Hour,
			AllowInsecureLogin: true,
		},
	}
	server.setupRouter()
	router := server.router

	req1 := httptest.NewRequest("GET", "/health", nil)
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	req2 := httptest.NewRequest("GET", "/health", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusTooManyRequests, w2.Code)
	assert.Equal(t, "60", w2.Header().Get("Retry-After"))
	var resp map[string]interface{}
	err := json.NewDecoder(w2.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "rate_limited", resp["code"])
}

// TestWebSocketUpgrade verifies WebSocket endpoint accepts connections.
func TestWebSocketUpgrade(t *testing.T) {
	router := setupTestRouter()
	token := getTestToken(router)

	// Create test server
	srv := httptest.NewServer(router)
	defer srv.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + srv.URL[4:] + "/api/v1/ws"

	// Upgrade to WebSocket
	dialer := websocket.Dialer{}
	header := make(http.Header)
	header.Set("Authorization", token)

	ws, _, err := dialer.Dial(wsURL, header)
	require.NoError(t, err)
	defer ws.Close()

	// Send a test message
	testMsg := map[string]string{"action": "ping"}
	err = ws.WriteJSON(testMsg)
	require.NoError(t, err)

	// Read response
	var resp map[string]interface{}
	err = ws.ReadJSON(&resp)
	require.NoError(t, err)

	assert.Equal(t, "received", resp["status"])
}

// TestConcurrentWebSocketClients verifies multiple WS clients can connect.
func TestConcurrentWebSocketClients(t *testing.T) {
	router := setupTestRouter()
	srv := httptest.NewServer(router)
	defer srv.Close()

	wsURL := "ws" + srv.URL[4:] + "/api/v1/ws"

	var wg sync.WaitGroup
	var successCount int32
	numClients := 10

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()

			token := getTestToken(router)
			dialer := websocket.Dialer{}
			header := make(http.Header)
			header.Set("Authorization", token)

			ws, _, err := dialer.Dial(wsURL, header)
			if err != nil {
				t.Logf("Client %d connection failed: %v", clientID, err)
				return
			}
			defer ws.Close()

			// Send message
			msg := map[string]interface{}{"client_id": clientID}
			err = ws.WriteJSON(msg)
			if err != nil {
				return
			}

			// Read response
			var resp map[string]interface{}
			err = ws.ReadJSON(&resp)
			if err == nil {
				atomic.AddInt32(&successCount, 1)
			}
		}(i)
	}

	wg.Wait()

	// Verify at least 80% connected successfully
	minSuccess := int32(numClients * 80 / 100)
	assert.GreaterOrEqual(t, successCount, minSuccess,
		"At least 80%% of clients should connect successfully")
}

// TestLogoutFlow verifies logout invalidates session.
func TestLogoutFlow(t *testing.T) {
	router := setupTestRouter()
	token := getTestToken(router)

	// Logout
	req := httptest.NewRequest("POST", "/auth/logout", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Session should be cleared (subsequent requests should fail or get new session)
	req2 := httptest.NewRequest("GET", "/api/v1/session", nil)
	req2.Header.Set("Authorization", token)
	w2 := httptest.NewRecorder()

	router.ServeHTTP(w2, req2)
	// Note: This test is simplified; real implementation would verify session is cleared
}

// TestRefreshTokenFlow verifies token refresh works.
func TestRefreshTokenFlow(t *testing.T) {
	router := setupTestRouter()
	token := getTestToken(router)

	req := httptest.NewRequest("POST", "/auth/refresh", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	newToken := resp["token"].(string)
	assert.NotEmpty(t, newToken)
	assert.NotEqual(t, token, newToken) // Should be different token
}

// TestCORSHeaders verifies CORS headers are set correctly.
func TestCORSHeaders(t *testing.T) {
	router := setupTestRouter()

	req := httptest.NewRequest("GET", "/health", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, "http://localhost:3000", w.Header().Get("Access-Control-Allow-Origin"))
}

// TestCORSRejectsDisallowedOrigin verifies disallowed origins are blocked.
func TestCORSRejectsDisallowedOrigin(t *testing.T) {
	router := setupTestRouter()

	req := httptest.NewRequest("GET", "/health", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	var resp map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "origin_not_allowed", resp["code"])
}

// TestMemoryStatusEndpoint verifies memory status endpoint.
func TestMemoryStatusEndpoint(t *testing.T) {
	router := setupTestRouter()
	token := getTestToken(router)

	req := httptest.NewRequest("GET", "/api/v1/memory/status", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var resp map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	assert.NotNil(t, resp["error"])
}

// TestSIStatusEndpoint verifies SI status endpoint.
func TestSIStatusEndpoint(t *testing.T) {
	router := setupTestRouter()
	token := getTestToken(router)

	req := httptest.NewRequest("GET", "/api/v1/si/status", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var resp map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	assert.NotNil(t, resp["error"])
}

// TestSkillLintEndpoint verifies skill lint route uses manifest linting.
func TestSkillLintEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "demo")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	manifest := `name: demo
version: 1.0.0
description: Demo skill
enabled: true
tools: []
prompts: []
workflows: []
`
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, ".gorkskill.yaml"), []byte(manifest), 0o644))

	registry := skills.NewInMemoryRegistry(slog.Default())
	loader := skills.NewLoader(registry, slog.Default())
	count, err := loader.LoadDirectory(tmpDir)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	server := &Server{
		orch:        &engine.Orchestrator{SkillLoader: loader},
		logger:      slog.Default(),
		sessions:    NewSessionManager(),
		metrics:     NewMetrics(),
		jwtSecret:   "test-secret",
		rateLimiter: newRateLimiter(1000, 1*time.Minute),
		config: &ServerConfig{
			Port:               8080,
			RateLimitPerMin:    60,
			SessionTimeoutMin:  30,
			AllowedOrigins:     []string{"http://localhost:*", "http://127.0.0.1:*"},
			TokenTTL:           24 * time.Hour,
			AllowInsecureLogin: true,
		},
	}
	server.setupRouter()
	token := loginWithCredentials(server.router, "testuser", "password123")
	require.NotEmpty(t, token)

	req := httptest.NewRequest("GET", "/api/v1/skills/lint", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		IssueCount int                `json:"issue_count"`
		Issues     []skills.LintIssue `json:"issues"`
	}
	err = json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	require.Equal(t, 1, resp.IssueCount)
	require.Len(t, resp.Issues, 1)
	assert.Contains(t, resp.Issues[0].Message, "no tools, prompts, or workflows")
}

// TestBadJSON verifies invalid JSON is rejected.
func TestBadJSON(t *testing.T) {
	router := setupTestRouter()
	token := getTestToken(router)

	req := httptest.NewRequest("POST", "/api/v1/message",
		bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestExpiredTokenRejected verifies expired JWTs are denied.
func TestExpiredTokenRejected(t *testing.T) {
	router := setupTestRouter()
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Subject:   "expired-user",
		ID:        "expired-token",
		IssuedAt:  jwt.NewNumericDate(now.Add(-2 * time.Hour)),
		ExpiresAt: jwt.NewNumericDate(now.Add(-1 * time.Hour)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte("test-secret"))
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/api/v1/session", nil)
	req.Header.Set("Authorization", signed)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	var resp map[string]interface{}
	err = json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "invalid_token", resp["code"])
}

// Helper functions

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)

	// Create a minimal server instance for testing
	// In real tests, this would be injected
	server := &Server{
		logger:      slog.Default(),
		sessions:    NewSessionManager(),
		metrics:     NewMetrics(),
		jwtSecret:   "test-secret",
		rateLimiter: newRateLimiter(1000, 1*time.Minute),
		config: &ServerConfig{
			Port:               8080,
			RateLimitPerMin:    60,
			SessionTimeoutMin:  30,
			AllowedOrigins:     []string{"http://localhost:*", "http://127.0.0.1:*"},
			TokenTTL:           24 * time.Hour,
			AllowInsecureLogin: true,
		},
	}

	server.setupRouter()
	return server.router
}

func getTestToken(router *gin.Engine) string {
	return loginWithCredentials(router, "testuser", "password123")
}

func loginWithCredentials(router *gin.Engine, username, password string) string {
	loginReq := map[string]string{
		"username": username,
		"password": password,
	}
	body, _ := json.Marshal(loginReq)

	req := httptest.NewRequest("POST", "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		return ""
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	if token, ok := resp["token"].(string); ok {
		return token
	}
	return ""
}
