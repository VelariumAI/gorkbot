// Package api provides REST and WebSocket interfaces to Gorkbot for headless operation.
// This enables multi-user access, third-party integrations, and automation workflows.
package api

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/velariumai/gorkbot/internal/engine"
	"github.com/velariumai/gorkbot/internal/integration"
	"github.com/velariumai/gorkbot/pkg/skills"
	"github.com/velariumai/gorkbot/pkg/tools"
)

// Server represents the headless API server (REST + WebSocket).
type Server struct {
	router      *gin.Engine
	orch        *engine.Orchestrator
	jwtSecret   string
	config      *ServerConfig
	logger      *slog.Logger
	sessions    *SessionManager
	eventBus    chan *ServerEvent
	shutdown    chan struct{}
	wg          sync.WaitGroup
	connectors  *integration.ConnectorRegistry // Integration connectors (Telegram, Discord, Email)
	metrics     *Metrics                       // API performance metrics
	audit       *AuditLogger                   // Audit trail logging
	rateLimiter *rateLimiter
}

// ServerConfig holds configuration for the API server.
type ServerConfig struct {
	Port               int
	Host               string
	TLSCert            string
	TLSKey             string
	RateLimitPerMin    int
	SessionTimeoutMin  int
	MaxConcurrentWS    int
	AllowedOrigins     []string
	TokenTTL           time.Duration
	AllowInsecureLogin bool
	AuthUsername       string
	AuthPassword       string
}

// NewServer creates a new headless API server.
func NewServer(port int, orch *engine.Orchestrator, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	config := &ServerConfig{
		Port:              port,
		Host:              "localhost",
		RateLimitPerMin:   60,
		SessionTimeoutMin: 30,
		MaxConcurrentWS:   100,
		AllowedOrigins:    []string{"http://localhost:*", "http://127.0.0.1:*"},
		TokenTTL:          24 * time.Hour,
	}

	authUser := os.Getenv("GORKBOT_API_USER")
	authPass := os.Getenv("GORKBOT_API_PASSWORD")
	allowInsecure := strings.EqualFold(os.Getenv("GORKBOT_API_ALLOW_INSECURE_LOGIN"), "true")
	config.AuthUsername = authUser
	config.AuthPassword = authPass
	config.AllowInsecureLogin = allowInsecure

	jwtSecret := os.Getenv("GORKBOT_JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = os.Getenv("GORKBOT_API_JWT_SECRET")
	}
	if jwtSecret == "" {
		jwtSecret = randomSecret(32)
		logger.Warn("JWT secret not set; using ephemeral secret (set GORKBOT_JWT_SECRET to persist tokens)")
	}

	s := &Server{
		orch:        orch,
		jwtSecret:   jwtSecret,
		config:      config,
		logger:      logger,
		sessions:    NewSessionManager(),
		eventBus:    make(chan *ServerEvent, 1000),
		shutdown:    make(chan struct{}),
		connectors:  integration.NewConnectorRegistry(),
		metrics:     NewMetrics(),
		audit:       nil, // Initialized after Start() if enabled
		rateLimiter: newRateLimiter(config.RateLimitPerMin, 10*time.Minute),
	}

	s.setupRouter()
	return s
}

