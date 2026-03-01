package tools

// code2world.go — Code2World Simulation Tool
//
// Implements the SENSE "Body" (Action Sandbox) concept.  Before executing a
// complex GUI or system action, Code2World generates a renderable HTML preview
// that lets the agent (and optionally the user) verify the expected outcome
// without side-effects.
//
// Usage by the AI agent:
//
//   {
//     "tool": "code2world",
//     "parameters": {
//       "action_description": "Create a directory called 'reports' under /home/user",
//       "action_type": "filesystem",
//       "expected_output": "A new directory at /home/user/reports"
//     }
//   }
//
// The tool returns the path to the generated HTML preview file and a plain-text
// summary.  The TUI can render the path as a clickable link.

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Code2WorldTool generates HTML previews of planned system actions.
type Code2WorldTool struct {
	BaseTool
}

// NewCode2WorldTool constructs the Code2WorldTool.
func NewCode2WorldTool() *Code2WorldTool {
	return &Code2WorldTool{
		BaseTool: BaseTool{
			name:               "code2world",
			description:        "Generate a sandboxed HTML preview of a planned system action before executing it. Use this before any destructive or complex operation to visualise the expected outcome.",
			category:           CategoryMeta,
			requiresPermission: false,
			defaultPermission:  PermissionAlways,
		},
	}
}

