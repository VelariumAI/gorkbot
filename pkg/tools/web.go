package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// WebFetchTool fetches content from URLs
type WebFetchTool struct {
	BaseTool
}

func NewWebFetchTool() *WebFetchTool {
	return &WebFetchTool{
		BaseTool: BaseTool{
			name:               "web_fetch",
			description:        "Fetch and parse web pages - returns clean text content (not raw HTML). Use this when you need to extract readable text from a URL.",
			category:           CategoryWeb,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *WebFetchTool) OutputFormat() OutputFormat {
	return FormatText
}

func (t *WebFetchTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "URL to fetch",
			},
			"method": map[string]interface{}{
				"type":        "string",
				"description": "HTTP method (default: GET)",
			},
			"headers": map[string]interface{}{
				"type":        "object",
				"description": "HTTP headers to include",
			},
			"follow_redirects": map[string]interface{}{
				"type":        "boolean",
				"description": "Follow redirects (default: true)",
			},
			"timeout": map[string]interface{}{
				"type":        "number",
				"description": "Timeout in seconds (default: 30)",
			},
			"raw": map[string]interface{}{
				"type":        "boolean",
				"description": "Return raw HTML instead of parsed text (default: false)",
				"default":     false,
			},
		},
		"required": []string{"url"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *WebFetchTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	url, ok := params["url"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "url is required"}, fmt.Errorf("url required")
	}

	raw, _ := params["raw"].(bool)

	// If raw HTML is requested, use simple curl
	if raw {
		return t.executeRaw(ctx, params)
	}

	// Otherwise, use Scrapling to get clean text
	return t.executeClean(ctx, url, params)
}

func (t *WebFetchTool) executeRaw(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	url := params["url"].(string)

	method := "GET"
	if m, ok := params["method"].(string); ok {
		method = m
	}

	followRedirects := true
	if f, ok := params["follow_redirects"].(bool); ok {
		followRedirects = f
	}

	timeout := 30
	if t, ok := params["timeout"].(float64); ok {
		timeout = int(t)
	}

	flags := fmt.Sprintf("-X %s --max-time %d", method, timeout)

	if !followRedirects {
		flags += " --no-location"
	} else {
		flags += " -L"
	}

	if headers, ok := params["headers"].(map[string]interface{}); ok {
		for key, value := range headers {
			flags += fmt.Sprintf(" -H %s", shellescape(fmt.Sprintf("%s: %v", key, value)))
		}
	}

	command := fmt.Sprintf("curl %s %s", flags, shellescape(url))

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
	})
}

func (t *WebFetchTool) executeClean(ctx context.Context, url string, params map[string]interface{}) (*ToolResult, error) {
	// Use scrapling to get clean text
	home := os.Getenv("HOME")
	if home == "" {
		home = "/data/data/com.termux/files/home"
	}

	pythonScript := fmt.Sprintf(`
import sys
import io
import contextlib

# Suppress verbose output
stderr_capture = io.StringIO()

sys.path.insert(0, %q)

with contextlib.redirect_stderr(stderr_capture):
    from scrapling.fetchers import FetcherSession

url = %q

try:
    with FetcherSession(impersonate='chrome') as session:
        page = session.get(url, timeout=30)
        # Get clean text content
        text = page.text[:50000]  # Limit to 50k chars

        print(text)
except Exception as e:
    print(f"ERROR: {e}", file=sys.stderr)
    sys.exit(1)
`, filepath.Join(home, "project/gorky/plugins/python/scrapling_fetch"), url)

	cmd := exec.CommandContext(ctx, "python3", "-c", pythonScript)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	if err == nil && stdout.Len() > 0 {
		return &ToolResult{
			Success:      true,
			Output:       stdout.String(),
			Data:         map[string]interface{}{"url": url, "method": "scrapling"},
			OutputFormat: FormatText,
		}, nil
	}

	// Log the error for debugging
	errMsg := fmt.Sprintf("scrapling failed: %v, stderr: %s", err, stderr.String())

	// Fallback: try lynx if available
	return t.executeLynx(ctx, url, errMsg)
}