// setupRouter configures all HTTP routes and middleware.
func (s *Server) setupRouter() {
	gin.SetMode(gin.ReleaseMode)
	s.router = gin.New()

	// Middleware
	s.router.Use(gin.Recovery())
	s.router.Use(s.loggingMiddleware())
	s.router.Use(s.corsMiddleware())
	s.router.Use(s.rateLimitMiddleware())

	// Health check and metrics (no auth)
	s.router.GET("/health", s.handleHealth)
	s.router.GET("/version", s.handleVersion)
	s.router.GET("/metrics", s.handleMetrics)

	// Authentication
	s.router.POST("/auth/login", s.handleLogin)
	s.router.POST("/auth/logout", s.handleLogout)
	s.router.POST("/auth/refresh", s.handleRefresh)

	// API v1 routes (require authentication)
	api := s.router.Group("/api/v1")
	api.Use(s.authMiddleware())
	{
		// Status and info
		api.GET("/status", s.handleStatus)
		api.GET("/info", s.handleInfo)

		// Conversation
		api.POST("/message", s.handleMessage)
		api.GET("/messages", s.handleMessages)
		api.DELETE("/messages", s.handleClearMessages)

		// Tools
		api.GET("/tools", s.handleToolList)
		api.GET("/tools/:name", s.handleToolInfo)
		api.POST("/tools/:name/execute", s.handleExecuteTool)
		api.GET("/tools/search", s.handleToolSearch)

		// Skills
		api.GET("/skills", s.handleSkillList)
		api.GET("/skills/lint", s.handleSkillLint)
		api.GET("/skills/:name", s.handleSkillInfo)

		// Self-Improvement
		api.GET("/si/status", s.handleSIStatus)
		api.GET("/si/metrics", s.handleSIMetrics)
		api.POST("/si/propose", s.handleSIPropose)

		// Session management
		api.GET("/session", s.handleSessionInfo)
		api.POST("/session/reset", s.handleSessionReset)

		// Memory
		api.GET("/memory/status", s.handleMemoryStatus)
		api.GET("/memory/facts", s.handleMemoryFacts)

		// Integration connectors (Telegram, Discord, Email)
		api.GET("/integrations", s.handleIntegrationList)
		api.POST("/integrations/:name/message", s.handleConnectorMessage)
		api.GET("/integrations/:name/health", s.handleConnectorHealth)

		// WebSocket
		api.GET("/ws", s.handleWebSocket)
	}

	s.logger.Info("API routes configured")
}

// loggingMiddleware logs all API requests and records metrics.
func (s *Server) loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		duration := time.Since(start)
		durationMs := duration.Milliseconds()
		status := c.Writer.Status()
		success := status >= 200 && status < 300

		// Record metrics
		s.metrics.RecordRequest(c.Request.URL.Path, c.Request.Method, durationMs, success)

		s.logger.Info("api request",
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", status),
			slog.Duration("duration", duration),
			slog.String("client", c.ClientIP()),
		)
	}
}

// corsMiddleware enables CORS for configured origins.
func (s *Server) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		allowedOrigin := false
		for _, allowed := range s.config.AllowedOrigins {
			if matchOrigin(allowed, origin) {
				allowedOrigin = true
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
				c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				c.Writer.Header().Set("Access-Control-Max-Age", "3600")
				break
			}
		}
		if origin != "" && !allowedOrigin {
			s.writeAPIError(c, http.StatusForbidden, "origin_not_allowed", "origin not allowed")
			c.Abort()
			return
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// rateLimitMiddleware enforces per-user rate limiting.
func (s *Server) rateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.rateLimiter != nil {
			key := c.ClientIP()
			if key == "" {
				key = "unknown"
			}
			if !s.rateLimiter.Allow(key) {
				c.Header("Retry-After", "60")
				s.writeAPIError(c, http.StatusTooManyRequests, "rate_limited", "rate limit exceeded")
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

// authMiddleware validates JWT tokens.
func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("Authorization")
		if token == "" {
			s.writeAPIError(c, http.StatusUnauthorized, "missing_authorization", "missing authorization")
			c.Abort()
			return
		}

		userID, valid := s.validateToken(token)
		if !valid {
			s.writeAPIError(c, http.StatusUnauthorized, "invalid_token", "invalid token")
			c.Abort()
			return
		}

		c.Set("user_id", userID)
		c.Set("correlation_id", uuid.New().String())
		c.Next()
	}
}

// Start starts the API server (blocking).
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	s.logger.Info("starting API server", "addr", addr)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	defer listener.Close()

	// Wrap listener with TLS if certificates are provided
	if s.config.TLSCert != "" && s.config.TLSKey != "" {
		tlsListener, err := setupTLSListener(listener, s.config.TLSCert, s.config.TLSKey, s.logger)
		if err != nil {
			return fmt.Errorf("failed to setup TLS: %w", err)
		}
		listener = tlsListener
		s.logger.Info("TLS enabled for API server")
	}

	// Start event bus processor
	s.wg.Add(1)
	go s.processEventBus()

	// Start connectors
	ctx := context.Background()
	if err := s.connectors.StartAll(ctx); err != nil {
		s.logger.Warn("failed to start all connectors", "error", err)
		// Continue even if some connectors fail to start
	}

	// Serve HTTP
	srv := &http.Server{
		Handler:      s.router,
		Addr:         addr,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Handle graceful shutdown
	go func() {
		<-s.shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	s.logger.Info("API server listening", "addr", addr)
	return srv.Serve(listener)
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Stop all connectors
	if err := s.connectors.StopAll(ctx); err != nil {
		s.logger.Warn("error stopping connectors", "error", err)
	}

	close(s.shutdown)
	s.wg.Wait()
	s.logger.Info("API server shutdown complete")
}

// RegisterConnector registers a new integration connector.
func (s *Server) RegisterConnector(connector integration.Connector) {
	s.connectors.Register(connector)
	s.logger.Info("registered connector", "name", connector.Name())
}

// processEventBus processes server events asynchronously.
func (s *Server) processEventBus() {
	defer s.wg.Done()

	for {
		select {
		case <-s.shutdown:
			return
		case evt := <-s.eventBus:
			if evt == nil {
				return
			}
			// Broadcast to connected WebSocket clients
			s.broadcastEvent(evt)
		}
	}
}

// broadcastEvent sends an event to all connected WebSocket clients.
func (s *Server) broadcastEvent(evt *ServerEvent) {
	if evt == nil || s.sessions == nil {
		return
	}
	s.sessions.mu.RLock()
	sessions := make([]*WSSession, 0, len(s.sessions.wsSessions))
	for _, ws := range s.sessions.wsSessions {
		sessions = append(sessions, ws)
	}
	s.sessions.mu.RUnlock()

	for _, ws := range sessions {
		if ws == nil || ws.Conn == nil {
			continue
		}
		if evt.UserID != "" && ws.UserID != evt.UserID {
			continue
		}
		if err := ws.Conn.WriteJSON(evt); err != nil {
			s.logger.Debug("broadcast event failed", "user_id", ws.UserID, "error", err)
		}
	}
}

// Handler functions

func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"uptime": time.Now().Unix(),
	})
}