func (t *Code2WorldTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action_description": {
				"type": "string",
				"description": "A natural-language description of the action to be previewed (e.g. 'Delete the file /tmp/old_log.txt')."
			},
			"action_type": {
				"type": "string",
				"description": "Category of the action.",
				"enum": ["filesystem","shell","network","database","git","ui","other"]
			},
			"expected_output": {
				"type": "string",
				"description": "What the action is expected to produce or change."
			},
			"code_snippet": {
				"type": "string",
				"description": "Optional: the exact command or code that will be executed."
			},
			"risks": {
				"type": "string",
				"description": "Optional: potential risks or side-effects of the action."
			}
		},
		"required": ["action_description", "action_type", "expected_output"]
	}`)
}

func (t *Code2WorldTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	desc, _ := params["action_description"].(string)
	actionType, _ := params["action_type"].(string)
	expected, _ := params["expected_output"].(string)
	snippet, _ := params["code_snippet"].(string)
	risks, _ := params["risks"].(string)

	if desc == "" {
		return &ToolResult{Success: false, Error: "action_description is required"}, nil
	}
	// Validate actionType server-side against the known enum to prevent
	// CSS class injection in the generated HTML.
	validTypes := map[string]bool{
		"filesystem": true, "shell": true, "network": true,
		"database": true, "git": true, "ui": true, "other": true,
	}
	if !validTypes[actionType] {
		actionType = "other"
	}

	// Build the HTML preview.
	htmlContent := buildCode2WorldHTML(desc, actionType, expected, snippet, risks)

	// Write to a temp file.
	tmpDir := os.TempDir()
	fname := fmt.Sprintf("gorkbot_c2w_%d.html", time.Now().UnixNano())
	fpath := filepath.Join(tmpDir, fname)
	if err := os.WriteFile(fpath, []byte(htmlContent), 0600); err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("failed to write preview file: %v", err),
		}, nil
	}

	summary := fmt.Sprintf(
		"Code2World Preview Generated\n"+
			"───────────────────────────\n"+
			"Action:   %s (%s)\n"+
			"Expected: %s\n"+
			"Preview:  %s\n",
		desc, actionType, expected, fpath,
	)
	if risks != "" {
		summary += "Risks:    " + risks + "\n"
	}
	summary += "\nReview the HTML preview before proceeding with execution."

	return &ToolResult{
		Success: true,
		Output:  summary,
	}, nil
}

// buildCode2WorldHTML produces a self-contained HTML5 preview document.
func buildCode2WorldHTML(desc, actionType, expected, snippet, risks string) string {
	iconMap := map[string]string{
		"filesystem": "📁",
		"shell":      "🖥️",
		"network":    "🌐",
		"database":   "🗄️",
		"git":        "🔀",
		"ui":         "🖼️",
		"other":      "⚙️",
	}
	icon := iconMap[actionType]
	if icon == "" {
		icon = "⚙️"
	}

	snippetSection := ""
	if snippet != "" {
		snippetSection = fmt.Sprintf(`
		<div class="section">
			<h2>📋 Command / Code</h2>
			<pre class="code">%s</pre>
		</div>`, html.EscapeString(snippet))
	}

	riskSection := ""
	if risks != "" {
		riskSection = fmt.Sprintf(`
		<div class="section risk">
			<h2>⚠️ Potential Risks</h2>
			<p>%s</p>
		</div>`, html.EscapeString(risks))
	}

	typeClass := strings.ToLower(actionType)
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Gorkbot with SENSE | Code2World Preview</title>
<style>
  :root {
    --bg: #1a1a2e; --card: #16213e; --accent: #e94560;
    --text: #eaeaea; --muted: #8892b0; --code-bg: #0a0a1a;
    --green: #64ffda; --yellow: #ffd700; --red: #ff4444;
  }
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { background: var(--bg); color: var(--text); font-family: 'Segoe UI', system-ui, sans-serif;
         padding: 20px; min-height: 100vh; }
  .header { text-align: center; padding: 30px 0 20px; border-bottom: 1px solid var(--accent); margin-bottom: 24px; }
  .header h1 { font-size: 1.5rem; color: var(--accent); letter-spacing: 2px; }
  .header .sub { font-size: 0.8rem; color: var(--muted); margin-top: 4px; }
  .badge { display: inline-block; background: var(--accent); color: #fff;
           font-size: 0.7rem; padding: 2px 10px; border-radius: 12px; margin-top: 8px; letter-spacing: 1px; }
  .card { background: var(--card); border-radius: 10px; padding: 24px; margin-bottom: 20px;
          border: 1px solid rgba(233,69,96,0.2); }
  .action-header { display: flex; align-items: flex-start; gap: 16px; }
  .icon { font-size: 2.5rem; }
  .action-title h2 { font-size: 1.2rem; color: var(--green); }
  .action-title .type { font-size: 0.75rem; color: var(--muted); text-transform: uppercase; letter-spacing: 1px; }
  .section { margin-top: 20px; padding-top: 16px; border-top: 1px solid rgba(255,255,255,0.05); }
  .section h2 { font-size: 0.95rem; color: var(--accent); margin-bottom: 10px; }
  .section p { color: var(--text); line-height: 1.6; }
  .code { background: var(--code-bg); border: 1px solid rgba(100,255,218,0.15);
          border-radius: 6px; padding: 14px; font-family: 'Fira Code', 'Consolas', monospace;
          font-size: 0.85rem; color: var(--green); overflow-x: auto; white-space: pre-wrap; }
  .risk { }
  .risk h2 { color: var(--yellow); }
  .risk p { color: var(--yellow); }
  .expected { }
  .expected h2 { color: var(--green); }
  .status-bar { display: flex; justify-content: space-between; align-items: center;
                background: rgba(233,69,96,0.1); border-radius: 6px; padding: 10px 16px;
                margin-top: 24px; font-size: 0.8rem; color: var(--muted); }
  .status-badge { color: var(--red); font-weight: bold; }
  .type-%s .badge { background: #3d5a80; }
</style>
</head>
<body class="type-%s">

<div class="header">
  <h1>⚡ Gorkbot with SENSE</h1>
  <div class="sub">Code2World Action Preview</div>
  <span class="badge">v1.5.3 VALIDATION REQUIRED</span>
</div>

<div class="card">
  <div class="action-header">
    <div class="icon">%s</div>
    <div class="action-title">
      <h2>%s</h2>
      <div class="type">%s action</div>
    </div>
  </div>

  <div class="section expected">
    <h2>✅ Expected Outcome</h2>
    <p>%s</p>
  </div>%s%s
</div>

<div class="status-bar">
  <span>Generated: %s</span>
  <span class="status-badge">⚠️ PREVIEW ONLY — NOT EXECUTED</span>
  <span>Gorkbot-SENSE v1.5.3 · Velarium AI</span>
</div>

</body>
</html>`,
		typeClass, typeClass,
		icon,
		html.EscapeString(desc),
		actionType,
		html.EscapeString(expected),
		snippetSection,
		riskSection,
		timestamp,
	)
}
