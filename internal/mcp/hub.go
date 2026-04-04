package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	mcptransport "github.com/mark3labs/mcp-go/client/transport"
	mcpproto "github.com/mark3labs/mcp-go/mcp"
)

// MCPServer represents an MCP (Model Context Protocol) server
type MCPServer struct {
	Name     string
	Path     string
	Command  string
	Args     []string
	Enabled  bool
	Tools    []MCPTool
	Status   string // "running", "stopped", "error"
	LastPing time.Time
}

// MCPTool represents a tool exposed by an MCP server
type MCPTool struct {
	Name        string
	Description string
	InputSchema map[string]interface{}
	ServerName  string
}

// MCPHub manages MCP server discovery and coordination
type MCPHub struct {
	logger           *slog.Logger
	servers          map[string]*MCPServer
	tools            map[string]*MCPTool
	configDir        string
	executor         mcpExecutor
	executionTimeout time.Duration
	healthTimeout    time.Duration
	mu               sync.RWMutex
}

type mcpExecutor interface {
	ExecuteTool(ctx context.Context, server *MCPServer, tool *MCPTool, input map[string]interface{}) (interface{}, error)
	HealthCheck(ctx context.Context, server *MCPServer) error
}

type stdioExecutor struct {
	logger *slog.Logger
}

// NewMCPHub creates a new MCP hub
func NewMCPHub(logger *slog.Logger) *MCPHub {
	if logger == nil {
		logger = slog.Default()
	}

	configDir := expandPath("~/.config/gorkbot/mcp")

	return &MCPHub{
		logger:           logger,
		servers:          make(map[string]*MCPServer),
		tools:            make(map[string]*MCPTool),
		configDir:        configDir,
		executionTimeout: 30 * time.Second,
		healthTimeout:    5 * time.Second,
		executor:         &stdioExecutor{logger: logger},
	}
}

// SetExecutor overrides MCP execution behavior (primarily for tests).
func (h *MCPHub) SetExecutor(executor mcpExecutor) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.executor = executor
}

// DiscoverServers discovers MCP servers in config directory
func (h *MCPHub) DiscoverServers() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Create config dir if doesn't exist
	if _, err := os.Stat(h.configDir); os.IsNotExist(err) {
		os.MkdirAll(h.configDir, 0755)
		h.logger.Info("created MCP config directory", slog.String("path", h.configDir))
		return nil
	}

	// Scan for .toml or .json config files
	entries, err := os.ReadDir(h.configDir)
	if err != nil {
		return fmt.Errorf("failed to read MCP config: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			// Check for default config in subdirectory
			configPath := filepath.Join(h.configDir, entry.Name(), "config.toml")
			if _, err := os.Stat(configPath); err == nil {
				server, err := h.loadServerConfig(configPath)
				if err == nil {
					h.servers[server.Name] = server
					h.logger.Debug("discovered MCP server",
						slog.String("name", server.Name),
						slog.String("path", server.Path),
					)
				}
			}
		}
	}

	h.logger.Info("MCP discovery complete",
		slog.Int("servers_found", len(h.servers)),
	)

	return nil
}

// loadServerConfig loads a server configuration
func (h *MCPHub) loadServerConfig(configPath string) (*MCPServer, error) {
	// Parse TOML config (simplified for now)
	serverName := filepath.Base(filepath.Dir(configPath))

	server := &MCPServer{
		Name:    serverName,
		Path:    filepath.Dir(configPath),
		Enabled: true,
		Tools:   []MCPTool{},
		Status:  "stopped",
	}

	return server, nil
}

// RegisterServer manually registers an MCP server
func (h *MCPHub) RegisterServer(server *MCPServer) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if server.Name == "" {
		return fmt.Errorf("server name required")
	}

	h.servers[server.Name] = server

	h.logger.Debug("registered MCP server",
		slog.String("name", server.Name),
		slog.Int("tools", len(server.Tools)),
	)

	return nil
}

// GetServer retrieves a server
func (h *MCPHub) GetServer(name string) *MCPServer {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.servers[name]
}

// GetTools returns all available tools from all servers
func (h *MCPHub) GetTools() map[string]*MCPTool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	tools := make(map[string]*MCPTool)
	for name, tool := range h.tools {
		tools[name] = tool
	}
	return tools
}

// RegisterTool registers a tool from an MCP server
func (h *MCPHub) RegisterTool(tool *MCPTool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.tools[tool.Name] = tool

	h.logger.Debug("registered MCP tool",
		slog.String("name", tool.Name),
		slog.String("server", tool.ServerName),
	)
}

// ExecuteTool executes a tool from an MCP server
func (h *MCPHub) ExecuteTool(toolName string, input map[string]interface{}) (interface{}, error) {
	return h.ExecuteToolContext(context.Background(), toolName, input)
}