func (s *Server) handleVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version": "6.1",
		"build":   "phase5",
	})
}

func (s *Server) handleLogin(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.BindJSON(&req); err != nil {
		s.writeAPIError(c, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}

	if s.config.AuthUsername != "" || s.config.AuthPassword != "" {
		if req.Username != s.config.AuthUsername || req.Password != s.config.AuthPassword {
			s.writeAPIError(c, http.StatusUnauthorized, "invalid_credentials", "invalid credentials")
			return
		}
	} else if !s.config.AllowInsecureLogin {
		s.writeAPIError(c, http.StatusUnauthorized, "login_disabled", "login disabled: configure GORKBOT_API_USER/PASSWORD or set GORKBOT_API_ALLOW_INSECURE_LOGIN=true")
		return
	}

	token, err := s.generateToken(req.Username)
	if err != nil {
		s.logger.Error("failed to generate token", "error", err)
		s.writeAPIError(c, http.StatusInternalServerError, "token_generation_failed", "token generation failed")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":   token,
		"expires": time.Now().Add(s.config.TokenTTL).Unix(),
		"user_id": req.Username,
	})
}

func (s *Server) handleLogout(c *gin.Context) {
	userID := c.GetString("user_id")
	s.sessions.RemoveSession(userID)
	c.JSON(http.StatusOK, gin.H{"status": "logged out"})
}

func (s *Server) handleRefresh(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			s.writeAPIError(c, http.StatusUnauthorized, "missing_authorization", "missing authorization")
			return
		}
		var ok bool
		userID, ok = s.validateToken(authHeader)
		if !ok {
			s.writeAPIError(c, http.StatusUnauthorized, "invalid_token", "invalid token")
			return
		}
	}
	token, err := s.generateToken(userID)
	if err != nil {
		s.logger.Error("failed to generate token", "error", err)
		s.writeAPIError(c, http.StatusInternalServerError, "token_generation_failed", "token generation failed")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"token":   token,
		"expires": time.Now().Add(s.config.TokenTTL).Unix(),
	})
}

func (s *Server) handleStatus(c *gin.Context) {
	if s.orch == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "orchestrator not initialized",
		})
		return
	}
	report := s.orch.GetContextReport()
	c.JSON(http.StatusOK, gin.H{
		"status": "active",
		"report": report,
	})
}

func (s *Server) handleInfo(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version": "6.1",
		"features": []string{
			"headless_api",
			"mcp_server",
			"plugin_sdk",
			"skill_registry",
		},
	})
}

