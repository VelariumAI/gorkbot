package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/velariumai/gorkbot/pkg/ai"
)

// WebSearchTool searches the web for information
type WebSearchTool struct {
	BaseTool
}

func NewWebSearchTool() *WebSearchTool {
	return &WebSearchTool{
		BaseTool: BaseTool{
			name:        "web_search",
			description: "Search the web for real-time information. Returns clean search results with titles, URLs, and snippets. Use when you need to find web resources on a topic.",
			category:    CategoryWeb,
			requiresPermission: true,
			defaultPermission: PermissionSession,
		},
	}
}

func (t *WebSearchTool) OutputFormat() OutputFormat {
	return FormatText
}

func (t *WebSearchTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query",
			},
			"num_results": map[string]interface{}{
				"type":        "number",
				"description": "Number of results to return (default: 5, max: 10)",
			},
		},
		"required": []string{"query"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *WebSearchTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	query, ok := params["query"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "query is required"}, fmt.Errorf("query required")
	}

	numResults := 5
	if n, ok := params["num_results"].(float64); ok {
		numResults = int(n)
		if numResults > 20 {
			numResults = 20
		}
	}

	// Use quiet_search.py to suppress verbose Scrapling output
	quietPluginPath := filepath.Join(os.Getenv("HOME"), "project/gorkbot/plugins/python/scrapling_search/quiet_search.py")

	input := map[string]interface{}{
		"query":        query,
		"num_results":  numResults,
		"engine":       "duckduckgo",
	}
	inputJSON, _ := json.Marshal(input)

	cmd := exec.CommandContext(ctx, "python3", quietPluginPath)
	cmd.Stdin = os.Stdin

	// Pipe input
	inputPipe, err := cmd.StdinPipe()
	if err != nil {
		return t.executeFallback(ctx, query, numResults)
	}
	inputPipe.Write(inputJSON)
	inputPipe.Close()

	outputBytes, err := cmd.Output()
	if err != nil {
		// Fallback to curl if Scrapling fails
		return t.executeFallback(ctx, query, numResults)
	}

	// Parse the JSON response
	var result struct {
		Success  bool     `json:"success"`
		Output   string   `json:"output"`
		Error    string   `json:"error"`
	}

	if err := json.Unmarshal(outputBytes, &result); err != nil {
		return t.executeFallback(ctx, query, numResults)
	}

	if !result.Success {
		return t.executeFallback(ctx, query, numResults)
	}

	return &ToolResult{
		Success: true,
		Output:  result.Output,
		Data:    map[string]interface{}{"query": query, "num_results": numResults},
	}, nil
}

func (t *WebSearchTool) executeFallback(ctx context.Context, query string, numResults int) (*ToolResult, error) {
	// Fallback to curl/sed if Scrapling is not available
	command := fmt.Sprintf(
		`curl -s -A "Mozilla/5.0" --data-urlencode %s https://lite.duckduckgo.com/lite/ | sed -n 's/.*href="\([^"]*\)" class=.result-link.>\([^<]*\)<\/a>.*/URL: \1\nTITLE: \2\n/p' | head -n %d`,
		shellescape("q="+query),
		numResults*3,
	)

	bashTool := NewBashTool()
	result, err := bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
		"timeout": 15,
	})

	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("search failed: %v", err)}, err
	}

	if strings.TrimSpace(result.Output) == "" {
		return &ToolResult{Success: false, Error: "No results found. Try a different query."}, nil
	}

	output := fmt.Sprintf("Search results for: %s\n\n%s", query, result.Output)
	return &ToolResult{
		Success: true,
		Output:  output,
		Data:    map[string]interface{}{"query": query, "num_results": numResults},
	}, nil
}

// WebReaderTool extracts and parses content from web pages
type WebReaderTool struct {
	BaseTool
}

func NewWebReaderTool() *WebReaderTool {
	return &WebReaderTool{
		BaseTool: BaseTool{
			name:        "web_reader",
			description: "Extract and parse clean text content from web pages. Similar to web_fetch but with more extraction options like link extraction. Use when you need to get readable content from a URL.",
			category:    CategoryWeb,
			requiresPermission: true,
			defaultPermission: PermissionSession,
		},
	}
}

func (t *WebReaderTool) OutputFormat() OutputFormat {
	return FormatText
}

