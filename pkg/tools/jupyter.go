package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// --- Jupyter notebook JSON structures ---

type nbformat struct {
	NBFormatMinor int        `json:"nbformat_minor"`
	NBFormat      int        `json:"nbformat"`
	Metadata      nbMetadata `json:"metadata"`
	Cells         []nbCell   `json:"cells"`
}

type nbMetadata struct {
	KernelInfo nbKernelInfo `json:"kernelspec"`
	Language   nbLangInfo   `json:"language_info"`
}

type nbKernelInfo struct {
	DisplayName string `json:"display_name"`
	Language    string `json:"language"`
	Name        string `json:"name"`
}

type nbLangInfo struct {
	Name string `json:"name"`
}

type nbCell struct {
	CellType       string          `json:"cell_type"`
	Source         []string        `json:"source"`
	Metadata       json.RawMessage `json:"metadata"`
	Outputs        []nbOutput      `json:"outputs,omitempty"`
	ExecutionCount *int            `json:"execution_count,omitempty"`
}

type nbOutput struct {
	OutputType string                 `json:"output_type"`
	Text       []string               `json:"text,omitempty"`
	Data       map[string]interface{} `json:"data,omitempty"`
	EName      string                 `json:"ename,omitempty"`
	EValue     string                 `json:"evalue,omitempty"`
	Traceback  []string               `json:"traceback,omitempty"`
}

func newNotebook(language string) nbformat {
	lang := language
	if lang == "" {
		lang = "python"
	}
	displayName := strings.ToTitle(lang[:1]) + lang[1:]
	return nbformat{
		NBFormat:      4,
		NBFormatMinor: 5,
		Metadata: nbMetadata{
			KernelInfo: nbKernelInfo{DisplayName: displayName, Language: lang, Name: lang},
			Language:   nbLangInfo{Name: lang},
		},
		Cells: []nbCell{},
	}
}

// JupyterTool provides operations on Jupyter .ipynb notebooks
type JupyterTool struct {
	BaseTool
}

// NewJupyterTool creates a new JupyterTool instance.
func NewJupyterTool() *JupyterTool {
	return &JupyterTool{
		BaseTool: NewBaseTool(
			"jupyter_notebook",
			"Create, read, append cells to, or execute Jupyter notebooks (.ipynb). Actions: create, read, append, execute, list_outputs.",
			CategoryShell,
			true,
			PermissionSession,
		),
	}
}