func (s *Server) handleMessage(c *gin.Context) {
	var req struct {
		Prompt string `json:"prompt" binding:"required"`
		Model  string `json:"model"`
	}

	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	correlationID := c.GetString("correlation_id")
	if s.orch == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "orchestrator not initialized"})
		return
	}

	response, err := s.orch.ExecuteTask(c.Request.Context(), req.Prompt)
	if err != nil {
		s.logger.Error("orchestrator execute failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":         "ok",
		"response":       response,
		"model":          req.Model,
		"correlation_id": correlationID,
	})
}

func (s *Server) handleMessages(c *gin.Context) {
	if s.orch == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "orchestrator not initialized"})
		return
	}
	history := s.orch.GetHistory()
	if history == nil {
		c.JSON(http.StatusOK, gin.H{"messages": []interface{}{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"messages": history.GetMessages()})
}

func (s *Server) handleClearMessages(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "cleared"})
}

func (s *Server) handleToolList(c *gin.Context) {
	if s.orch == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "orchestrator not initialized",
		})
		return
	}
	tools := s.orch.Registry.List()
	c.JSON(http.StatusOK, gin.H{"tools": tools})
}

func (s *Server) handleToolInfo(c *gin.Context) {
	name := c.Param("name")
	if s.orch == nil || s.orch.Registry == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "orchestrator not initialized"})
		return
	}
	tool, ok := s.orch.Registry.Get(name)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "tool not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"name":        name,
		"description": tool.Description(),
		"category":    tool.Category(),
		"parameters":  json.RawMessage(tool.Parameters()),
	})
}

func (s *Server) handleExecuteTool(c *gin.Context) {
	tool := c.Param("name")
	if s.orch == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "orchestrator not initialized"})
		return
	}

	var payload map[string]interface{}
	if err := c.BindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	params := payload
	if rawParams, ok := payload["parameters"]; ok {
		if parsed, ok := rawParams.(map[string]interface{}); ok {
			params = parsed
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "parameters must be an object"})
			return
		}
	}

	res, err := s.orch.ExecuteTool(c.Request.Context(), tools.ToolRequest{
		ToolName:   tool,
		Parameters: params,
		RequestID:  uuid.New().String(),
		AgentID:    c.GetString("user_id"),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if res == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tool returned nil result"})
		return
	}

	status := http.StatusOK
	if !res.Success {
		status = http.StatusBadRequest
	}
	c.JSON(status, gin.H{
		"tool":           tool,
		"result":         res,
		"correlation_id": c.GetString("correlation_id"),
	})
}

func (s *Server) handleToolSearch(c *gin.Context) {
	query := c.Query("q")
	if s.orch == nil || s.orch.Registry == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "orchestrator not initialized"})
		return
	}
	toolsFound := s.orch.Registry.Search(query)
	results := make([]gin.H, 0, len(toolsFound))
	for _, t := range toolsFound {
		results = append(results, gin.H{
			"name":        t.Name(),
			"description": t.Description(),
			"category":    t.Category(),
		})
	}
	c.JSON(http.StatusOK, gin.H{"query": query, "results": results})
}

func (s *Server) handleSkillList(c *gin.Context) {
	if s.orch == nil || s.orch.SkillLoader == nil {
		c.JSON(http.StatusOK, gin.H{"skills": []interface{}{}})
		return
	}
	skills := s.orch.SkillLoader.List()
	resp := make([]gin.H, 0, len(skills))
	for _, sm := range skills {
		resp = append(resp, gin.H{
			"name":        sm.Manifest.Name,
			"version":     sm.Manifest.Version,
			"description": sm.Manifest.Description,
			"category":    sm.Manifest.Category,
			"enabled":     sm.Manifest.Enabled,
		})
	}
	c.JSON(http.StatusOK, gin.H{"skills": resp})
}

func (s *Server) handleSkillInfo(c *gin.Context) {
	name := c.Param("name")
	if s.orch == nil || s.orch.SkillLoader == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill loader not configured"})
		return
	}
	skill := s.orch.SkillLoader.Get(name)
	if skill == nil || skill.Manifest == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"name":      skill.Manifest.Name,
		"manifest":  skill.Manifest,
		"file_path": skill.FilePath,
	})
}

