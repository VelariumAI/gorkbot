package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"time"
)

// sandboxAllowedTools is the safe allowlist of tools callable from inside the
// Python sandbox via the gorkbot_tools RPC module.
var sandboxAllowedTools = map[string]bool{
	"read_file":      true,
	"write_file":     true,
	"list_directory": true,
	"grep_content":   true,
	"search_files":   true,
	"web_fetch":      true,
	"http_request":   true,
	"session_search": true,
}

// gorkbotToolsPy is the content of gorkbot_tools.py written into the sandbox
// temp directory so Python code can do `import gorkbot_tools` and call tools
// through the Unix-domain RPC socket.
const gorkbotToolsPy = `import socket, json, os, sys

_sock_path = os.environ.get('GORKBOT_RPC_SOCKET', '')

def _call(tool, **params):
    if not _sock_path:
        raise RuntimeError('GORKBOT_RPC_SOCKET not set')
    with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as s:
        s.settimeout(30)
        s.connect(_sock_path)
        msg = json.dumps({"tool": tool, "params": params}).encode() + b'\n'
        s.sendall(msg)
        data = b''
        while True:
            chunk = s.recv(65536)
            if not chunk:
                break
            data += chunk
            if data.endswith(b'\n'):
                break
    resp = json.loads(data)
    if not resp.get('success'):
        raise RuntimeError(resp.get('error', 'tool error'))
    return resp.get('output', '')

def read_file(path): return _call('read_file', path=path)
def write_file(path, content): return _call('write_file', path=path, content=content)
def list_directory(path='.'): return _call('list_directory', path=path)
def grep_content(pattern, path='.'): return _call('grep_content', pattern=pattern, path=path)
def search_files(pattern, path='.'): return _call('search_files', pattern=pattern, path=path)
def web_fetch(url): return _call('web_fetch', url=url)
def http_request(url, method='GET', body=''): return _call('http_request', url=url, method=method, body=body)
def session_search(query, days=0, top_k=5): return _call('session_search', query=query, days=days, top_k=top_k)

def json_parse(text):
    import json as _json
    try:
        return _json.loads(text)
    except Exception:
        return text

def retry(fn, times=3, delay=1.0):
    import time
    for i in range(times):
        try:
            return fn()
        except Exception as e:
            if i == times - 1:
                raise
            time.sleep(delay * (i + 1))
`

// limitedBuffer is an io.Writer that caps buffered output to a fixed byte
// limit, silently dropping excess bytes and recording a truncation flag.
type limitedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (lb *limitedBuffer) Write(p []byte) (int, error) {
	if lb.buf.Len() >= lb.limit {
		lb.truncated = true
		return len(p), nil // silently drop
	}
	n := lb.limit - lb.buf.Len()
	if len(p) > n {
		lb.truncated = true
		p = p[:n]
	}
	return lb.buf.Write(p)
}

func (lb *limitedBuffer) String() string {
	s := lb.buf.String()
	if lb.truncated {
		s += "\n[... output truncated ...]"
	}
	return s
}

// PythonSandboxTool executes Python code in an isolated temp directory with
// access to a curated set of Gorkbot tools via the gorkbot_tools RPC module.
type PythonSandboxTool struct {
	BaseTool
	registry *Registry // wired after construction via SetRegistry
}

// NewPythonSandboxTool creates the python_execute tool.
func NewPythonSandboxTool() *PythonSandboxTool {
	return &PythonSandboxTool{
		BaseTool: NewBaseTool(
			"python_execute",
			"Execute Python code in a sandboxed environment with access to a safe subset of Gorkbot tools via gorkbot_tools module",
			CategoryShell,
			true,
			PermissionOnce,
		),
	}
}

// SetRegistry wires the tool registry so sandboxed Python code can invoke
// allowed tools through the RPC socket.
func (p *PythonSandboxTool) SetRegistry(reg *Registry) { p.registry = reg }

// Parameters returns the JSON schema for python_execute.
func (p *PythonSandboxTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"code": map[string]interface{}{
				"type":        "string",
				"description": "The Python code to execute",
			},
		},
		"required": []string{"code"},
	}
	data, _ := json.Marshal(schema)
	return data
}