// Parameters returns the JSON schema for the tool's parameters.
func (t *JupyterTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {
				"type": "string",
				"enum": ["create","read","append","execute","list_outputs"],
				"description": "Operation to perform"
			},
			"path": {"type": "string", "description": "Path to the .ipynb file"},
			"cell_type": {"type": "string", "enum": ["code","markdown"], "description": "Cell type for append (default: code)"},
			"source": {"type": "string", "description": "Cell source code/text for append"},
			"language": {"type": "string", "description": "Kernel language for create (default: python)"},
			"timeout": {"type": "number", "description": "Execution timeout in seconds (default 120)"}
		},
		"required": ["action","path"]
	}`)
}

// Execute runs the jupyter_notebook tool.
func (t *JupyterTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	action, _ := params["action"].(string)
	path, _ := params["path"].(string)
	if action == "" || path == "" {
		return &ToolResult{Success: false, Error: "action and path are required"}, nil
	}
	if !strings.HasSuffix(path, ".ipynb") {
		path += ".ipynb"
	}

	switch action {
	case "create":
		lang, _ := params["language"].(string)
		nb := newNotebook(lang)
		data, err := json.MarshalIndent(nb, "", " ")
		if err != nil {
			return &ToolResult{Success: false, Error: err.Error()}, nil
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return &ToolResult{Success: false, Error: err.Error()}, nil
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			return &ToolResult{Success: false, Error: err.Error()}, nil
		}
		return &ToolResult{Success: true, Output: fmt.Sprintf("Created notebook: %s", path)}, nil

	case "read":
		data, err := os.ReadFile(path)
		if err != nil {
			return &ToolResult{Success: false, Error: err.Error()}, nil
		}
		var nb nbformat
		if err := json.Unmarshal(data, &nb); err != nil {
			return &ToolResult{Success: false, Error: "invalid notebook: " + err.Error()}, nil
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("# Notebook: %s (%d cells)\n\n", filepath.Base(path), len(nb.Cells)))
		for i, cell := range nb.Cells {
			src := strings.Join(cell.Source, "")
			sb.WriteString(fmt.Sprintf("## Cell %d [%s]\n```%s\n%s\n```\n", i+1, cell.CellType, nb.Metadata.Language.Name, src))
			for _, out := range cell.Outputs {
				if len(out.Text) > 0 {
					sb.WriteString("**Output:**\n```\n" + strings.Join(out.Text, "") + "\n```\n")
				} else if out.EName != "" {
					sb.WriteString(fmt.Sprintf("**Error:** %s: %s\n", out.EName, out.EValue))
				}
			}
			sb.WriteString("\n")
		}
		return &ToolResult{Success: true, Output: sb.String(), OutputFormat: FormatText}, nil

	case "append":
		cellType, _ := params["cell_type"].(string)
		source, _ := params["source"].(string)
		if cellType == "" {
			cellType = "code"
		}
		if source == "" {
			return &ToolResult{Success: false, Error: "source is required for append"}, nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return &ToolResult{Success: false, Error: err.Error()}, nil
		}
		var nb nbformat
		if err := json.Unmarshal(data, &nb); err != nil {
			return &ToolResult{Success: false, Error: "invalid notebook: " + err.Error()}, nil
		}
		cell := nbCell{
			CellType: cellType,
			Source:   []string{source},
			Metadata: json.RawMessage(`{}`),
		}
		if cellType == "code" {
			cell.Outputs = []nbOutput{}
		}
		nb.Cells = append(nb.Cells, cell)
		out, err := json.MarshalIndent(nb, "", " ")
		if err != nil {
			return &ToolResult{Success: false, Error: err.Error()}, nil
		}
		if err := os.WriteFile(path, out, 0644); err != nil {
			return &ToolResult{Success: false, Error: err.Error()}, nil
		}
		return &ToolResult{Success: true, Output: fmt.Sprintf("Appended %s cell. Notebook now has %d cells.", cellType, len(nb.Cells))}, nil

	case "execute":
		timeout := 120
		if v, ok := params["timeout"].(float64); ok && v > 0 {
			timeout = int(v)
		}
		// Check for nbconvert
		if _, err := exec.LookPath("jupyter"); err != nil {
			return &ToolResult{Success: false, Error: "jupyter not found on PATH. Install with: pip install jupyter"}, nil
		}
		execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
		cmd := exec.CommandContext(execCtx, "jupyter", "nbconvert", "--to", "notebook",
			"--execute", "--inplace", path)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return &ToolResult{
				Success: false,
				Error:   fmt.Sprintf("execution failed: %v\n%s", err, string(out)),
			}, nil
		}
		return &ToolResult{Success: true, Output: fmt.Sprintf("Executed notebook in-place: %s\n%s", path, string(out))}, nil

	case "list_outputs":
		data, err := os.ReadFile(path)
		if err != nil {
			return &ToolResult{Success: false, Error: err.Error()}, nil
		}
		var nb nbformat
		if err := json.Unmarshal(data, &nb); err != nil {
			return &ToolResult{Success: false, Error: "invalid notebook: " + err.Error()}, nil
		}
		var sb strings.Builder
		for i, cell := range nb.Cells {
			if len(cell.Outputs) == 0 {
				continue
			}
			sb.WriteString(fmt.Sprintf("**Cell %d outputs:**\n", i+1))
			for _, out := range cell.Outputs {
				if len(out.Text) > 0 {
					sb.WriteString("```\n" + strings.Join(out.Text, "") + "\n```\n")
				} else if out.EName != "" {
					sb.WriteString(fmt.Sprintf("Error: %s: %s\n", out.EName, out.EValue))
				}
			}
		}
		if sb.Len() == 0 {
			return &ToolResult{Success: true, Output: "No outputs found. Run execute first."}, nil
		}
		return &ToolResult{Success: true, Output: sb.String(), OutputFormat: FormatText}, nil
	}

	return &ToolResult{Success: false, Error: "unknown action: " + action}, nil
}
