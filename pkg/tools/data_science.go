package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// CsvPivotTool uses duckdb or pandas (via python) to process CSV.
type CsvPivotTool struct {
	BaseTool
}

func NewCsvPivotTool() *CsvPivotTool {
	return &CsvPivotTool{
		BaseTool: BaseTool{
			name:               "csv_pivot",
			description:        "Execute SQL on a CSV file using DuckDB.",
			category:           CategoryDatabase,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *CsvPivotTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"csv": map[string]interface{}{
				"type":        "string",
				"description": "Path to CSV file.",
			},
			"query": map[string]interface{}{
				"type":        "string",
				"description": "SQL query (use 'read_csv_auto(\"path\")' or table name if loaded).",
			},
		},
		"required": []string{"csv", "query"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *CsvPivotTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	csvPath, _ := args["csv"].(string)
	query, _ := args["query"].(string)

	fullQuery := strings.ReplaceAll(query, "{{csv}}", fmt.Sprintf("read_csv_auto('%s')", csvPath))

	cmd := exec.CommandContext(ctx, "duckdb", "-c", fullQuery)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("DuckDB failed: %v\n%s", err, string(out))}, nil
	}
	return &ToolResult{Success: true, Output: string(out)}, nil
}

// PlotGenerateTool generates charts using python matplotlib.
type PlotGenerateTool struct {
	BaseTool
}

func NewPlotGenerateTool() *PlotGenerateTool {
	return &PlotGenerateTool{
		BaseTool: BaseTool{
			name:               "plot_generate",
			description:        "Generate a chart from data using Python/Matplotlib.",
			category:           CategoryDatabase,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *PlotGenerateTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"data_json": map[string]interface{}{
				"type":        "string",
				"description": "JSON string of data (x, y lists).",
			},
			"type": map[string]interface{}{
				"type":        "string",
				"description": "Chart type (line, bar, scatter).",
			},
			"output": map[string]interface{}{
				"type":        "string",
				"description": "Output image file.",
			},
		},
		"required": []string{"data_json", "output"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *PlotGenerateTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	dataStr, _ := args["data_json"].(string)
	chartType, _ := args["type"].(string)
	output, _ := args["output"].(string)

	script := fmt.Sprintf(`
import matplotlib.pyplot as plt
import json

data = json.loads('%s')
x = data.get('x', [])
y = data.get('y', [])

if '%s' == 'bar':
    plt.bar(x, y)
elif '%s' == 'scatter':
    plt.scatter(x, y)
else:
    plt.plot(x, y)

plt.savefig('%s')
`, strings.ReplaceAll(dataStr, "'", "\\'"), chartType, chartType, output)

	cmd := exec.CommandContext(ctx, "python3", "-c", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Plotting failed: %v\n%s", err, string(out))}, nil
	}
	return &ToolResult{Success: true, Output: "Chart saved to " + output}, nil
}

// ArxivSearchTool searches arxiv.
type ArxivSearchTool struct {
	BaseTool
}

func NewArxivSearchTool() *ArxivSearchTool {
	return &ArxivSearchTool{
		BaseTool: BaseTool{
			name:               "arxiv_search",
			description:        "Search ArXiv for academic papers.",
			category:           CategoryWeb,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *ArxivSearchTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query.",
			},
			"max_results": map[string]interface{}{
				"type":        "integer",
				"description": "Max results (default 5).",
			},
		},
		"required": []string{"query"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *ArxivSearchTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	query, _ := args["query"].(string)
	max, _ := args["max_results"].(int)
	if max == 0 {
		max = 5
	}
	url := fmt.Sprintf("http://export.arxiv.org/api/query?search_query=all:%s&max_results=%d", strings.ReplaceAll(query, " ", "+"), max)

	cmd := exec.CommandContext(ctx, "curl", "-s", url)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Arxiv API failed: %v", err)}, nil
	}
	return &ToolResult{Success: true, Output: string(out)}, nil
}

// WebArchiveTool checks Wayback Machine.
type WebArchiveTool struct {
	BaseTool
}

func NewWebArchiveTool() *WebArchiveTool {
	return &WebArchiveTool{
		BaseTool: BaseTool{
			name:               "web_archive",
			description:        "Check Wayback Machine for url snapshots.",
			category:           CategoryWeb,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *WebArchiveTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "URL to check.",
			},
		},
		"required": []string{"url"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *WebArchiveTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	url, _ := args["url"].(string)
	api := fmt.Sprintf("http://archive.org/wayback/available?url=%s", url)

	cmd := exec.CommandContext(ctx, "curl", "-s", api)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &ToolResult{Success: false, Error: "Archive check failed."}, nil
	}
	return &ToolResult{Success: true, Output: string(out)}, nil
}

// WhoisLookupTool runs whois.
type WhoisLookupTool struct {
	BaseTool
}

func NewWhoisLookupTool() *WhoisLookupTool {
	return &WhoisLookupTool{
		BaseTool: BaseTool{
			name:               "whois_lookup",
			description:        "Perform WHOIS lookup on a domain.",
			category:           CategoryNetwork,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *WhoisLookupTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"domain": map[string]interface{}{
				"type":        "string",
				"description": "Domain name.",
			},
		},
		"required": []string{"domain"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *WhoisLookupTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	domain, _ := args["domain"].(string)
	cmd := exec.CommandContext(ctx, "whois", domain)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &ToolResult{Success: false, Error: "Whois failed (install whois package)."}, nil
	}
	return &ToolResult{Success: true, Output: string(out)}, nil
}