func (t *WebFetchTool) executeLynx(ctx context.Context, url string, prevErr string) (*ToolResult, error) {
	// Try lynx as fallback for text extraction
	cmd := exec.CommandContext(ctx, "lynx", "-dump", "-nolist", url)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	if err == nil && stdout.Len() > 0 {
		// Clean up the output
		cleaned := strings.TrimSpace(stdout.String())
		return &ToolResult{
			Success:      true,
			Output:       cleaned,
			Data:         map[string]interface{}{"url": url, "method": "lynx"},
			OutputFormat: FormatText,
		}, nil
	}

	// Log why lynx failed
	lynxErr := fmt.Sprintf("lynx failed: %v, stderr: %s", err, stderr.String())

	// Final fallback: use curl with HTML tag stripping (fixed sed command)
	stripScript := fmt.Sprintf("curl -sL '%s' | sed 's/<[^>]*>//g' | grep -v '^$' | head -n 200", url)
	cmd = exec.CommandContext(ctx, "sh", "-c", stripScript)

	stdout.Reset()
	stderr.Reset()
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()

	if err != nil || stdout.Len() == 0 {
		// All methods failed - return detailed error
		return &ToolResult{
			Success:      false,
			Error:        fmt.Sprintf("failed to fetch %s. Tried: scrapling (error: %s), lynx (error: %s), curl+sed (error: %v, stderr: %s)", url, prevErr, lynxErr, err, stderr.String()),
			OutputFormat: FormatError,
		}, nil
	}

	return &ToolResult{
		Success:      true,
		Output:       stdout.String(),
		Data:         map[string]interface{}{"url": url, "method": "curl+sed"},
		OutputFormat: FormatText,
	}, nil
}

// HttpRequestTool makes advanced HTTP requests
type HttpRequestTool struct {
	BaseTool
}

func NewHttpRequestTool() *HttpRequestTool {
	return &HttpRequestTool{
		BaseTool: BaseTool{
			name:               "http_request",
			description:        "Make advanced HTTP requests with custom methods, headers, body, and authentication. Use for API calls, testing endpoints, or when you need fine-grained control over HTTP requests.",
			category:           CategoryWeb,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *HttpRequestTool) OutputFormat() OutputFormat {
	// Returns raw response (could be JSON, text, or binary)
	return FormatText
}

func (t *HttpRequestTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "URL to request",
			},
			"method": map[string]interface{}{
				"type":        "string",
				"description": "HTTP method (GET, POST, PUT, DELETE, etc.)",
			},
			"headers": map[string]interface{}{
				"type":        "object",
				"description": "HTTP headers",
			},
			"body": map[string]interface{}{
				"type":        "string",
				"description": "Request body (for POST/PUT)",
			},
			"json": map[string]interface{}{
				"type":        "object",
				"description": "JSON body (alternative to body)",
			},
			"auth": map[string]interface{}{
				"type":        "string",
				"description": "Basic auth in format 'username:password'",
			},
			"bearer": map[string]interface{}{
				"type":        "string",
				"description": "Bearer token for Authorization header",
			},
		},
		"required": []string{"url", "method"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *HttpRequestTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	url, ok := params["url"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "url is required", OutputFormat: FormatError}, fmt.Errorf("url required")
	}

	method, ok := params["method"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "method is required", OutputFormat: FormatError}, fmt.Errorf("method required")
	}

	flags := fmt.Sprintf("-X %s", method)

	// Add headers
	if headers, ok := params["headers"].(map[string]interface{}); ok {
		for key, value := range headers {
			flags += fmt.Sprintf(" -H %s", shellescape(fmt.Sprintf("%s: %v", key, value)))
		}
	}

	// Add body
	if body, ok := params["body"].(string); ok {
		flags += fmt.Sprintf(" -d %s", shellescape(body))
	}

	// Add JSON body
	if jsonBody, ok := params["json"].(map[string]interface{}); ok {
		jsonData, err := json.Marshal(jsonBody)
		if err != nil {
			return &ToolResult{Success: false, Error: "failed to marshal JSON", OutputFormat: FormatError}, err
		}
		flags += fmt.Sprintf(" -H 'Content-Type: application/json' -d %s", shellescape(string(jsonData)))
	}

	// Add auth
	if auth, ok := params["auth"].(string); ok {
		flags += fmt.Sprintf(" -u %s", shellescape(auth))
	}

	// Add bearer token
	if bearer, ok := params["bearer"].(string); ok {
		flags += fmt.Sprintf(" -H %s", shellescape(fmt.Sprintf("Authorization: Bearer %s", bearer)))
	}

	command := fmt.Sprintf("curl %s %s", flags, shellescape(url))

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
	})
}