func (s *Server) handleSkillLint(c *gin.Context) {
	if s.orch == nil || s.orch.SkillLoader == nil {
		c.JSON(http.StatusOK, gin.H{"issues": []interface{}{}})
		return
	}
	issues := make([]skills.LintIssue, 0)
	for _, sm := range s.orch.SkillLoader.List() {
		if sm == nil || sm.FilePath == "" {
			continue
		}
		issues = append(issues, s.orch.SkillLoader.LintFile(sm.FilePath)...)
	}
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].File != issues[j].File {
			return issues[i].File < issues[j].File
		}
		if issues[i].Skill != issues[j].Skill {
			return issues[i].Skill < issues[j].Skill
		}
		return issues[i].Message < issues[j].Message
	})
	c.JSON(http.StatusOK, gin.H{"issues": issues, "issue_count": len(issues)})
}

func (s *Server) handleSIStatus(c *gin.Context) {
	if s.orch == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "orchestrator not initialized"})
		return
	}
	snapshot := s.orch.SISnapshot()
	c.JSON(http.StatusOK, gin.H{"snapshot": snapshot, "si_enabled": snapshot.Enabled})
}

func (s *Server) handleSIMetrics(c *gin.Context) {
	if s.orch == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "orchestrator not initialized"})
		return
	}
	snapshot := s.orch.SISnapshot()
	c.JSON(http.StatusOK, gin.H{
		"metrics": gin.H{
			"cycle_count":     snapshot.CycleCount,
			"drive_score":     snapshot.DriveScore,
			"raw_score":       snapshot.RawScore,
			"pending_signals": snapshot.PendingSignals,
			"active_phase":    snapshot.ActivePhase,
		},
	})
}

func (s *Server) handleSIPropose(c *gin.Context) {
	if s.orch == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "orchestrator not initialized"})
		return
	}
	before := s.orch.SISnapshot()
	after := s.orch.TriggerSICycle(c.Request.Context())
	c.JSON(http.StatusAccepted, gin.H{
		"status":        "executed",
		"si_enabled":    after.Enabled,
		"before":        before,
		"after":         after,
		"active_phase":  after.ActivePhase,
		"next_tick":     after.NextHeartbeat.Unix(),
		"pending_count": after.PendingSignals,
	})
}

func (s *Server) handleSessionInfo(c *gin.Context) {
	userID := c.GetString("user_id")
	c.JSON(http.StatusOK, gin.H{"user_id": userID})
}

func (s *Server) handleSessionReset(c *gin.Context) {
	userID := c.GetString("user_id")
	s.sessions.RemoveSession(userID)
	c.JSON(http.StatusOK, gin.H{"status": "reset"})
}

func (s *Server) handleMemoryStatus(c *gin.Context) {
	if s.orch == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "orchestrator not initialized"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"memory_enabled":    s.orch.AgeMem != nil || s.orch.UnifiedMem != nil,
		"agemem_enabled":    s.orch.AgeMem != nil,
		"unified_enabled":   s.orch.UnifiedMem != nil,
		"goal_ledger_ready": s.orch.GoalLedger != nil,
	})
}

func (s *Server) handleMemoryFacts(c *gin.Context) {
	// Fact export endpoint intentionally returns a stable empty list when no
	// fact backend query adapter is wired.
	c.JSON(http.StatusOK, gin.H{"facts": []interface{}{}})
}

func (s *Server) handleWebSocket(c *gin.Context) {
	userID := c.GetString("user_id")

	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				origin = "http://" + r.Host
			}
			for _, allowed := range s.config.AllowedOrigins {
				if matchOrigin(allowed, origin) {
					return true
				}
			}
			return false
		},
	}

	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		s.logger.Error("websocket upgrade failed", "error", err)
		return
	}
	defer ws.Close()

	// Handle WebSocket connection
	session := NewWSSession(userID, ws, s.logger)
	s.sessions.AddWSSession(userID, session)
	defer s.sessions.RemoveWSSession(userID)

	for {
		var msg json.RawMessage
		if err := ws.ReadJSON(&msg); err != nil {
			s.logger.Debug("websocket read error", "error", err)
			break
		}

		response := gin.H{"status": "received"}
		if s.orch != nil {
			var req struct {
				Prompt string `json:"prompt"`
			}
			if err := json.Unmarshal(msg, &req); err == nil && strings.TrimSpace(req.Prompt) != "" {
				if out, execErr := s.orch.ExecuteTask(c.Request.Context(), req.Prompt); execErr == nil {
					response["status"] = "ok"
					response["response"] = out
				} else {
					response["status"] = "error"
					response["error"] = execErr.Error()
				}
			}
		}
		if err := ws.WriteJSON(response); err != nil {
			s.logger.Error("websocket write error", "error", err)
			break
		}
	}
}

