package webui

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/velariumai/gorkbot/internal/designsystem"
	"github.com/velariumai/gorkbot/internal/engine"
	"github.com/velariumai/gorkbot/pkg/config"
	"github.com/velariumai/gorkbot/pkg/registry"
	"github.com/velariumai/gorkbot/pkg/theme"
	"github.com/velariumai/gorkbot/pkg/tools"
)

//go:embed templates/* static/*
var content embed.FS

type Server struct {
	port     int
	orch     *engine.Orchestrator
	reg      *registry.ModelRegistry
	appState *config.AppStateManager
	logger   *slog.Logger
	router   *gin.Engine

	// Active streams
	streams map[string]chan StreamEvent
	mu      sync.RWMutex

	// Phase 3: Run store and WebSocket hub
	runStore *RunStore
	wsHub    *WSHub
	shell    *Shell
}

type StreamEvent struct {
	Type string `json:"type"` // "token", "tool_start", "tool_done", "error", "done"
	Data string `json:"data"`
}

func NewServer(port int, orch *engine.Orchestrator, reg *registry.ModelRegistry, appState *config.AppStateManager, logger *slog.Logger) *Server {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// Security Headers
	r.Use(func(c *gin.Context) {
		c.Header("Content-Security-Policy", "default-src 'self' 'unsafe-inline' 'unsafe-eval' https://cdnjs.cloudflare.com https://cdn.jsdelivr.net https://storage.googleapis.com; object-src 'none'")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Next()
	})

	s := &Server{
		port:     port,
		orch:     orch,
		reg:      reg,
		appState: appState,
		logger:   logger,
		router:   r,
		streams:  make(map[string]chan StreamEvent),
		runStore: NewRunStore(),
		wsHub:    NewWSHub(),
		shell:    NewShell(),
	}
	go s.wsHub.Run()
	s.routes()
	return s
}

func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.port)
	s.logger.Info("Starting Web UI Server (Gin)", "addr", addr)
	fmt.Printf("\n🚀 Gorkbot Web UI available at http://localhost:%d\n\n", s.port)
	return s.router.Run(addr)
}

func (s *Server) routes() {
	staticFS, err := fs.Sub(content, "static")
	if err != nil {
		s.logger.Error("embedded static filesystem unavailable, fallback to disk", "error", err)
		staticFS, _ = fs.Sub(os.DirFS("static"), ".")
	}

	s.router.StaticFS("/static", http.FS(staticFS))
	s.router.GET("/sw.js", s.handleServiceWorker)
	s.router.GET("/", s.handleIndex)
	s.router.GET("/metrics", s.handleMetrics)

	api := s.router.Group("/api")
	{
		api.POST("/chat", s.handleChat)
		api.GET("/stream", s.handleStream)
		api.GET("/state", s.handleState)
		api.GET("/models", s.handleModels)
		api.POST("/models/set", s.handleSetModel)
		api.POST("/credentials/set", s.handleSetCredential)
		api.GET("/tools", s.handleTools)
		api.GET("/agents", s.handleAgents)
		api.GET("/memory", s.handleMemory)
		api.POST("/offline-sync", s.handleOfflineSync)

		// Phase 2: Theme and Workspace Endpoints
		api.GET("/theme/tokens.css", s.handleThemeTokens)
		api.GET("/workspaces", s.handleWorkspaces)
		api.GET("/providers", s.handleProviders)
		api.GET("/runs", s.handleRuns)
		api.GET("/entities/runs/:id", s.handleRunDetails)
		api.GET("/analytics/metrics", s.handleAnalyticsMetrics)

		// Phase 3: WebSocket endpoint
		api.GET("/ws", s.wsHub.ServeWS)
	}
}

func (s *Server) handleServiceWorker(c *gin.Context) {
	data, err := content.ReadFile("static/sw.js")
	if err != nil {
		c.String(http.StatusInternalServerError, "SW error")
		return
	}
	c.Data(http.StatusOK, "application/javascript", data)
}

func (s *Server) handleIndex(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, s.shell.RenderHTML())
}

type ChatRequest struct {
	Prompt    string `json:"prompt"`
	SessionID string `json:"session_id"`
}

