package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/velariumai/gorkbot/pkg/tools"
)

// ManagedMCPServer wraps the mark3labs MCP client with resilient lifecycle
// management: automatic restart on crash, shaper pre/post-processing, and
// structured auth-sentinel routing.
//
// Design invariants:
//   - config + envVars are immutable after construction; stored so that each
//     restart can create a FRESH ProcessManager (with a fresh dead channel).
//   - Only one recovery can run at a time (atomic.Bool recovering guard).
//   - stopCh + sync.Once prevent spurious recovery after an intentional Stop().
//   - watchdog takes pm as a parameter — never reads s.pm — so it always
//     monitors the exact process it was launched for (no stale-reference race).
type ManagedMCPServer struct {
	name       string
	config     ServerConfig // immutable; used by bootstrap to recreate ProcessManager
	envVars    []string     // immutable; injected into subprocess env on each start
	shaperPath string

	mu        sync.Mutex
	pm        *ProcessManager
	mcpClient *client.Client
	toolDefs  []mcp.Tool
	registry  *tools.Registry
	running   bool

	stopCh   chan struct{}
	stopOnce sync.Once

	recovering atomic.Bool // guards against double-recovery

	logger *slog.Logger
}

// NewManagedMCPServer constructs a server that is ready to Start().
// cfg and envVars are stored for use on every (re)bootstrap.
func NewManagedMCPServer(cfg ServerConfig, envVars []string, logger *slog.Logger) *ManagedMCPServer {
	return &ManagedMCPServer{
		name:       cfg.Name,
		config:     cfg,
		envVars:    envVars,
		shaperPath: cfg.ShaperPath,
		stopCh:     make(chan struct{}),
		logger:     logger,
	}
}

// ── Lifecycle ──────────────────────────────────────────────────────────────────

// Start bootstraps the subprocess and launches the watchdog goroutine.
func (s *ManagedMCPServer) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.bootstrap(ctx); err != nil {
		return err
	}
	go s.watchdog(ctx, s.pm)
	return nil
}

// bootstrap creates a FRESH ProcessManager, starts it, initialises the MCP
// client, and discovers tools. Must be called with s.mu held.
//
// A new ProcessManager is required on every call because the dead channel in
// the previous ProcessManager is already closed; reusing it would cause the
// watchdog to fire immediately on the new process.
func (s *ManagedMCPServer) bootstrap(ctx context.Context) error {
	pm := NewProcessManager(s.config.Command, s.config.Args, s.envVars, s.logger)

	stdin, stdout, err := pm.Start(ctx)
	if err != nil {
		return fmt.Errorf("process start: %w", err)
	}

	t := transport.NewIO(stdout, stdin, nil)
	mcpClient := client.NewClient(t)

	if err := mcpClient.Start(ctx); err != nil {
		_ = pm.Stop()
		return fmt.Errorf("client transport start: %w", err)
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "GorkbotController",
		Version: "4.6.0",
	}
	if _, err := mcpClient.Initialize(ctx, initReq); err != nil {
		_ = pm.Stop()
		return fmt.Errorf("mcp initialize: %w", err)
	}

	toolsRes, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		_ = pm.Stop()
		return fmt.Errorf("tool discovery: %w", err)
	}

	s.pm = pm
	s.mcpClient = mcpClient
	s.toolDefs = toolsRes.Tools
	s.running = true

	for _, td := range s.toolDefs {
		s.logger.Info("MCP tool discovered", "server", s.name, "tool", td.Name)
	}
	if s.registry != nil {
		s.syncWithRegistry()
	}

	s.logger.Info("MCP server ready", "name", s.name, "tools", len(s.toolDefs))
	return nil
}

