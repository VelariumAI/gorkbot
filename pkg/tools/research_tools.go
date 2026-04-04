package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/velariumai/gorkbot/pkg/research"
)

// researchEngineContextKey is the context key for the shared research engine.
var researchEngineContextKey = &contextKey{"researchEngine"}

// ── BrowserSearchTool ────────────────────────────────────────────────────────

type BrowserSearchTool struct {
	BaseTool
}

func NewBrowserSearchTool() *BrowserSearchTool {
	return &BrowserSearchTool{
		BaseTool: NewBaseTool(
			"browser_search",
			"Search the web and return structured results (title, URL, snippet). No page content is fetched — use browser_open to read a page.",
			CategoryWeb,
			true,
			PermissionSession,
		),
	}
}

func (t *BrowserSearchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "Search query"
			},
			"top_k": {
				"type": "integer",
				"description": "Max number of results (default 10)"
			}
		},
		"required": ["query"]
	}`)
}

func (t *BrowserSearchTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	engine := getResearchEngine(ctx)
	if engine == nil {
		return &ToolResult{Success: false, Error: "research engine not available"}, nil
	}

	query, _ := params["query"].(string)
	if query == "" {
		return &ToolResult{Success: false, Error: "query is required"}, nil
	}

	topK := 10
	if k, ok := params["top_k"].(float64); ok {
		topK = int(k)
	}

	results, err := engine.Search(ctx, query, topK)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("search failed: %v", err)}, nil
	}

	if len(results) == 0 {
		return &ToolResult{Success: true, Output: "No results found."}, nil
	}

	var sb strings.Builder
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. %s\n   %s\n   %s\n\n", i+1, r.Title, r.URL, r.Snippet))
	}

	return &ToolResult{
		Success:      true,
		Output:       sb.String(),
		OutputFormat: FormatList,
	}, nil
}

// ── BrowserOpenTool ──────────────────────────────────────────────────────────

type BrowserOpenTool struct {
	BaseTool
}

func NewBrowserOpenTool() *BrowserOpenTool {
	return &BrowserOpenTool{
		BaseTool: NewBaseTool(
			"browser_open",
			"Open a URL and buffer its content. Returns only a summary (title, length, preview). Full content stays in the buffer — use browser_find to search it.",
			CategoryWeb,
			true,
			PermissionSession,
		),
	}
}

func (t *BrowserOpenTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": {
				"type": "string",
				"description": "URL to open and buffer"
			}
		},
		"required": ["url"]
	}`)
}

func (t *BrowserOpenTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	engine := getResearchEngine(ctx)
	if engine == nil {
		return &ToolResult{Success: false, Error: "research engine not available"}, nil
	}

	url, _ := params["url"].(string)
	if url == "" {
		return &ToolResult{Success: false, Error: "url is required"}, nil
	}

	summary, err := engine.Open(ctx, url)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("open failed: %v", err)}, nil
	}

	data, _ := json.MarshalIndent(summary, "", "  ")
	return &ToolResult{
		Success:      true,
		Output:       fmt.Sprintf("Page buffered successfully (%d chars).\n\n%s", summary.Length, string(data)),
		OutputFormat: FormatJSON,
	}, nil
}

// ── BrowserFindTool ──────────────────────────────────────────────────────────

type BrowserFindTool struct {
	BaseTool
}

func NewBrowserFindTool() *BrowserFindTool {
	return &BrowserFindTool{
		BaseTool: NewBaseTool(
			"browser_find",
			"Search the currently active buffered document for a pattern. Returns only matching excerpts with context lines — not the full page.",
			CategoryWeb,
			false,
			PermissionAlways,
		),
	}
}

func (t *BrowserFindTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "Regex or literal pattern to search for in the buffered page"
			},
			"context_lines": {
				"type": "integer",
				"description": "Number of surrounding lines per match (default 2)"
			}
		},
		"required": ["pattern"]
	}`)
}

func (t *BrowserFindTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	engine := getResearchEngine(ctx)
	if engine == nil {
		return &ToolResult{Success: false, Error: "research engine not available"}, nil
	}

	pattern, _ := params["pattern"].(string)
	if pattern == "" {
		return &ToolResult{Success: false, Error: "pattern is required"}, nil
	}

	contextLines := 2
	if cl, ok := params["context_lines"].(float64); ok {
		contextLines = int(cl)
	}

	matches, err := engine.Find(pattern, contextLines)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("find failed: %v", err)}, nil
	}

	if len(matches) == 0 {
		return &ToolResult{Success: true, Output: "No matches found."}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d match(es):\n\n", len(matches)))
	for i, m := range matches {
		sb.WriteString(fmt.Sprintf("── Match %d (line %d) ──\n", i+1, m.LineNumber))
		sb.WriteString(m.Context)
		sb.WriteString("\n\n")
	}

	return &ToolResult{
		Success:      true,
		Output:       sb.String(),
		OutputFormat: FormatText,
	}, nil
}

// ── Registration ─────────────────────────────────────────────────────────────

func RegisterResearchTools(reg *Registry) {
	_ = reg.Register(NewBrowserSearchTool())
	_ = reg.Register(NewBrowserOpenTool())
	_ = reg.Register(NewBrowserFindTool())
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func getResearchEngine(ctx context.Context) *research.Engine {
	if e, ok := ctx.Value(researchEngineContextKey).(*research.Engine); ok {
		return e
	}
	return nil
}