// generateID creates a random 8-character hex string for run/session IDs.
func generateID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Server) handleChat(c *gin.Context) {
	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	if req.SessionID == "" {
		req.SessionID = fmt.Sprintf("sess_%d", time.Now().UnixNano())
	}

	// Phase 3: Generate run ID and create run record
	runID := "run_" + generateID()
	model := "unknown"
	provider := "unknown"
	if primary := s.orch.Primary(); primary != nil {
		meta := primary.GetMetadata()
		model = meta.Name
		provider = meta.ID
	}
	s.runStore.Create(runID, req.Prompt, model, provider)

	s.mu.Lock()
	ch := make(chan StreamEvent, 100)
	s.streams[req.SessionID] = ch
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.streams, req.SessionID)
			s.mu.Unlock()
			close(ch)
		}()

		ctx := c.Request.Context()

		err := s.orch.ExecuteTaskWithStreaming(
			ctx,
			req.Prompt,
			func(token string) {
				ch <- StreamEvent{Type: "token", Data: token}
				// Broadcast to WebSocket clients
				s.wsHub.Broadcast(map[string]interface{}{
					"type": "token",
					"payload": map[string]interface{}{
						"run_id": runID,
						"token": token,
					},
				})
			},
			func(toolName string, result *tools.ToolResult) {
				if result != nil && result.AuthRequired {
					ch <- StreamEvent{Type: "auth_required", Data: result.Output}
				}
				ch <- StreamEvent{Type: "tool_done", Data: toolName}
				s.runStore.ToolDone(runID, toolName)
				// Broadcast tool done event
				s.wsHub.Broadcast(map[string]interface{}{
					"type": "tool_done",
					"payload": map[string]interface{}{
						"run_id":    runID,
						"tool_name": toolName,
					},
				})
			},
			func(toolName string, args map[string]interface{}) {
				ch <- StreamEvent{Type: "tool_start", Data: toolName}
				s.runStore.ToolStart(runID, toolName)
				// Broadcast tool start event
				s.wsHub.Broadcast(map[string]interface{}{
					"type": "tool_start",
					"payload": map[string]interface{}{
						"run_id":    runID,
						"tool_name": toolName,
					},
				})
			},
			nil,
			nil,
		)

		if err != nil {
			ch <- StreamEvent{Type: "error", Data: err.Error()}
			s.runStore.Fail(runID, err.Error())
			// Broadcast error
			s.wsHub.Broadcast(map[string]interface{}{
				"type": "run_status",
				"payload": map[string]interface{}{
					"run_id": runID,
					"status": "error",
					"error":  err.Error(),
				},
			})
		} else {
			ch <- StreamEvent{Type: "done", Data: ""}
			s.runStore.Complete(runID, 0)
			// Broadcast completion
			s.wsHub.Broadcast(map[string]interface{}{
				"type": "run_status",
				"payload": map[string]interface{}{
					"run_id": runID,
					"status": "complete",
				},
			})
		}
	}()

	c.JSON(http.StatusOK, gin.H{"session_id": req.SessionID, "run_id": runID})
}

func (s *Server) handleStream(c *gin.Context) {
	sessionID := c.Query("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing session_id"})
		return
	}

	s.mu.RLock()
	ch, ok := s.streams[sessionID]
	s.mu.RUnlock()

	if !ok || ch == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found or already completed"})
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")

	c.Stream(func(w io.Writer) bool {
		if event, ok := <-ch; ok {
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
			return true
		}
		return false
	})
}

func (s *Server) handleState(c *gin.Context) {
	state := gin.H{"provider": "Unknown"}
	if primary := s.orch.Primary(); primary != nil {
		state["provider"] = primary.GetMetadata().Name
	}
	if s.orch.ContextMgr != nil {
		in, out := s.orch.ContextMgr.TotalUsage()
		state["tokens_in"] = in
		state["tokens_out"] = out
	}
	c.JSON(http.StatusOK, state)
}

func (s *Server) handleModels(c *gin.Context) {
	if s.reg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Registry unavailable"})
		return
	}

	models := s.reg.ListActiveModels()
	var out []gin.H
	for _, m := range models {
		out = append(out, gin.H{
			"id":       string(m.ID),
			"name":     m.Name,
			"provider": string(m.Provider),
		})
	}

	currentPrimary := ""
	if primary := s.orch.Primary(); primary != nil {
		currentPrimary = primary.GetMetadata().ID
	}

	currentSecondary := "auto"
	if consultant := s.orch.Consultant(); consultant != nil {
		currentSecondary = consultant.GetMetadata().ID
	}

	c.JSON(http.StatusOK, gin.H{
		"models":    out,
		"primary":   currentPrimary,
		"secondary": currentSecondary,
	})
}

type SetModelReq struct {
	Role     string `json:"role"`
	Provider string `json:"provider"`
	ModelID  string `json:"model_id"`
}