// watchdog monitors the specific ProcessManager pm for unexpected exit.
// It deliberately takes pm as a parameter — never reads s.pm — to avoid a
// stale-reference race where recoverLocked has already swapped in a new pm.
func (s *ManagedMCPServer) watchdog(ctx context.Context, pm *ProcessManager) {
	select {
	case <-pm.Dead():
		// Check for intentional stop before triggering recovery.
		select {
		case <-s.stopCh:
			s.logger.Info("MCP watchdog: intentional stop, no recovery", "name", s.name)
			return
		default:
		}
		s.logger.Warn("MCP subprocess died unexpectedly, recovering", "name", s.name)
		s.recoverLocked(ctx)

	case <-s.stopCh:
		s.logger.Debug("MCP watchdog: stop signalled", "name", s.name)

	case <-ctx.Done():
		s.logger.Debug("MCP watchdog: context done", "name", s.name)
	}
}

// recoverLocked performs a full restart: stop → sleep → bootstrap → new watchdog.
// The atomic recovering guard ensures only one recovery runs at a time.
func (s *ManagedMCPServer) recoverLocked(ctx context.Context) {
	if !s.recovering.CompareAndSwap(false, true) {
		s.logger.Debug("MCP recovery already in progress, skipping", "name", s.name)
		return
	}
	defer s.recovering.Store(false)

	s.logger.Info("MCP recovery: starting", "name", s.name)

	// Grab the old ProcessManager and mark server as not running.
	s.mu.Lock()
	oldPm := s.pm
	s.running = false
	s.mu.Unlock()

	// Stop the old process outside the lock to avoid holding it during I/O.
	if oldPm != nil {
		_ = oldPm.Stop()
	}
	time.Sleep(500 * time.Millisecond)

	// Bootstrap a fresh process while holding the lock.
	s.mu.Lock()
	err := s.bootstrap(ctx)
	newPm := s.pm
	s.mu.Unlock()

	if err != nil {
		s.logger.Error("MCP recovery failed", "name", s.name, "err", err)
		return
	}

	go s.watchdog(ctx, newPm)
	s.logger.Info("MCP recovery succeeded", "name", s.name)
}

// Stop signals the watchdog and shuts down the subprocess.
func (s *ManagedMCPServer) Stop() error {
	s.stopOnce.Do(func() { close(s.stopCh) })
	s.mu.Lock()
	pm := s.pm
	s.running = false
	s.mu.Unlock()
	if pm != nil {
		return pm.Stop()
	}
	return nil
}

// ── Registry integration ───────────────────────────────────────────────────────

// SyncWith registers (or re-registers after recovery) all tools into reg.
func (s *ManagedMCPServer) SyncWith(reg *tools.Registry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.registry = reg
	if s.running {
		s.syncWithRegistry()
	}
}

func (s *ManagedMCPServer) syncWithRegistry() {
	for _, td := range s.toolDefs {
		wrapper := &mcpToolWrapper{
			server: s,
			def:    td,
			name:   fmt.Sprintf("mcp_%s_%s", s.name, td.Name),
		}
		s.registry.RegisterOrReplace(wrapper)
	}
}

// ── Tool execution ─────────────────────────────────────────────────────────────

// CallTool executes a named MCP tool with shaper pre/post-processing.
func (s *ManagedMCPServer) CallTool(ctx context.Context, toolName string, args map[string]interface{}) (*mcp.CallToolResult, error) {
	if s.recovering.Load() {
		return nil, fmt.Errorf("mcp server %q is recovering, retry shortly", s.name)
	}

	// Pre-process: run shaper on the "query" argument if present.
	shapedArgs := make(map[string]interface{}, len(args))
	for k, v := range args {
		shapedArgs[k] = v
	}
	if s.shaperPath != "" {
		if _, hasQuery := shapedArgs["query"]; hasQuery {
			rawQuery := fmt.Sprintf("%v", shapedArgs["query"])
			if processed, err := s.runShaper("shape", map[string]interface{}{
				"query":  rawQuery,
				"result": "",
			}); err == nil {
				shapedArgs["query"] = processed // FIX: was logged but never applied
			} else {
				s.logger.Warn("Shaper pre-process failed", "server", s.name, "err", err)
			}
		}
	}

	// Snapshot the client reference without holding the lock during the call.
	s.mu.Lock()
	mcpClient := s.mcpClient
	s.mu.Unlock()

	if mcpClient == nil {
		return nil, fmt.Errorf("mcp client not initialized for %q", s.name)
	}

	res, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      toolName,
			Arguments: shapedArgs,
		},
	})
	if err != nil {
		s.logger.Warn("MCP tool call failed, scheduling recovery",
			"server", s.name, "tool", toolName, "err", err)
		go s.recoverLocked(context.Background())
		return nil, fmt.Errorf("mcp call failed: %w", err)
	}

	// Post-process: run shaper on the first text content block.
	if s.shaperPath != "" && len(res.Content) > 0 {
		if tc, ok := res.Content[0].(mcp.TextContent); ok {
			if parsed, err := s.runShaper("parse", map[string]interface{}{
				"result": tc.Text,
				"query":  "",
			}); err == nil {
				res.Content[0] = mcp.TextContent{Type: "text", Text: parsed}
			}
		}
	}

	return res, nil
}