// ExecuteToolContext executes a tool with caller-provided context.
func (h *MCPHub) ExecuteToolContext(ctx context.Context, toolName string, input map[string]interface{}) (interface{}, error) {
	h.mu.RLock()
	tool, ok := h.tools[toolName]
	executor := h.executor
	executionTimeout := h.executionTimeout
	h.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("tool not found: %s", toolName)
	}

	// Find server
	h.mu.RLock()
	server := h.servers[tool.ServerName]
	h.mu.RUnlock()

	if server == nil {
		return nil, fmt.Errorf("server not found: %s", tool.ServerName)
	}
	if !server.Enabled {
		return nil, fmt.Errorf("server disabled: %s", tool.ServerName)
	}
	if executor == nil {
		return nil, fmt.Errorf("mcp executor not configured")
	}

	h.logger.Debug("executing MCP tool",
		slog.String("tool", toolName),
		slog.String("server", tool.ServerName),
	)

	callCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline && executionTimeout > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, executionTimeout)
		defer cancel()
	}

	result, err := executor.ExecuteTool(callCtx, server, tool, input)
	now := time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()
	if tracked := h.servers[tool.ServerName]; tracked != nil {
		tracked.LastPing = now
		if err != nil {
			tracked.Status = "error"
		} else {
			tracked.Status = "running"
		}
	}
	if err != nil {
		return nil, fmt.Errorf("mcp tool execution failed (%s/%s): %w", tool.ServerName, toolName, err)
	}
	return result, nil
}

// HealthCheck checks if an MCP server is healthy
func (h *MCPHub) HealthCheck(serverName string) error {
	h.mu.RLock()
	server := h.servers[serverName]
	executor := h.executor
	healthTimeout := h.healthTimeout
	h.mu.RUnlock()

	if server == nil {
		return fmt.Errorf("server not found: %s", serverName)
	}
	if executor == nil {
		return fmt.Errorf("mcp executor not configured")
	}
	if !server.Enabled {
		return fmt.Errorf("server disabled: %s", serverName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), healthTimeout)
	defer cancel()
	err := executor.HealthCheck(ctx, server)

	h.mu.Lock()
	defer h.mu.Unlock()
	tracked := h.servers[serverName]
	if tracked != nil {
		tracked.LastPing = time.Now()
		if err != nil {
			tracked.Status = "error"
		} else {
			tracked.Status = "running"
		}
	}
	if err != nil {
		return fmt.Errorf("mcp server health check failed (%s): %w", serverName, err)
	}

	h.logger.Debug("MCP server health check passed",
		slog.String("server", serverName),
	)

	return nil
}

// GetStats returns MCP hub statistics
func (h *MCPHub) GetStats() map[string]interface{} {
	h.mu.RLock()
	defer h.mu.RUnlock()

	activeServers := 0
	for _, server := range h.servers {
		if server.Status == "running" {
			activeServers++
		}
	}

	return map[string]interface{}{
		"servers":        len(h.servers),
		"active_servers": activeServers,
		"tools":          len(h.tools),
		"config_dir":     h.configDir,
	}
}

// ListServers returns all servers
func (h *MCPHub) ListServers() []*MCPServer {
	h.mu.RLock()
	defer h.mu.RUnlock()

	servers := make([]*MCPServer, 0, len(h.servers))
	for _, server := range h.servers {
		servers = append(servers, server)
	}
	return servers
}

// ListTools returns all tools
func (h *MCPHub) ListTools() []*MCPTool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	tools := make([]*MCPTool, 0, len(h.tools))
	for _, tool := range h.tools {
		tools = append(tools, tool)
	}
	return tools
}

// Helper function
func expandPath(path string) string {
	if path == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if len(path) > 0 && path[0] == '~' {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[1:])
	}
	return path
}

func (e *stdioExecutor) ExecuteTool(ctx context.Context, server *MCPServer, tool *MCPTool, input map[string]interface{}) (interface{}, error) {
	mcpClient, cleanup, err := e.newClient(ctx, server)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	res, err := mcpClient.CallTool(ctx, mcpproto.CallToolRequest{
		Params: mcpproto.CallToolParams{
			Name:      tool.Name,
			Arguments: input,
		},
	})
	if err != nil {
		return nil, err
	}
	return toPlainMap(res)
}

func (e *stdioExecutor) HealthCheck(ctx context.Context, server *MCPServer) error {
	mcpClient, cleanup, err := e.newClient(ctx, server)
	if err != nil {
		return err
	}
	defer cleanup()
	_, err = mcpClient.ListTools(ctx, mcpproto.ListToolsRequest{})
	return err
}

func (e *stdioExecutor) newClient(ctx context.Context, server *MCPServer) (*mcpclient.Client, func(), error) {
	if server == nil {
		return nil, nil, fmt.Errorf("server is nil")
	}
	if server.Command == "" {
		return nil, nil, fmt.Errorf("server command is empty for %s", server.Name)
	}

	transport := mcptransport.NewStdio(server.Command, nil, server.Args...)
	mcpClient := mcpclient.NewClient(transport)
	if err := mcpClient.Start(ctx); err != nil {
		return nil, nil, fmt.Errorf("mcp transport start failed: %w", err)
	}
	initReq := mcpproto.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcpproto.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcpproto.Implementation{
		Name:    "GorkbotMCPHub",
		Version: "1.0.0",
	}
	if _, err := mcpClient.Initialize(ctx, initReq); err != nil {
		_ = mcpClient.Close()
		return nil, nil, fmt.Errorf("mcp initialize failed: %w", err)
	}
	cleanup := func() { _ = mcpClient.Close() }
	return mcpClient, cleanup, nil
}

func toPlainMap(v interface{}) (map[string]interface{}, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal mcp response: %w", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode mcp response: %w", err)
	}
	return out, nil
}