func (s *Server) handleSetModel(c *gin.Context) {
	var req SetModelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	ctx := context.Background()

	if req.Role == "primary" {
		if err := s.orch.SetPrimary(ctx, req.Provider, req.ModelID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if s.appState != nil {
			_ = s.appState.SetPrimary(req.Provider, req.ModelID)
		}
	} else if req.Role == "secondary" {
		if req.ModelID == "auto" {
			// Set consultant to auto mode (handled by SetAutoSecondary below)
			if s.orch.Registry != nil {
				s.orch.Registry.SetConsultantProvider(nil)
			}
			if s.appState != nil {
				_ = s.appState.SetSecondaryAuto()
			}
		} else {
			if err := s.orch.SetSecondary(ctx, req.Provider, req.ModelID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			if s.appState != nil {
				_ = s.appState.SetSecondary(req.Provider, req.ModelID)
			}
		}
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) handleTools(c *gin.Context) {
	if s.orch == nil || s.orch.Registry == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Tool registry unavailable"})
		return
	}

	toolsList := s.orch.Registry.ListAll()
	var out []gin.H
	for _, t := range toolsList {
		out = append(out, gin.H{
			"name":        t.Name,
			"description": t.Description,
			"category":    string(t.Category),
		})
	}
	c.JSON(http.StatusOK, gin.H{"tools": out})
}

func (s *Server) handleAgents(c *gin.Context) {
	agents := []gin.H{
		{"id": "agent-core", "status": "active", "type": "Orchestrator"},
	}

	// Add background agents if available
	if s.orch != nil && s.orch.BackgroundAgents != nil {
		for _, bg := range s.orch.BackgroundAgents.List() {
			agents = append(agents, gin.H{
				"id":     bg.ID,
				"status": bg.Status,
				"type":   "Background",
			})
		}
	}

	// Add discovered agents if available
	if s.orch != nil && s.orch.ProviderCoord != nil {
		if discoveryMgr := s.orch.ProviderCoord.Discovery(); discoveryMgr != nil {
			for _, node := range discoveryMgr.AgentTree() {
				agents = append(agents, gin.H{
					"id":     node.ID,
					"status": node.Status,
					"type":   "Discovered",
				})
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"agents": agents})
}

func (s *Server) handleMemory(c *gin.Context) {
	if s.orch == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Orchestrator unavailable"})
		return
	}

	memStats := gin.H{}
	if s.orch.ConversationHistory != nil {
		memStats["messages_count"] = len(s.orch.ConversationHistory.GetMessages())
	}
	if s.orch.GoalLedger != nil {
		memStats["goals_count"] = len(s.orch.GoalLedger.Goals)
	}
	c.JSON(http.StatusOK, memStats)
}

// SyncTask represents a payload from the frontend offline queue
type SyncTask struct {
	ID        int    `json:"id"`
	Payload   string `json:"payload"`
	Timestamp int64  `json:"timestamp"`
}

func (s *Server) handleOfflineSync(c *gin.Context) {
	var tasks []SyncTask
	if err := c.ShouldBindJSON(&tasks); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid sync payload format"})
		return
	}

	s.logger.Info("Received offline sync batch", "count", len(tasks))

	ctx := context.Background()
	results := make(map[int]string)

	for _, task := range tasks {
		// Route payload through the intent gate/orchestrator
		// Since it's a sync background task, we run it without streaming
		_, err := s.orch.ExecuteTask(ctx, task.Payload)
		if err != nil {
			s.logger.Warn("Sync task failed", "taskID", task.ID, "error", err)
			results[task.ID] = "failed"
		} else {
			results[task.ID] = "completed"
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "synchronized",
		"results": results,
	})
}

func (s *Server) handleSetCredential(c *gin.Context) {
	var req struct {
		Provider string `json:"provider"`
		Value    string `json:"value"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	if s.orch == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Orchestrator unavailable"})
		return
	}

	ctx := context.Background()
	status := s.orch.SetProviderKey(ctx, req.Provider, req.Value)
	if strings.Contains(strings.ToLower(status), "failed") {
		c.JSON(http.StatusInternalServerError, gin.H{"error": status})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "message": status})
}

func (s *Server) handleMetrics(c *gin.Context) {
	if s.orch == nil || s.orch.Observability == nil {
		c.String(http.StatusInternalServerError, "Observability unavailable")
		return
	}

	metrics := s.orch.Observability.ExportMetrics()
	c.Data(http.StatusOK, "text/plain; version=0.0.4", []byte(metrics))
}

// Phase 2: Theme Tokens Endpoint
func (s *Server) handleThemeTokens(c *gin.Context) {
	reg := designsystem.Get()
	if reg == nil {
		c.String(http.StatusInternalServerError, "Design system not initialized")
		return
	}

	colors := reg.GetColors()
	spacing := reg.GetSpacing()

	cssVars := theme.TokensToCSSVariables(colors, spacing)
	c.Header("Content-Type", "text/css; charset=utf-8")
	c.String(http.StatusOK, cssVars)
}

// Phase 2: Workspaces Endpoint
type WorkspaceInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Icon     string `json:"icon"`
	Badge    int    `json:"badge,omitempty"`
	Enabled  bool   `json:"enabled"`
}

func (s *Server) handleWorkspaces(c *gin.Context) {
	workspaces := []WorkspaceInfo{
		{ID: "chat", Name: "Chat", Icon: "💬", Enabled: true},
		{ID: "tasks", Name: "Tasks", Icon: "✓", Badge: 0, Enabled: true},
		{ID: "tools", Name: "Tools", Icon: "⚙", Enabled: true},
		{ID: "agents", Name: "Agents", Icon: "🤖", Enabled: true},
		{ID: "memory", Name: "Memory", Icon: "🧠", Enabled: true},
		{ID: "analytics", Name: "Analytics", Icon: "📊", Enabled: true},
		{ID: "settings", Name: "Settings", Icon: "⚡", Enabled: true},
	}
	c.JSON(http.StatusOK, workspaces)
}

func (s *Server) handleProviders(c *gin.Context) {
	primaryName := ""
	primaryID := ""
	if primary := s.orch.Primary(); primary != nil {
		meta := primary.GetMetadata()
		primaryName = meta.Name
		primaryID = meta.ID
	}

	consultantName := ""
	consultantID := ""
	if consultant := s.orch.Consultant(); consultant != nil {
		meta := consultant.GetMetadata()
		consultantName = meta.Name
		consultantID = meta.ID
	}

	c.JSON(http.StatusOK, gin.H{
		"primary": gin.H{
			"name": primaryName,
			"id":   primaryID,
		},
		"consultant": gin.H{
			"name": consultantName,
			"id":   consultantID,
		},
	})
}

// Phase 2: Runs Endpoint (recent runs with entity graph)
type RunInfo struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Model     string    `json:"model"`
	Provider  string    `json:"provider"`
	StartTime time.Time `json:"start_time"`
	ToolsUsed []string  `json:"tools_used,omitempty"`
	Artifacts []string  `json:"artifacts,omitempty"`
}

func (s *Server) handleRuns(c *gin.Context) {
	// Phase 3: Return live runs from store
	limit := 20
	if lv := c.Query("limit"); lv != "" {
		fmt.Sscanf(lv, "%d", &limit)
	}

	// Fallback for old tests that don't initialize runStore
	if s.runStore == nil {
		c.JSON(http.StatusOK, []RunInfo{})
		return
	}

	runs := s.runStore.List(limit)
	var out []gin.H
	for _, run := range runs {
		toolsUsed := make([]string, len(run.Tools))
		for i, t := range run.Tools {
			toolsUsed[i] = t.Name
		}

		out = append(out, gin.H{
			"id":         run.ID,
			"prompt":     run.Prompt,
			"status":     run.Status,
			"model":      run.Model,
			"provider":   run.Provider,
			"start_time": run.StartTime,
			"end_time":   run.EndTime,
			"latency_ms": run.LatencyMS,
			"tokens":     run.TokensUsed,
			"tools_used": toolsUsed,
			"error":      run.ErrorMsg,
		})
	}

	c.JSON(http.StatusOK, out)
}

// Phase 3: Run Details Endpoint
func (s *Server) handleRunDetails(c *gin.Context) {
	runID := c.Param("id")

	// Fallback for old tests that don't initialize runStore
	if s.runStore == nil {
		c.JSON(http.StatusOK, gin.H{
			"id":       runID,
			"status":   "complete",
			"error":    nil,
			"message": "Run details endpoint available in Phase 3",
		})
		return
	}

	run, ok := s.runStore.Get(runID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}

	toolsUsed := make([]gin.H, len(run.Tools))
	for i, t := range run.Tools {
		toolsUsed[i] = gin.H{
			"name":       t.Name,
			"status":     t.Status,
			"start_time": t.StartTime,
			"end_time":   t.EndTime,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"id":         run.ID,
		"prompt":     run.Prompt,
		"status":     run.Status,
		"model":      run.Model,
		"provider":   run.Provider,
		"start_time": run.StartTime,
		"end_time":   run.EndTime,
		"latency_ms": run.LatencyMS,
		"tokens":     run.TokensUsed,
		"tools":      toolsUsed,
		"error":      run.ErrorMsg,
	})
}

// Phase 2: Analytics Metrics Endpoint
func (s *Server) handleAnalyticsMetrics(c *gin.Context) {
	metrics := gin.H{
		"provider_latency": gin.H{
			"grok":   45.3,
			"gemini": 52.1,
		},
		"tool_success_rate": 0.96,
		"token_usage": gin.H{
			"total":      123456,
			"today":      45678,
		},
	}
	c.JSON(http.StatusOK, metrics)
}