// runShaper invokes the Python shaper script as a short-lived subprocess.
// It uses the package-level resolveBinary so the shaper works even when the
// main ProcessManager hasn't been started yet (e.g. during recovery).
func (s *ManagedMCPServer) runShaper(action string, data map[string]interface{}) (string, error) {
	python, err := resolveBinary("python3")
	if err != nil {
		return "", fmt.Errorf("python3 not found: %w", err)
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"action": action,
		"query":  data["query"],
		"result": data["result"],
	})

	cmd := exec.Command(python, s.shaperPath)
	cmd.Stdin = bytes.NewReader(payload)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("shaper process: %w", err)
	}

	var res struct {
		Output string `json:"output"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		return "", fmt.Errorf("shaper output parse: %w", err)
	}
	if res.Error != "" {
		return "", fmt.Errorf("shaper: %s", res.Error)
	}
	return res.Output, nil
}

// ── Registry tool wrapper ──────────────────────────────────────────────────────

type mcpToolWrapper struct {
	server *ManagedMCPServer
	def    mcp.Tool
	name   string
}

func (w *mcpToolWrapper) Name() string                             { return w.name }
func (w *mcpToolWrapper) Description() string                      { return w.def.Description }
func (w *mcpToolWrapper) Category() tools.ToolCategory             { return tools.CategoryCustom }
func (w *mcpToolWrapper) RequiresPermission() bool                 { return true }
func (w *mcpToolWrapper) DefaultPermission() tools.PermissionLevel { return tools.PermissionOnce }
func (w *mcpToolWrapper) OutputFormat() tools.OutputFormat         { return tools.FormatText }

func (w *mcpToolWrapper) Parameters() json.RawMessage {
	b, _ := json.Marshal(w.def.InputSchema)
	return b
}

// authRequiredSentinel is the prefix Python MCP servers emit when a credential
// is absent. The Go layer intercepts it and converts it to a structured
// ToolResult that gives the AI (and user) clear authentication instructions.
const authRequiredSentinel = "[GORKBOT_AUTH_REQUIRED]"

func (w *mcpToolWrapper) Execute(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
	res, err := w.server.CallTool(ctx, w.def.Name, params)
	if err != nil {
		return &tools.ToolResult{Success: false, Error: err.Error()}, nil
	}

	if res.IsError {
		var errText strings.Builder
		for _, c := range res.Content {
			if tc, ok := c.(mcp.TextContent); ok {
				errText.WriteString(tc.Text)
			}
		}
		raw := errText.String()
		if strings.HasPrefix(raw, authRequiredSentinel) {
			return parseAuthRequired(raw), nil
		}
		return &tools.ToolResult{Success: false, Error: "MCP server error: " + raw}, nil
	}

	var output strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			output.WriteString(tc.Text)
		}
	}
	raw := output.String()
	// Even success responses may embed an auth sentinel (e.g. chat_with_notebook
	// returning auth-required text with IsError=false).
	if strings.HasPrefix(raw, authRequiredSentinel) {
		return parseAuthRequired(raw), nil
	}
	return &tools.ToolResult{Success: true, Output: raw}, nil
}

// parseAuthRequired converts a sentinel-prefixed string into a user-facing
// ToolResult with precise authentication instructions for the AI and user.
func parseAuthRequired(raw string) *tools.ToolResult {
	jsonPart := strings.TrimSpace(strings.TrimPrefix(raw, authRequiredSentinel))
	var payload struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal([]byte(jsonPart), &payload)

	var sb strings.Builder
	sb.WriteString("🔐 **Authentication Required**\n\n")
	sb.WriteString(payload.Message)
	sb.WriteString("\n\n")

	switch payload.Type {
	case "gemini_api_key":
		sb.WriteString("**How to fix:**\n")
		sb.WriteString("1. Open your `.env` file in the Gorkbot project directory.\n")
		sb.WriteString("2. Add or update: `GEMINI_API_KEY=your_key_here`\n")
		sb.WriteString("3. Restart Gorkbot.\n\n")
		sb.WriteString("Get a key at: https://aistudio.google.com/app/apikey")
	case "google_oauth":
		sb.WriteString("**How to fix:**\n")
		sb.WriteString("Run the following command to authenticate with Google:\n\n")
		sb.WriteString("  `/auth notebooklm login`\n\n")
		sb.WriteString("This starts a Device Authorization flow — enter the code shown\n")
		sb.WriteString("at https://www.google.com/device")
	default:
		sb.WriteString("Run `/auth notebooklm status` for details.")
	}

	return &tools.ToolResult{
		Success:      false,
		Output:       sb.String(),
		Error:        "authentication required: " + payload.Type,
		AuthRequired: true,
		AuthType:     payload.Type,
	}
}

// ── MCP Sampling Protocol ──────────────────────────────────────────────────────

// orchestratorSampler is a minimal interface to call back to the orchestrator
// for LLM sampling, avoiding a direct import cycle.
type orchestratorSampler interface {
	SampleOnce(ctx context.Context, model, systemPrompt, userPrompt string) (string, error)
}

// auditLogger is a minimal interface for recording tool/sampling executions.
type auditLogger interface {
	LogExecution(toolName, toolInput, result string, success bool, durationMs int64)
}

// samplingRateLimiter implements a simple token bucket (10 RPM per server).
type samplingRateLimiter struct {
	mu       sync.Mutex
	tokens   float64
	maxTok   float64
	lastTick time.Time
}

func newSamplingRateLimiter() *samplingRateLimiter {
	return &samplingRateLimiter{tokens: 10, maxTok: 10, lastTick: time.Now()}
}

// Allow returns true if a request is permitted (consumes 1 token).
func (r *samplingRateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(r.lastTick).Seconds()
	r.lastTick = now
	// Refill at 10 RPM = 1/6 token per second
	r.tokens += elapsed * (10.0 / 60.0)
	if r.tokens > r.maxTok {
		r.tokens = r.maxTok
	}
	if r.tokens < 1 {
		return false
	}
	r.tokens--
	return true
}

// samplingAllowedModels is the whitelist of models MCP servers may request.
var samplingAllowedModels = map[string]bool{
	"grok-3-mini": true, "grok-3-mini-fast": true,
	"gemini-2.0-flash": true, "gemini-2.5-flash": true,
	"claude-haiku-4-5": true,
}

// SamplingHandler processes MCP sampling/createMessage requests from MCP servers.
type SamplingHandler struct {
	sampler orchestratorSampler
	rl      *samplingRateLimiter
}

// NewSamplingHandler creates a SamplingHandler backed by the given sampler.
func NewSamplingHandler(sampler orchestratorSampler) *SamplingHandler {
	return &SamplingHandler{sampler: sampler, rl: newSamplingRateLimiter()}
}

// HandleSamplingRequest processes a sampling/createMessage request from an MCP server.
// model must be in samplingAllowedModels whitelist.
func (h *SamplingHandler) HandleSamplingRequest(ctx context.Context, model, systemPrompt, userPrompt string) (string, error) {
	if !samplingAllowedModels[model] {
		return "", fmt.Errorf("model %q not in sampling allowlist", model)
	}
	if !h.rl.Allow() {
		return "", fmt.Errorf("sampling rate limit exceeded (10 RPM)")
	}
	if h.sampler == nil {
		return "", fmt.Errorf("no sampler configured")
	}
	return h.sampler.SampleOnce(ctx, model, systemPrompt, userPrompt)
}
