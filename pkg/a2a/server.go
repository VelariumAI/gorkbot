package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ----------------------------------------------------------------------------
// TaskRunnerFunc
// ----------------------------------------------------------------------------

// TaskRunnerFunc is called when a new task arrives via A2A HTTP.
// It should execute the prompt and return the AI response.
type TaskRunnerFunc func(ctx context.Context, prompt string) (string, error)

// ----------------------------------------------------------------------------
// A2ATask & TaskStore
// ----------------------------------------------------------------------------

// A2ATask holds the lifecycle state of a single agent task.
type A2ATask struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"` // submitted | working | completed | failed | canceled
	Prompt    string    `json:"prompt"`
	Result    string    `json:"result,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	cancel context.CancelFunc `json:"-"`
}

// TaskStore is a thread-safe in-memory store for A2ATask objects.
type TaskStore struct {
	mu    sync.RWMutex
	tasks map[string]*A2ATask
}

func newTaskStore() *TaskStore {
	return &TaskStore{tasks: make(map[string]*A2ATask)}
}

func (ts *TaskStore) create(prompt string) *A2ATask {
	id := fmt.Sprintf("task_%d", time.Now().UnixNano())
	now := time.Now().UTC()
	t := &A2ATask{
		ID:        id,
		Status:    "submitted",
		Prompt:    prompt,
		CreatedAt: now,
		UpdatedAt: now,
	}
	ts.mu.Lock()
	ts.tasks[id] = t
	ts.mu.Unlock()
	return t
}

func (ts *TaskStore) get(id string) (*A2ATask, bool) {
	ts.mu.RLock()
	t, ok := ts.tasks[id]
	ts.mu.RUnlock()
	return t, ok
}

func (ts *TaskStore) update(id, status, result, errMsg string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if t, ok := ts.tasks[id]; ok {
		t.Status = status
		if result != "" {
			t.Result = result
		}
		if errMsg != "" {
			t.Error = errMsg
		}
		t.UpdatedAt = time.Now().UTC()
	}
}

func (ts *TaskStore) setCancel(id string, cancel context.CancelFunc) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if t, ok := ts.tasks[id]; ok {
		t.cancel = cancel
	}
}

func (ts *TaskStore) cancel(id string) bool {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	t, ok := ts.tasks[id]
	if !ok {
		return false
	}
	if t.cancel != nil {
		t.cancel()
	}
	if t.Status != "completed" && t.Status != "failed" {
		t.Status = "canceled"
		t.UpdatedAt = time.Now().UTC()
	}
	return true
}

// snapshot returns a value copy safe for JSON serialisation (no cancel func).
func (ts *TaskStore) snapshot(t *A2ATask) A2ATask {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return A2ATask{
		ID:        t.ID,
		Status:    t.Status,
		Prompt:    t.Prompt,
		Result:    t.Result,
		Error:     t.Error,
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
	}
}

// ----------------------------------------------------------------------------
// JSON-RPC 2.0 wire types
// ----------------------------------------------------------------------------

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func rpcOK(id json.RawMessage, result interface{}) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: id, Result: result}
}

func rpcErr(id json.RawMessage, code int, msg string) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}}
}

// ----------------------------------------------------------------------------
// Server
// ----------------------------------------------------------------------------

// Server is the A2A HTTP gateway server.
type Server struct {
	addr    string
	token   string
	tasks   *TaskStore
	runner  TaskRunnerFunc
	logger  *slog.Logger
	httpSrv *http.Server
	mu      sync.RWMutex
}

// NewServer creates a new A2A HTTP server.
// addr is the bind address (e.g. "127.0.0.1:18890").
// token is the Bearer token required on POST /a2a/v1; empty means no auth.
// runner is called with each incoming prompt and returns the response.
func NewServer(addr, token string, runner TaskRunnerFunc, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{
		addr:   addr,
		token:  token,
		tasks:  newTaskStore(),
		runner: runner,
		logger: logger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/agent.json", s.handleAgentCard)
	mux.HandleFunc("GET /a2a/health", s.handleHealth)
	mux.HandleFunc("POST /a2a/v1", s.handleRPC)

	s.httpSrv = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 5 * time.Minute, // tasks may be long
		IdleTimeout:  120 * time.Second,
	}
	return s
}

// Start begins listening. It returns immediately; the server runs in a goroutine
// and shuts down when ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("a2a gateway listening", "addr", s.addr)
		if err := s.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Watch for context cancellation to trigger graceful shutdown.
	go func() {
		<-ctx.Done()
		_ = s.Stop()
	}()

	// Give the listener a moment to fail fast (e.g. port in use).
	select {
	case err := <-errCh:
		return fmt.Errorf("a2a server start: %w", err)
	case <-time.After(150 * time.Millisecond):
		return nil
	}
}

// Stop gracefully shuts down the HTTP server.
func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.logger.Info("a2a gateway shutting down")
	return s.httpSrv.Shutdown(ctx)
}

// ----------------------------------------------------------------------------
// Handlers
// ----------------------------------------------------------------------------

func (s *Server) handleAgentCard(w http.ResponseWriter, r *http.Request) {
	setCORSPublic(w)
	w.Header().Set("Content-Type", "application/json")

	scheme := "http"
	host := s.addr

	card := map[string]interface{}{
		"name":        "Gorkbot",
		"description": "Multi-provider AI orchestration agent",
		"url":         scheme + "://" + host,
		"version":     "2.8.0",
		"capabilities": map[string]interface{}{
			"streaming":         true,
			"pushNotifications": false,
		},
		"defaultInputModes":  []string{"text/plain"},
		"defaultOutputModes": []string{"text/plain"},
		"skills": []map[string]interface{}{
			{
				"id":          "chat",
				"name":        "Chat",
				"description": "General conversation and task execution",
			},
			{
				"id":          "tools",
				"name":        "Tools",
				"description": "Execute tools: file ops, web search, git, bash, etc.",
			},
		},
	}
	writeJSON(w, http.StatusOK, card)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	setCORSPublic(w)
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, http.StatusOK, map[string]string{
		"status":    "ok",
		"version":   "2.8.0",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleRPC(w http.ResponseWriter, r *http.Request) {
	// CORS — restrict to loopback origins for POST.
	origin := r.Header.Get("Origin")
	if origin != "" && !isLoopbackOrigin(origin) {
		http.Error(w, "forbidden origin", http.StatusForbidden)
		return
	}
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	}

	// Handle preflight.
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Auth check.
	if s.token != "" {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") || strings.TrimPrefix(authHeader, "Bearer ") != s.token {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(rpcErr(json.RawMessage(`null`), -32001, "unauthorized"))
			return
		}
	}

	// Decode JSON-RPC request.
	var req rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONRPC(w, http.StatusBadRequest, rpcErr(json.RawMessage(`null`), -32700, "parse error: "+err.Error()))
		return
	}
	if req.JSONRPC != "2.0" {
		writeJSONRPC(w, http.StatusBadRequest, rpcErr(req.ID, -32600, "invalid request: jsonrpc must be '2.0'"))
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch req.Method {
	case "message/send":
		s.rpcMessageSend(w, req)
	case "tasks/get":
		s.rpcTasksGet(w, req)
	case "tasks/cancel":
		s.rpcTasksCancel(w, req)
	default:
		writeJSONRPC(w, http.StatusOK, rpcErr(req.ID, -32601, "method not found: "+req.Method))
	}
}

// rpcMessageSend handles the message/send JSON-RPC method.
func (s *Server) rpcMessageSend(w http.ResponseWriter, req rpcRequest) {
	// Parse params: {"message": {"parts": [{"text": "..."}]}}
	var params struct {
		Message struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"message"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPC(w, http.StatusOK, rpcErr(req.ID, -32602, "invalid params: "+err.Error()))
		return
	}

	// Concatenate all text parts.
	var sb strings.Builder
	for _, p := range params.Message.Parts {
		sb.WriteString(p.Text)
	}
	prompt := strings.TrimSpace(sb.String())
	if prompt == "" {
		writeJSONRPC(w, http.StatusOK, rpcErr(req.ID, -32602, "invalid params: empty prompt"))
		return
	}

	// Create task and respond immediately with submitted status.
	task := s.tasks.create(prompt)

	// Run the task asynchronously.
	taskCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	s.tasks.setCancel(task.ID, cancel)

	go func() {
		defer cancel()
		s.tasks.update(task.ID, "working", "", "")
		s.logger.Info("a2a task started", "id", task.ID)

		result, err := s.runner(taskCtx, prompt)
		if err != nil {
			s.logger.Warn("a2a task failed", "id", task.ID, "err", err)
			s.tasks.update(task.ID, "failed", "", err.Error())
			return
		}
		s.logger.Info("a2a task completed", "id", task.ID)
		s.tasks.update(task.ID, "completed", result, "")
	}()

	snap := s.tasks.snapshot(task)
	writeJSONRPC(w, http.StatusOK, rpcOK(req.ID, snap))
}

// rpcTasksGet handles the tasks/get JSON-RPC method.
func (s *Server) rpcTasksGet(w http.ResponseWriter, req rpcRequest) {
	var params struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPC(w, http.StatusOK, rpcErr(req.ID, -32602, "invalid params: "+err.Error()))
		return
	}
	if params.ID == "" {
		writeJSONRPC(w, http.StatusOK, rpcErr(req.ID, -32602, "invalid params: id required"))
		return
	}

	task, ok := s.tasks.get(params.ID)
	if !ok {
		writeJSONRPC(w, http.StatusOK, rpcErr(req.ID, -32001, "task not found: "+params.ID))
		return
	}
	snap := s.tasks.snapshot(task)
	writeJSONRPC(w, http.StatusOK, rpcOK(req.ID, snap))
}

// rpcTasksCancel handles the tasks/cancel JSON-RPC method.
func (s *Server) rpcTasksCancel(w http.ResponseWriter, req rpcRequest) {
	var params struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPC(w, http.StatusOK, rpcErr(req.ID, -32602, "invalid params: "+err.Error()))
		return
	}
	if params.ID == "" {
		writeJSONRPC(w, http.StatusOK, rpcErr(req.ID, -32602, "invalid params: id required"))
		return
	}

	if !s.tasks.cancel(params.ID) {
		writeJSONRPC(w, http.StatusOK, rpcErr(req.ID, -32001, "task not found: "+params.ID))
		return
	}

	task, _ := s.tasks.get(params.ID)
	snap := s.tasks.snapshot(task)
	writeJSONRPC(w, http.StatusOK, rpcOK(req.ID, snap))
}

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONRPC(w http.ResponseWriter, code int, resp rpcResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(resp)
}

func setCORSPublic(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

// isLoopbackOrigin returns true if the Origin header references localhost or 127.0.0.1.
func isLoopbackOrigin(origin string) bool {
	return strings.HasPrefix(origin, "http://localhost") ||
		strings.HasPrefix(origin, "http://127.0.0.1") ||
		strings.HasPrefix(origin, "https://localhost") ||
		strings.HasPrefix(origin, "https://127.0.0.1")
}