// Integration connector handlers

func (s *Server) handleIntegrationList(c *gin.Context) {
	connectors := s.connectors.List()
	names := make([]string, len(connectors))
	for i, conn := range connectors {
		names[i] = conn.Name()
	}
	c.JSON(http.StatusOK, gin.H{
		"integrations": names,
	})
}

func (s *Server) handleConnectorMessage(c *gin.Context) {
	name := c.Param("name")
	connector := s.connectors.Get(name)
	if connector == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("connector not found: %s", name),
		})
		return
	}

	var req struct {
		TargetID string `json:"target_id" binding:"required"`
		Message  string `json:"message" binding:"required"`
	}

	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	// Send response through connector
	resp := &integration.Response{
		SourceID: req.TargetID,
		Text:     req.Message,
		Metadata: make(map[string]string),
	}

	if err := connector.Send(c.Request.Context(), resp); err != nil {
		s.logger.Error("failed to send via connector", "connector", name, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    "sent",
		"connector": name,
		"target_id": req.TargetID,
	})
}

func (s *Server) handleConnectorHealth(c *gin.Context) {
	name := c.Param("name")
	connector := s.connectors.Get(name)
	if connector == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("connector not found: %s", name),
		})
		return
	}

	healthy := connector.IsHealthy(c.Request.Context())
	status := "healthy"
	code := http.StatusOK
	if !healthy {
		status = "unhealthy"
		code = http.StatusServiceUnavailable
	}

	c.JSON(code, gin.H{
		"connector": name,
		"status":    status,
		"healthy":   healthy,
	})
}

// Metrics endpoint handler

func (s *Server) handleMetrics(c *gin.Context) {
	stats := s.metrics.GetStats()
	endpoints := s.metrics.GetEndpointStats()

	c.JSON(http.StatusOK, gin.H{
		"summary":   stats,
		"endpoints": endpoints,
		"timestamp": time.Now().Unix(),
	})
}

// Helper functions

func (s *Server) writeAPIError(c *gin.Context, status int, code, message string) {
	if code == "" {
		code = "error"
	}
	if message == "" {
		message = http.StatusText(status)
	}
	c.JSON(status, gin.H{
		"error": message,
		"code":  code,
	})
}

// setupTLSListener wraps a TCP listener with TLS.
func setupTLSListener(listener net.Listener, certFile, keyFile string, logger *slog.Logger) (net.Listener, error) {
	// Load certificate and private key
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load certificates: %w", err)
	}

	// Configure TLS 1.3
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
		MaxVersion:   tls.VersionTLS13,
		// Recommended cipher suites for TLS 1.3
		CipherSuites: []uint16{
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
			tls.TLS_AES_128_GCM_SHA256,
		},
	}

	return tls.NewListener(listener, tlsConfig), nil
}

func matchOrigin(pattern, origin string) bool {
	// Wildcard pattern matching
	if pattern == "*" || pattern == "" {
		return true
	}
	if pattern == origin {
		return true
	}
	// Handle wildcard patterns like "http://localhost:*"
	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return strings.HasPrefix(origin, prefix)
	}
	return false
}

func (s *Server) generateToken(userID string) (string, error) {
	if userID == "" {
		return "", fmt.Errorf("userID required")
	}
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Subject:   userID,
		ID:        uuid.NewString(),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(s.config.TokenTTL)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.jwtSecret))
}