func (t *WebReaderTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "URL to extract content from",
			},
			"extract_links": map[string]interface{}{
				"type":        "boolean",
				"description": "Also extract links from the page (default: false)",
			},
		},
		"required": []string{"url"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *WebReaderTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	url, ok := params["url"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "url is required"}, fmt.Errorf("url required")
	}

	extractLinks := false
	if el, ok := params["extract_links"].(bool); ok {
		extractLinks = el
	}

	// Try Scrapling first, fall back to curl if not available
	pythonScript := fmt.Sprintf(`
import sys
sys.path.insert(0, '%s/plugins/python/scrapling_fetch')
from scrapling.fetchers import FetcherSession

url = %q
extract_links = %v

with FetcherSession(impersonate='chrome') as session:
    page = session.get(url, timeout=30)

    output = f"Content from: {url}\\n\\n"

    # Get main text content
    body = page.css('body')
    if body:
        text = body[0].text(separator='\\n', strip=True)
        lines = [l for l in text.split('\\n') if l.strip()]
        output += '\\n'.join(lines[:200])

    if extract_links:
        output += "\\n\\n=== LINKS ===\\n"
        links = page.css('a')
        seen = set()
        for link in links[:30]:
            href = link.attrib.get('href', '')
            if href and href.startswith('http') and href not in seen:
                seen.add(href)
                output += f"{href}\\n"

    print(output[:15000])
`, os.Getenv("HOME"), url, extractLinks)

	cmd := exec.CommandContext(ctx, "python3", "-c", pythonScript)
	outputBytes, err := cmd.Output()

	if err != nil {
		// Fallback to curl
		return t.executeFallback(ctx, url, extractLinks)
	}

	output := string(outputBytes)
	if strings.TrimSpace(output) == "" {
		return t.executeFallback(ctx, url, extractLinks)
	}

	return &ToolResult{
		Success: true,
		Output:  output,
		Data:    map[string]interface{}{"url": url},
	}, nil
}

func (t *WebReaderTool) executeFallback(ctx context.Context, url string, extractLinks bool) (*ToolResult, error) {
	bashTool := NewBashTool()

	command := fmt.Sprintf(`curl -sL -A "Mozilla/5.0" %s | sed 's/<[^>]*>//g' | grep -v '^$' | head -n 200`, shellescape(url))

	if extractLinks {
		command = fmt.Sprintf(`_tmpf=$(mktemp) && curl -sL -A "Mozilla/5.0" %s > "$_tmpf" && echo "=== CONTENT ===" && sed 's/<[^>]*>//g' "$_tmpf" | grep -v '^$' | head -n 100 && echo "=== LINKS ===" && grep -oP 'href="\K[^"]+' "$_tmpf" | head -n 20; rm -f "$_tmpf"`, shellescape(url))
	}

	result, err := bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
		"timeout": 20,
	})

	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("failed to read webpage: %v", err),
		}, err
	}

	output := fmt.Sprintf("Content from: %s\n\n%s", url, result.Output)
	return &ToolResult{
		Success: true,
		Output:  output,
		Data:    map[string]interface{}{"url": url},
	}, nil
}

// Office Document Tools

// DOCXTool handles Word documents
type DOCXTool struct {
	BaseTool
}

func NewDOCXTool() *DOCXTool {
	return &DOCXTool{
		BaseTool: BaseTool{
			name:              "docx",
			description:       "Create or edit Word documents (.docx files)",
			category:          CategoryFile,
			requiresPermission: true,
			defaultPermission: PermissionOnce,
		},
	}
}