// Execute runs the Python code in a sandboxed temp directory, exposing a Unix
// domain RPC socket at GORKBOT_RPC_SOCKET so the code can call allowed tools.
func (p *PythonSandboxTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	code, ok := params["code"].(string)
	if !ok || code == "" {
		return &ToolResult{Success: false, Error: "code parameter required"}, nil
	}

	// Create isolated temp dir
	tmpDir, err := os.MkdirTemp("", "gorkbot-py-*")
	if err != nil {
		return &ToolResult{Success: false, Error: "failed to create sandbox dir: " + err.Error()}, nil
	}
	defer os.RemoveAll(tmpDir)

	// Write gorkbot_tools.py into the sandbox
	if err := os.WriteFile(filepath.Join(tmpDir, "gorkbot_tools.py"), []byte(gorkbotToolsPy), 0644); err != nil {
		return &ToolResult{Success: false, Error: "failed to write gorkbot_tools.py: " + err.Error()}, nil
	}

	// Create Unix domain socket for RPC
	sockPath := filepath.Join(tmpDir, "rpc.sock")
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		return &ToolResult{Success: false, Error: "failed to create RPC socket: " + err.Error()}, nil
	}
	defer listener.Close()

	// Start RPC server goroutine — survives for the full 300 s budget
	callCount := &atomic.Int32{}
	rpcCtx, rpcCancel := context.WithTimeout(ctx, 300*time.Second)
	defer rpcCancel()

	go p.serveRPC(rpcCtx, listener, callCount)

	// Run python3 with a 300 s hard deadline
	execCtx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "python3", "-c", code)
	cmd.Dir = tmpDir
	cmd.Env = append(os.Environ(),
		"GORKBOT_RPC_SOCKET="+sockPath,
		"PYTHONPATH="+tmpDir,
	)

	// Cap stdout at 100 KB, stderr at 50 KB
	var stdout limitedBuffer
	var stderr limitedBuffer
	stdout.limit = 100 * 1024
	stderr.limit = 50 * 1024
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	output := stdout.String()
	if se := stderr.String(); se != "" {
		output += "\n[stderr]: " + se
	}

	if runErr != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			return &ToolResult{
				Success: false,
				Error:   "python execution timed out after 300s",
				Output:  output,
			}, nil
		}
		// Non-zero exit is reported but not a hard tool error so partial
		// output is still visible to the AI.
		return &ToolResult{
			Success: false,
			Error:   runErr.Error(),
			Output:  output,
		}, nil
	}

	return &ToolResult{Success: true, Output: output}, nil
}

// serveRPC accepts connections on the Unix socket and dispatches each in its
// own goroutine until the context is cancelled or the listener is closed.
func (p *PythonSandboxTool) serveRPC(ctx context.Context, listener net.Listener, callCount *atomic.Int32) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go p.handleRPCConn(ctx, conn, callCount)
	}
}

// handleRPCConn processes a single tool call request from the Python sandbox.
func (p *PythonSandboxTool) handleRPCConn(ctx context.Context, conn net.Conn, callCount *atomic.Int32) {
	defer conn.Close()

	// Enforce per-sandbox call cap
	if callCount.Add(1) > 50 {
		resp := map[string]interface{}{"success": false, "error": "sandbox tool call limit exceeded"}
		data, _ := json.Marshal(resp)
		conn.Write(append(data, '\n'))
		return
	}

	var req struct {
		Tool   string                 `json:"tool"`
		Params map[string]interface{} `json:"params"`
	}
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		return
	}

	// Allowlist check
	if !sandboxAllowedTools[req.Tool] {
		resp := map[string]interface{}{"success": false, "error": "tool not allowed in sandbox: " + req.Tool}
		data, _ := json.Marshal(resp)
		conn.Write(append(data, '\n'))
		return
	}

	// Registry availability check
	if p.registry == nil {
		resp := map[string]interface{}{"success": false, "error": "registry not available"}
		data, _ := json.Marshal(resp)
		conn.Write(append(data, '\n'))
		return
	}

	// Dispatch via the real tool registry
	toolReq := &ToolRequest{ToolName: req.Tool, Parameters: req.Params}
	result, err := p.registry.Execute(ctx, toolReq)
	var resp map[string]interface{}
	if err != nil {
		resp = map[string]interface{}{"success": false, "error": err.Error()}
	} else if result.Success {
		resp = map[string]interface{}{"success": true, "output": result.Output}
	} else {
		resp = map[string]interface{}{"success": false, "error": result.Error}
	}
	data, _ := json.Marshal(resp)
	conn.Write(append(data, '\n'))
}