func (s *Server) validateToken(tokenHeader string) (string, bool) {
	if tokenHeader == "" {
		return "", false
	}
	tokenString := strings.TrimSpace(tokenHeader)
	if strings.HasPrefix(strings.ToLower(tokenString), "bearer ") {
		tokenString = strings.TrimSpace(tokenString[7:])
	}
	if tokenString == "" {
		return "", false
	}
	parsed, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(s.jwtSecret), nil
	})
	if err != nil || !parsed.Valid {
		return "", false
	}
	claims, ok := parsed.Claims.(*jwt.RegisteredClaims)
	if !ok {
		return "", false
	}
	if claims.Subject == "" {
		return "", false
	}
	if claims.ExpiresAt != nil && time.Now().After(claims.ExpiresAt.Time) {
		return "", false
	}
	if claims.NotBefore != nil && time.Now().Before(claims.NotBefore.Time) {
		return "", false
	}
	return claims.Subject, true
}

func randomSecret(length int) string {
	if length <= 0 {
		length = 32
	}
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("gorkbot-%d", time.Now().UnixNano())
	}
	return base64.RawStdEncoding.EncodeToString(buf)
}

type tokenBucket struct {
	capacity float64
	tokens   float64
	rate     float64
	lastFill time.Time
	lastSeen time.Time
}

type rateLimiter struct {
	mu          sync.Mutex
	buckets     map[string]*tokenBucket
	ratePerSec  float64
	capacity    float64
	maxIdle     time.Duration
	lastCleanup time.Time
}

func newRateLimiter(rpm int, maxIdle time.Duration) *rateLimiter {
	if rpm <= 0 {
		rpm = 60
	}
	if maxIdle <= 0 {
		maxIdle = 10 * time.Minute
	}
	return &rateLimiter{
		buckets:    make(map[string]*tokenBucket),
		ratePerSec: float64(rpm) / 60.0,
		capacity:   float64(rpm),
		maxIdle:    maxIdle,
	}
}

func (rl *rateLimiter) Allow(key string) bool {
	now := time.Now()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	if rl.lastCleanup.IsZero() || now.Sub(rl.lastCleanup) > rl.maxIdle {
		for k, b := range rl.buckets {
			if now.Sub(b.lastSeen) > rl.maxIdle {
				delete(rl.buckets, k)
			}
		}
		rl.lastCleanup = now
	}

	bucket, ok := rl.buckets[key]
	if !ok {
		bucket = &tokenBucket{
			capacity: rl.capacity,
			tokens:   rl.capacity,
			rate:     rl.ratePerSec,
			lastFill: now,
			lastSeen: now,
		}
		rl.buckets[key] = bucket
	}

	elapsed := now.Sub(bucket.lastFill).Seconds()
	bucket.tokens += elapsed * bucket.rate
	if bucket.tokens > bucket.capacity {
		bucket.tokens = bucket.capacity
	}
	bucket.lastFill = now
	bucket.lastSeen = now

	if bucket.tokens >= 1.0 {
		bucket.tokens -= 1.0
		return true
	}
	return false
}

// ServerEvent represents an event emitted by the server.
type ServerEvent struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
	UserID  string      `json:"user_id"`
	Time    time.Time   `json:"time"`
}

// WSSession represents a WebSocket session.
type WSSession struct {
	UserID string
	Conn   *websocket.Conn
	Logger *slog.Logger
}

// NewWSSession creates a new WebSocket session.
func NewWSSession(userID string, conn *websocket.Conn, logger *slog.Logger) *WSSession {
	return &WSSession{
		UserID: userID,
		Conn:   conn,
		Logger: logger,
	}
}

// SessionManager manages headless API sessions.
type SessionManager struct {
	mu         sync.RWMutex
	sessions   map[string]*Session
	wsSessions map[string]*WSSession
}

// Session represents an API session.
type Session struct {
	UserID    string
	Token     string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// NewSessionManager creates a new session manager.
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions:   make(map[string]*Session),
		wsSessions: make(map[string]*WSSession),
	}
}

// AddSession adds a new session.
func (sm *SessionManager) AddSession(userID string, token string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.sessions[userID] = &Session{
		UserID:    userID,
		Token:     token,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
}

// RemoveSession removes a session.
func (sm *SessionManager) RemoveSession(userID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, userID)
}

// GetSession retrieves a session.
func (sm *SessionManager) GetSession(userID string) (*Session, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	session, ok := sm.sessions[userID]
	return session, ok
}

// AddWSSession adds a WebSocket session.
func (sm *SessionManager) AddWSSession(userID string, session *WSSession) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.wsSessions[userID] = session
}

// RemoveWSSession removes a WebSocket session.
func (sm *SessionManager) RemoveWSSession(userID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.wsSessions, userID)
}
