package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ScraplingFetchTool - Fast HTTP fetching with Scrapling
type ScraplingFetchTool struct {
	BaseTool
}

func NewScraplingFetchTool() *ScraplingFetchTool {
	return &ScraplingFetchTool{
		BaseTool: BaseTool{
			name:               "scrapling_fetch",
			description:        "Fetch web pages using Scrapling with CSS/XPath selector support. Use when you need to extract specific elements from a page (e.g., 'div.content', '//table/tr'). Returns extracted content as text.",
			category:           CategoryWeb,
			requiresPermission: true,
			defaultPermission:  PermissionOnce,
		},
	}
}

func (t *ScraplingFetchTool) OutputFormat() OutputFormat {
	return FormatText
}

func (t *ScraplingFetchTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "URL to fetch",
			},
			"selector": map[string]interface{}{
				"type":        "string",
				"description": "CSS or XPath selector to extract elements",
			},
			"impersonate": map[string]interface{}{
				"type":        "string",
				"description": "Browser to impersonate (chrome, firefox, etc.)",
				"default":     "chrome",
			},
			"get_text": map[string]interface{}{
				"type":        "boolean",
				"description": "Extract text content only (vs HTML)",
				"default":     false,
			},
			"timeout": map[string]interface{}{
				"type":        "number",
				"description": "Request timeout in seconds",
				"default":     30,
			},
		},
		"required": []string{"url"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *ScraplingFetchTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	url, ok := params["url"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "url is required"}, fmt.Errorf("url required")
	}

	selector, _ := params["selector"].(string)
	impersonate, _ := params["impersonate"].(string)
	if impersonate == "" {
		impersonate = "chrome"
	}
	getText, _ := params["get_text"].(bool)
	timeout, _ := params["timeout"].(float64)
	if timeout == 0 {
		timeout = 30
	}

	// Use Python plugin instead
	pluginPath := filepath.Join(os.Getenv("HOME"), "project/gorkbot/plugins/python/scrapling_fetch/tool.py")

	// Check if plugin exists
	if _, err := os.Stat(pluginPath); err == nil {
		return t.executePlugin(ctx, pluginPath, params)
	}

	// Fallback to direct scrapling call
	return t.executeDirect(ctx, url, selector, impersonate, getText, int(timeout))
}