func (t *DOCXTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Action: create, read, convert_to_text",
				"enum":        []string{"create", "read", "convert_to_text"},
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the .docx file",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Content for document creation",
			},
		},
		"required": []string{"action", "path"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *DOCXTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	action, ok := params["action"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "action is required"}, fmt.Errorf("action required")
	}

	path, ok := params["path"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "path is required"}, fmt.Errorf("path required")
	}

	bashTool := NewBashTool()

	switch action {
	case "read", "convert_to_text":
		// Use pandoc or unzip to extract text
		command := fmt.Sprintf("command -v pandoc >/dev/null && pandoc -f docx -t plain %s || (unzip -p %s word/document.xml | sed 's/<[^>]*>//g')", shellescape(path), shellescape(path))
		return bashTool.Execute(ctx, map[string]interface{}{"command": command})

	case "create":
		content, ok := params["content"].(string)
		if !ok {
			return &ToolResult{Success: false, Error: "content is required for create"}, fmt.Errorf("content required")
		}

		// Use pandoc to create docx if available, otherwise create simple text file with .docx extension
		command := fmt.Sprintf("command -v pandoc >/dev/null && (echo %s | pandoc -f markdown -t docx -o %s) || (echo %s > %s.txt && echo 'Warning: pandoc not available, created text file instead')", shellescape(content), shellescape(path), shellescape(content), shellescape(path))
		return bashTool.Execute(ctx, map[string]interface{}{"command": command})

	default:
		return &ToolResult{Success: false, Error: "invalid action"}, fmt.Errorf("invalid action")
	}
}

// Similar tools for XLSX, PDF, PPTX

// XLSXTool handles Excel spreadsheets
type XLSXTool struct {
	BaseTool
}

func NewXLSXTool() *XLSXTool {
	return &XLSXTool{
		BaseTool: BaseTool{
			name:              "xlsx",
			description:       "Create or read Excel spreadsheets (.xlsx files) - basic CSV conversion",
			category:          CategoryFile,
			requiresPermission: true,
			defaultPermission: PermissionOnce,
		},
	}
}

func (t *XLSXTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Action: create_csv, read_csv",
				"enum":        []string{"create_csv", "read_csv"},
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the CSV file (use .csv extension)",
			},
			"data": map[string]interface{}{
				"type":        "string",
				"description": "CSV data for creation",
			},
		},
		"required": []string{"action", "path"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *XLSXTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	action, ok := params["action"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "action is required"}, fmt.Errorf("action required")
	}

	path, ok := params["path"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "path is required"}, fmt.Errorf("path required")
	}

	bashTool := NewBashTool()

	switch action {
	case "read_csv":
		return bashTool.Execute(ctx, map[string]interface{}{
			"command": fmt.Sprintf("cat %s", shellescape(path)),
		})

	case "create_csv":
		data, ok := params["data"].(string)
		if !ok {
			return &ToolResult{Success: false, Error: "data is required for create"}, fmt.Errorf("data required")
		}

		return bashTool.Execute(ctx, map[string]interface{}{
			"command": fmt.Sprintf("cat <<'XLSX_EOF' > %s\n%s\nXLSX_EOF", shellescape(path), data),
		})

	default:
		return &ToolResult{Success: false, Error: "invalid action"}, fmt.Errorf("invalid action")
	}
}

// PDFTool handles PDF documents
type PDFTool struct {
	BaseTool
}

func NewPDFTool() *PDFTool {
	return &PDFTool{
		BaseTool: BaseTool{
			name:              "pdf",
			description:       "Create or read PDF documents (requires pdftotext/pandoc for reading)",
			category:          CategoryFile,
			requiresPermission: true,
			defaultPermission: PermissionOnce,
		},
	}
}

func (t *PDFTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Action: read, create",
				"enum":        []string{"read", "create"},
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the PDF file",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Markdown content to convert to PDF (for create action)",
			},
		},
		"required": []string{"action", "path"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *PDFTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	action, ok := params["action"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "action is required"}, fmt.Errorf("action required")
	}

	path, ok := params["path"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "path is required"}, fmt.Errorf("path required")
	}

	bashTool := NewBashTool()

	switch action {
	case "read":
		command := fmt.Sprintf("command -v pdftotext >/dev/null && pdftotext %s - || echo 'Error: pdftotext not installed'", shellescape(path))
		return bashTool.Execute(ctx, map[string]interface{}{"command": command})

	case "create":
		content, ok := params["content"].(string)
		if !ok {
			return &ToolResult{Success: false, Error: "content is required for create"}, fmt.Errorf("content required")
		}

		command := fmt.Sprintf("command -v pandoc >/dev/null && (echo %s | pandoc -f markdown -t pdf -o %s) || echo 'Error: pandoc not installed'", shellescape(content), shellescape(path))
		return bashTool.Execute(ctx, map[string]interface{}{"command": command})

	default:
		return &ToolResult{Success: false, Error: "invalid action"}, fmt.Errorf("invalid action")
	}
}

