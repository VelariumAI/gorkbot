package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
)

// Client manages a connection to one MCP server over stdio transport.
type Client struct {
	cfg    ServerConfig
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader

	mu      sync.Mutex
	nextID  int64
	pending map[int]chan *Response

	tools []ToolDefinition
	ready bool
}

// NewStdioClient creates a Client for a stdio-transport server.
// Call Handshake() before using the client.
func NewStdioClient(cfg ServerConfig) *Client {
	return &Client{
		cfg:     cfg,
		pending: make(map[int]chan *Response),
	}
}

// Start launches the child process and starts the reader goroutine.
func (c *Client) Start(ctx context.Context) error {
	// #nosec G204 — args come from user config, not user input
	cmd := exec.CommandContext(ctx, c.cfg.Command, c.cfg.Args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Set extra env vars
	if len(c.cfg.Env) > 0 {
		env := os.Environ()
		for k, v := range c.cfg.Env {
			env = append(env, k+"="+v)
		}
		cmd.Env = env
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("mcp: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("mcp: stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr // let MCP server errors surface in our stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("mcp: start %q: %w", c.cfg.Command, err)
	}

	c.cmd = cmd
	c.stdin = stdin
	c.stdout = bufio.NewReader(stdout)

	go c.readLoop()
	return nil
}

// Handshake performs the MCP initialization exchange.
func (c *Client) Handshake(ctx context.Context) error {
	result, err := c.call(ctx, "initialize", InitializeParams{
		ProtocolVersion: "2024-11-05",
		Capabilities:    ClientCaps{},
		ClientInfo:      ClientInfo{Name: "gorkbot", Version: "4.5.2"},
	})
	if err != nil {
		return fmt.Errorf("mcp: initialize: %w", err)
	}

	var init InitializeResult
	if err := json.Unmarshal(result, &init); err != nil {
		return fmt.Errorf("mcp: parse initialize result: %w", err)
	}

	// Confirm initialization
	if err := c.notify("notifications/initialized", nil); err != nil {
		return fmt.Errorf("mcp: notifications/initialized: %w", err)
	}

	c.ready = true
	return nil
}

// ListTools fetches available tools from the MCP server and caches them.
func (c *Client) ListTools(ctx context.Context) ([]ToolDefinition, error) {
	if !c.ready {
		return nil, fmt.Errorf("mcp: client not initialized")
	}

	result, err := c.call(ctx, "tools/list", nil)
	if err != nil {
		return nil, fmt.Errorf("mcp: tools/list: %w", err)
	}

	var listResult ListToolsResult
	if err := json.Unmarshal(result, &listResult); err != nil {
		return nil, fmt.Errorf("mcp: parse tools/list result: %w", err)
	}

	c.tools = listResult.Tools
	return c.tools, nil
}

// CallTool executes a tool on the MCP server.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]interface{}) (*CallToolResult, error) {
	if !c.ready {
		return nil, fmt.Errorf("mcp: client not initialized")
	}

	result, err := c.call(ctx, "tools/call", CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		return nil, fmt.Errorf("mcp: tools/call %q: %w", name, err)
	}

	var callResult CallToolResult
	if err := json.Unmarshal(result, &callResult); err != nil {
		return nil, fmt.Errorf("mcp: parse tools/call result: %w", err)
	}

	return &callResult, nil
}

// CachedTools returns the tools discovered during the last ListTools call.
func (c *Client) CachedTools() []ToolDefinition { return c.tools }

// ServerName returns the configured server name.
func (c *Client) ServerName() string { return c.cfg.Name }

// ServerDescription returns the configured server description.
func (c *Client) ServerDescription() string { return c.cfg.Description }

// Stop terminates the child process.
func (c *Client) Stop() {
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		_ = c.cmd.Wait()
	}
}

// ── internal JSON-RPC plumbing ───────────────────────────────────────────────

func (c *Client) call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	id := int(atomic.AddInt64(&c.nextID, 1))

	ch := make(chan *Response, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	req := Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}

	c.mu.Lock()
	_, err = fmt.Fprintf(c.stdin, "%s\n", data)
	c.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("write to mcp server: %w", err)
	}

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	case resp := <-ch:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	}
}

func (c *Client) notify(method string, params interface{}) error {
	req := Request{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err = fmt.Fprintf(c.stdin, "%s\n", data)
	return err
}

func (c *Client) readLoop() {
	for {
		line, err := c.stdout.ReadString('\n')
		if err != nil {
			return
		}
		if len(line) == 0 {
			continue
		}

		var resp Response
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			continue // skip non-JSON (e.g. server debug output)
		}

		c.mu.Lock()
		ch, exists := c.pending[resp.ID]
		if exists {
			delete(c.pending, resp.ID)
		}
		c.mu.Unlock()

		if exists {
			ch <- &resp
		}
	}
}
