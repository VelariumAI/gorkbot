package webui

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/velariumai/gorkbot/internal/engine"
	"github.com/velariumai/gorkbot/pkg/config"
	"github.com/velariumai/gorkbot/pkg/registry"
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
	mu      sync.Mutex
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
	}
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
		panic(err)
	}

	s.router.StaticFS("/static", http.FS(staticFS))
	s.router.GET("/sw.js", s.handleServiceWorker)
	s.router.GET("/", s.handleIndex)

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
	tmpl, err := template.ParseFS(content, "templates/index.html")
	if err != nil {
		c.String(http.StatusInternalServerError, "Template error")
		return
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	tmpl.Execute(c.Writer, map[string]string{"Title": "Gorkbot — Adaptive AI Orchestrator"})
}

type ChatRequest struct {
	Prompt    string `json:"prompt"`
	SessionID string `json:"session_id"`
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

		ctx := context.Background()

		err := s.orch.ExecuteTaskWithStreaming(
			ctx,
			req.Prompt,
			func(token string) { ch <- StreamEvent{Type: "token", Data: token} },
			func(toolName string, result *tools.ToolResult) {
				if result != nil && result.AuthRequired {
					ch <- StreamEvent{Type: "auth_required", Data: result.Output}
				}
				ch <- StreamEvent{Type: "tool_done", Data: toolName}
			},
			func(toolName string, args map[string]interface{}) {
				ch <- StreamEvent{Type: "tool_start", Data: toolName}
			},
			nil,
			nil,
		)

		if err != nil {
			ch <- StreamEvent{Type: "error", Data: err.Error()}
		} else {
			ch <- StreamEvent{Type: "done", Data: ""}
		}
	}()

	c.JSON(http.StatusOK, gin.H{"session_id": req.SessionID})
}

func (s *Server) handleStream(c *gin.Context) {
	sessionID := c.Query("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing session_id"})
		return
	}

	s.mu.Lock()
	ch, ok := s.streams[sessionID]
	s.mu.Unlock()

	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session_id"})
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
	if s.orch.Primary != nil {
		state["provider"] = s.orch.Primary.GetMetadata().Name
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
	if s.orch.Primary != nil {
		currentPrimary = s.orch.Primary.GetMetadata().ID
	}

	currentSecondary := "auto"
	if s.orch.Consultant != nil {
		currentSecondary = s.orch.Consultant.GetMetadata().ID
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
			s.orch.Consultant = nil
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
	out := []gin.H{
		{"id": "agent-core", "status": "active", "type": "Orchestrator"},
	}
	c.JSON(http.StatusOK, gin.H{"agents": out})
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