func (t *ScraplingFetchTool) executePlugin(ctx context.Context, pluginPath string, params map[string]interface{}) (*ToolResult, error) {
	input := map[string]interface{}{
		"action": "execute",
		"tool":   "fetch",
		"params": params,
	}
	inputJSON, _ := json.Marshal(input)

	cmd := exec.CommandContext(ctx, "python3", pluginPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "GORKBOT_PLUGIN=1")

	// Run via the plugin bridge
	bridgePath := filepath.Join(os.Getenv("HOME"), "project/gorkbot/plugins/python/gorkbot_bridge.py")
	cmd2 := exec.CommandContext(ctx, "python3", bridgePath)
	cmd2.Stdin = strings.NewReader(string(inputJSON))

	output, err := cmd2.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return &ToolResult{Success: false, Error: string(exitErr.Stderr)}, nil
		}
		return &ToolResult{Success: false, Error: err.Error()}, err
	}

	var result struct {
		Success bool   `json:"success"`
		Output  string `json:"output"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return &ToolResult{Success: false, Error: "Failed to parse result"}, err
	}

	return &ToolResult{
		Success: result.Success,
		Output:  result.Output,
		Error:   result.Error,
	}, nil
}

func (t *ScraplingFetchTool) executeDirect(ctx context.Context, url, selector, impersonate string, getText bool, timeout int) (*ToolResult, error) {
	// Build Python script for direct scraping
	script := fmt.Sprintf(`
import sys
sys.path.insert(0, '%s/plugins/python/scrapling_fetch')
from scrapling.fetchers import Fetcher, FetcherSession

with FetcherSession(impersonate='%s') as session:
    page = session.get('%s', timeout=%d)
    %s
`, os.Getenv("HOME"), impersonate, url, timeout,
		formatSelectorCode(selector, getText))

	cmd := exec.CommandContext(ctx, "python3", "-c", script)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return &ToolResult{Success: false, Error: string(exitErr.Stderr)}, nil
		}
		return &ToolResult{Success: false, Error: err.Error()}, err
	}

	return &ToolResult{
		Success: true,
		Output:  string(output),
	}, nil
}

func formatSelectorCode(selector string, getText bool) string {
	if selector == "" {
		return "print(page.text)"
	}
	if getText {
		return fmt.Sprintf("for e in page.css('%s'): print(e.text(strip=True))", selector)
	}
	return fmt.Sprintf("for e in page.css('%s'): print(e)", selector)
}

// ScraplingStealthTool - Stealthy fetch with Cloudflare bypass
type ScraplingStealthTool struct {
	BaseTool
}

func NewScraplingStealthTool() *ScraplingStealthTool {
	return &ScraplingStealthTool{
		BaseTool: BaseTool{
			name:               "scrapling_stealth",
			description:        "Stealthy web scraping with Cloudflare bypass and anti-bot evasion. Use when accessing protected sites that block regular requests. Returns extracted content as text.",
			category:           CategoryWeb,
			requiresPermission: true,
			defaultPermission:  PermissionOnce,
		},
	}
}

func (t *ScraplingStealthTool) OutputFormat() OutputFormat {
	return FormatText
}

func (t *ScraplingStealthTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "URL to fetch",
			},
			"selector": map[string]interface{}{
				"type":        "string",
				"description": "CSS or XPath selector to extract elements",
			},
			"headless": map[string]interface{}{
				"type":        "boolean",
				"description": "Run browser in headless mode",
				"default":     true,
			},
			"solve_cloudflare": map[string]interface{}{
				"type":        "boolean",
				"description": "Attempt to solve Cloudflare challenges",
				"default":     true,
			},
			"get_text": map[string]interface{}{
				"type":        "boolean",
				"description": "Extract text content only",
				"default":     false,
			},
			"timeout": map[string]interface{}{
				"type":        "number",
				"description": "Request timeout in seconds",
				"default":     60,
			},
		},
		"required": []string{"url"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *ScraplingStealthTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	url, ok := params["url"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "url is required"}, fmt.Errorf("url required")
	}

	selector, _ := params["selector"].(string)
	headless, _ := params["headless"].(bool)
	if headless == true {
		headless = true
	}
	solveCF, _ := params["solve_cloudflare"].(bool)
	getText, _ := params["get_text"].(bool)
	timeout, _ := params["timeout"].(float64)
	if timeout == 0 {
		timeout = 60
	}

	// Build Python script
	headlessStr := "True"
	if !headless {
		headlessStr = "False"
	}
	solveCFStr := "True"
	if !solveCF {
		solveCFStr = "False"
	}

	script := fmt.Sprintf(`
import sys
sys.path.insert(0, '%s/plugins/python/scrapling_stealth')
from scrapling.fetchers import StealthyFetcher

page = StealthyFetcher.fetch('%s', headless=%s, solve_cloudflare=%s, network_idle=True, timeout=%d)
%s
`, os.Getenv("HOME"), url, headlessStr, solveCFStr, int(timeout),
		formatSelectorCode(selector, getText))

	cmd := exec.CommandContext(ctx, "python3", "-c", script)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return &ToolResult{Success: false, Error: string(exitErr.Stderr)}, nil
		}
		return &ToolResult{Success: false, Error: err.Error()}, err
	}

	return &ToolResult{
		Success: true,
		Output:  string(output),
	}, nil
}

// ScraplingDynamicTool - Full browser automation
type ScraplingDynamicTool struct {
	BaseTool
}

func NewScraplingDynamicTool() *ScraplingDynamicTool {
	return &ScraplingDynamicTool{
		BaseTool: BaseTool{
			name:               "scrapling_dynamic",
			description:        "Full browser automation for dynamic JavaScript-heavy websites. Use when you need to render pages that require JavaScript execution. Returns extracted content as text.",
			category:           CategoryWeb,
			requiresPermission: true,
			defaultPermission:  PermissionOnce,
		},
	}
}

func (t *ScraplingDynamicTool) OutputFormat() OutputFormat {
	return FormatText
}

func (t *ScraplingDynamicTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "URL to fetch",
			},
			"selector": map[string]interface{}{
				"type":        "string",
				"description": "CSS or XPath selector to extract elements",
			},
			"headless": map[string]interface{}{
				"type":        "boolean",
				"description": "Run browser in headless mode",
				"default":     true,
			},
			"network_idle": map[string]interface{}{
				"type":        "boolean",
				"description": "Wait for network idle after page load",
				"default":     true,
			},
			"get_text": map[string]interface{}{
				"type":        "boolean",
				"description": "Extract text content only",
				"default":     false,
			},
			"timeout": map[string]interface{}{
				"type":        "number",
				"description": "Request timeout in seconds",
				"default":     60,
			},
		},
		"required": []string{"url"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *ScraplingDynamicTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	url, ok := params["url"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "url is required"}, fmt.Errorf("url required")
	}

	selector, _ := params["selector"].(string)
	headless, _ := params["headless"].(bool)
	networkIdle, _ := params["network_idle"].(bool)
	getText, _ := params["get_text"].(bool)
	timeout, _ := params["timeout"].(float64)
	if timeout == 0 {
		timeout = 60
	}

	headlessStr := "True"
	if !headless {
		headlessStr = "False"
	}
	networkIdleStr := "True"
	if !networkIdle {
		networkIdleStr = "False"
	}

	script := fmt.Sprintf(`
import sys
sys.path.insert(0, '%s/plugins/python/scrapling_dynamic')
from scrapling.fetchers import DynamicFetcher

page = DynamicFetcher.fetch('%s', headless=%s, network_idle=%s, timeout=%d)
%s
`, os.Getenv("HOME"), url, headlessStr, networkIdleStr, int(timeout),
		formatSelectorCode(selector, getText))

	cmd := exec.CommandContext(ctx, "python3", "-c", script)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return &ToolResult{Success: false, Error: string(exitErr.Stderr)}, nil
		}
		return &ToolResult{Success: false, Error: err.Error()}, err
	}

	return &ToolResult{
		Success: true,
		Output:  string(output),
	}, nil
}

// ScraplingExtractTool - CLI extract command
type ScraplingExtractTool struct {
	BaseTool
}

func NewScraplingExtractTool() *ScraplingExtractTool {
	return &ScraplingExtractTool{
		BaseTool: BaseTool{
			name:               "scrapling_extract",
			description:        "Extract content from URLs using Scrapling CLI with CSS selector support. Use for simple extractions without writing code. Returns extracted content as text.",
			category:           CategoryWeb,
			requiresPermission: true,
			defaultPermission:  PermissionOnce,
		},
	}
}

func (t *ScraplingExtractTool) OutputFormat() OutputFormat {
	return FormatText
}

func (t *ScraplingExtractTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "URL to extract from",
			},
			"selector": map[string]interface{}{
				"type":        "string",
				"description": "CSS selector for targeted extraction",
			},
			"impersonate": map[string]interface{}{
				"type":        "string",
				"description": "Browser to impersonate",
				"default":     "chrome",
			},
			"fetch_mode": map[string]interface{}{
				"type":        "string",
				"description": "Fetch mode: get, fetch, or stealthy-fetch",
				"default":     "get",
			},
			"output": map[string]interface{}{
				"type":        "string",
				"description": "Output file path (optional)",
			},
		},
		"required": []string{"url"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *ScraplingExtractTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	url, ok := params["url"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "url is required"}, fmt.Errorf("url required")
	}

	selector, _ := params["selector"].(string)
	impersonate, _ := params["impersonate"].(string)
	if impersonate == "" {
		impersonate = "chrome"
	}
	fetchMode, _ := params["fetch_mode"].(string)
	if fetchMode == "" {
		fetchMode = "get"
	}
	output, _ := params["output"].(string)

	// Build command
	args := []string{"scrapling", "extract", fetchMode, url}
	if selector != "" {
		args = append(args, "--css-selector", selector)
	}
	if impersonate != "" {
		args = append(args, "--impersonate", impersonate)
	}

	// Build shell command
	shellCmd := "scrapling extract " + fetchMode + " " + url
	if selector != "" {
		shellCmd += " --css-selector '" + selector + "'"
	}
	if impersonate != "" {
		shellCmd += " --impersonate " + impersonate
	}
	if output != "" {
		shellCmd += " -o " + output
	}

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": shellCmd,
	})
}

// ScraplingSearchTool - Web search using Scrapling
type ScraplingSearchTool struct {
	BaseTool
}

func NewScraplingSearchTool() *ScraplingSearchTool {
	return &ScraplingSearchTool{
		BaseTool: BaseTool{
			name:               "scrapling_search",
			description:        "Web search using Scrapling to scrape search engines (DuckDuckGo, Google, Bing). Returns clean search results. Use when web_search is unavailable.",
			category:           CategoryWeb,
			requiresPermission: true,
			defaultPermission:  PermissionOnce,
		},
	}
}

func (t *ScraplingSearchTool) OutputFormat() OutputFormat {
	return FormatText
}

func (t *ScraplingSearchTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query",
			},
			"engine": map[string]interface{}{
				"type":        "string",
				"description": "Search engine: duckduckgo, google, or bing",
				"default":     "duckduckgo",
			},
			"num_results": map[string]interface{}{
				"type":        "number",
				"description": "Number of results to return",
				"default":     10,
			},
			"safe_search": map[string]interface{}{
				"type":        "boolean",
				"description": "Enable safe search",
				"default":     true,
			},
			"locale": map[string]interface{}{
				"type":        "string",
				"description": "Locale for search (e.g., us-en)",
				"default":     "en-us",
			},
		},
		"required": []string{"query"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *ScraplingSearchTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	query, ok := params["query"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "query is required"}, fmt.Errorf("query required")
	}

	engine, _ := params["engine"].(string)
	if engine == "" {
		engine = "duckduckgo"
	}

	numResults, _ := params["num_results"].(float64)
	if numResults == 0 {
		numResults = 10
	}

	safeSearch, _ := params["safe_search"].(bool)
	locale, _ := params["locale"].(string)
	if locale == "" {
		locale = "en-us"
	}

	// Execute via Python plugin
	pluginPath := filepath.Join(os.Getenv("HOME"), "project/gorkbot/plugins/python/scrapling_search/tool.py")

	// Check if plugin exists
	if _, err := os.Stat(pluginPath); err == nil {
		return t.executePlugin(ctx, pluginPath, params)
	}

	// Fallback to inline Scrapling code
	return t.executeInline(ctx, query, engine, int(numResults), safeSearch, locale)
}

func (t *ScraplingSearchTool) executePlugin(ctx context.Context, pluginPath string, params map[string]interface{}) (*ToolResult, error) {
	input := map[string]interface{}{
		"action": "execute",
		"tool":   "search",
		"params": params,
	}
	inputJSON, _ := json.Marshal(input)

	// Use the gorkbot bridge
	bridgePath := filepath.Join(os.Getenv("HOME"), "project/gorkbot/plugins/python/gorkbot_bridge.py")
	cmd := exec.CommandContext(ctx, "python3", bridgePath)
	cmd.Stdin = strings.NewReader(string(inputJSON))
	cmd.Env = append(os.Environ(), "GORKBOT_PLUGIN=1")

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return &ToolResult{Success: false, Error: string(exitErr.Stderr)}, nil
		}
		return &ToolResult{Success: false, Error: err.Error()}, err
	}

	var result struct {
		Success bool   `json:"success"`
		Output  string `json:"output"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return &ToolResult{Success: false, Error: "Failed to parse result"}, err
	}

	return &ToolResult{
		Success: result.Success,
		Output:  result.Output,
		Error:   result.Error,
	}, nil
}