// PPTXTool handles PowerPoint presentations
type PPTXTool struct {
	BaseTool
}

func NewPPTXTool() *PPTXTool {
	return &PPTXTool{
		BaseTool: BaseTool{
			name:              "pptx",
			description:       "Create or read PowerPoint presentations (.pptx files, requires pandoc)",
			category:          CategoryFile,
			requiresPermission: true,
			defaultPermission: PermissionOnce,
		},
	}
}

func (t *PPTXTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Action: create, read",
				"enum":        []string{"create", "read"},
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the .pptx file",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Markdown content for slides (for create action)",
			},
		},
		"required": []string{"action", "path"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *PPTXTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	action, ok := params["action"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "action is required"}, fmt.Errorf("action required")
	}

	path, ok := params["path"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "path is required"}, fmt.Errorf("path required")
	}

	bashTool := NewBashTool()

	switch action {
	case "read":
		command := fmt.Sprintf("unzip -p %s ppt/slides/*.xml | sed 's/<[^>]*>//g' | grep -v '^$' || echo 'Error: Could not extract slides'", shellescape(path))
		return bashTool.Execute(ctx, map[string]interface{}{"command": command})

	case "create":
		content, ok := params["content"].(string)
		if !ok {
			return &ToolResult{Success: false, Error: "content is required for create"}, fmt.Errorf("content required")
		}

		command := fmt.Sprintf("command -v pandoc >/dev/null && (echo %s | pandoc -f markdown -t pptx -o %s) || echo 'Error: pandoc not installed'", shellescape(content), shellescape(path))
		return bashTool.Execute(ctx, map[string]interface{}{"command": command})

	default:
		return &ToolResult{Success: false, Error: "invalid action"}, fmt.Errorf("invalid action")
	}
}

// FrontendDesignTool assists with UI design and frontend code generation
type FrontendDesignTool struct {
	BaseTool
}

func NewFrontendDesignTool() *FrontendDesignTool {
	return &FrontendDesignTool{
		BaseTool: BaseTool{
			name:              "frontend_design",
			description:       "Generate UI designs and frontend code using AI assistance",
			category:          CategoryMeta,
			requiresPermission: false,
			defaultPermission: PermissionAlways,
		},
	}
}

func (t *FrontendDesignTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task": map[string]interface{}{
				"type":        "string",
				"description": "Design task description (e.g., 'create a landing page', 'design a button component')",
			},
			"style": map[string]interface{}{
				"type":        "string",
				"description": "Design style (e.g., 'modern', 'minimal', 'colorful', 'dark mode')",
			},
			"framework": map[string]interface{}{
				"type":        "string",
				"description": "Frontend framework (e.g., 'React', 'Vue', 'plain HTML/CSS')",
			},
		},
		"required": []string{"task"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *FrontendDesignTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	task, ok := params["task"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "task is required"}, fmt.Errorf("task required")
	}

	style := "modern and clean"
	if s, ok := params["style"].(string); ok {
		style = s
	}

	framework := "React"
	if f, ok := params["framework"].(string); ok {
		framework = f
	}

	// Get AI provider from registry
	registry, ok := ctx.Value(registryContextKey).(*Registry)
	if !ok || registry == nil {
		return &ToolResult{Success: false, Error: "registry not available"}, fmt.Errorf("registry not available")
	}

	aiProvider, ok := registry.GetAIProvider().(ai.AIProvider)
	if !ok || aiProvider == nil {
		return &ToolResult{Success: false, Error: "AI provider not configured"}, fmt.Errorf("AI provider not configured")
	}

	// Create specialized prompt for frontend design
	designPrompt := fmt.Sprintf(`You are a frontend design expert. Create a %s design for the following task:

Task: %s
Framework: %s

Please provide:
1. A brief design description
2. Complete, production-ready code
3. Styling approach and color scheme
4. Any additional considerations

Generate clean, modern, accessible code that follows best practices.`, style, task, framework)

	response, err := aiProvider.Generate(ctx, designPrompt)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("design generation failed: %v", err)}, err
	}

	return &ToolResult{
		Success: true,
		Output:  response,
		Data: map[string]interface{}{
			"task":      task,
			"style":     style,
			"framework": framework,
		},
	}, nil
}