// CheckPortTool checks if a port is open/listening
type CheckPortTool struct {
	BaseTool
}

func NewCheckPortTool() *CheckPortTool {
	return &CheckPortTool{
		BaseTool: BaseTool{
			name:               "check_port",
			description:        "Check if a TCP port is open and listening on localhost or a remote host. Use for debugging network services or checking if a server is running.",
			category:           CategoryWeb,
			requiresPermission: false,
			defaultPermission:  PermissionAlways,
		},
	}
}

func (t *CheckPortTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"port": map[string]interface{}{
				"type":        "number",
				"description": "Port number to check",
			},
			"host": map[string]interface{}{
				"type":        "string",
				"description": "Host to check (default: localhost)",
			},
			"timeout": map[string]interface{}{
				"type":        "number",
				"description": "Timeout in seconds (default: 5)",
			},
		},
		"required": []string{"port"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *CheckPortTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	port, ok := params["port"].(float64)
	if !ok {
		return &ToolResult{Success: false, Error: "port is required", OutputFormat: FormatError}, fmt.Errorf("port required")
	}

	host := "localhost"
	if h, ok := params["host"].(string); ok {
		host = h
	}

	timeout := 5
	if t, ok := params["timeout"].(float64); ok {
		timeout = int(t)
	}

	// Use nc (netcat) to check port, with timeout
	command := fmt.Sprintf("timeout %d nc -zv %s %d 2>&1 && echo 'Port is OPEN' || echo 'Port is CLOSED'",
		timeout, shellescape(host), int(port))

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
	})
}

// DownloadFileTool downloads files from URLs
type DownloadFileTool struct {
	BaseTool
}

func NewDownloadFileTool() *DownloadFileTool {
	return &DownloadFileTool{
		BaseTool: BaseTool{
			name:               "download_file",
			description:        "Download a file from a URL to the local filesystem. Use when you need to save a file from the web to disk.",
			category:           CategoryWeb,
			requiresPermission: true,
			defaultPermission:  PermissionOnce,
		},
	}
}

func (t *DownloadFileTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "URL to download from",
			},
			"output": map[string]interface{}{
				"type":        "string",
				"description": "Output file path",
			},
			"resume": map[string]interface{}{
				"type":        "boolean",
				"description": "Resume partial download (default: false)",
			},
			"follow_redirects": map[string]interface{}{
				"type":        "boolean",
				"description": "Follow redirects (default: true)",
			},
		},
		"required": []string{"url", "output"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *DownloadFileTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	url, ok := params["url"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "url is required", OutputFormat: FormatError}, fmt.Errorf("url required")
	}

	output, ok := params["output"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "output is required", OutputFormat: FormatError}, fmt.Errorf("output required")
	}

	resume := false
	if r, ok := params["resume"].(bool); ok {
		resume = r
	}

	followRedirects := true
	if f, ok := params["follow_redirects"].(bool); ok {
		followRedirects = f
	}

	flags := "-o " + shellescape(output)

	if resume {
		flags += " -C -"
	}

	if followRedirects {
		flags += " -L"
	}

	// Show progress
	flags += " --progress-bar"

	command := fmt.Sprintf("curl %s %s", flags, shellescape(url))

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
		"timeout": 300, // 5 minutes for downloads
	})
}