func (t *ScraplingSearchTool) executeInline(ctx context.Context, query, engine string, numResults int, safeSearch bool, locale string) (*ToolResult, error) {
	script := fmt.Sprintf(`
import sys
sys.path.insert(0, '%s/plugins/python/scrapling_search')
from scrapling.fetchers import FetcherSession
import urllib.parse

query = %q
num_results = %d
engine = %q

with FetcherSession(impersonate='chrome') as session:
    if engine == "duckduckgo":
        page = session.post('https://html.duckduckgo.com/html/', data={'q': query})
        results = page.css('.result')
    elif engine == "google":
        page = session.get(f'https://www.google.com/search?q={urllib.parse.quote(query)}&num={num_results}&hl={%q}')
        results = page.css('div.g')
    elif engine == "bing":
        page = session.get(f'https://www.bing.com/search?q={urllib.parse.quote(query)}&count={num_results}')
        results = page.css('.b_algo')
    else:
        print(f"Unknown engine: {engine}")
        sys.exit(1)

    output = f"Search results for: {query} ({engine})\\n\\n"

    for i, r in enumerate(results[:num_results], 1):
        if engine == "duckduckgo":
            title = r.css('.result__title')[0].text(strip=True) if r.css('.result__title') else ""
            url_elem = r.css('.result__url')
            url = url_elem[0].attrib.get('href', '') if url_elem else ""
            if 'uddg=' in url:
                url = urllib.parse.parse_qs(urllib.parse.urlparse(url).query).get('uddg', [''])[0]
            snippet = r.css('.result__snippet')[0].text(strip=True) if r.css('.result__snippet') else ""
        elif engine == "google":
            title_elem = r.css('h3')
            title = title_elem[0].text(strip=True) if title_elem else ""
            link = r.css('a')
            url = link[0].attrib.get('href', '') if link else ""
            snippet = r.css('.VwiC3b')[0].text(strip=True) if r.css('.VwiC3b') else ""
        else:  # bing
            title_elem = r.css('h2 a')
            title = title_elem[0].text(strip=True) if title_elem else ""
            url = title_elem[0].attrib.get('href', '') if title_elem else ""
            snippet = r.css('p')[0].text(strip=True) if r.css('p') else ""

        output += f"{i}. {title}\\n"
        output += f"   {url}\\n"
        if snippet:
            output += f"   {snippet[:200]}\\n"
        output += "\\n"

    print(output)
`, os.Getenv("HOME"), query, numResults, engine, locale)

	cmd := exec.CommandContext(ctx, "python3", "-c", script)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return &ToolResult{Success: false, Error: string(exitErr.Stderr)}, nil
		}
		return &ToolResult{Success: false, Error: err.Error()}, err
	}

	return &ToolResult{
		Success: true,
		Output:  string(output),
	}, nil
}
