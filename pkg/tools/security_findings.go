package tools

// security_findings.go — report_finding tool for red team agents.
//
// Agents use this to write findings into the shared SecurityContext
// so that subsequent agents (and the final reporter) see them.

import (
	"context"
	"encoding/json"
	"fmt"
)

// FindingRecorder is the interface the report_finding tool needs from SecurityContext.
// *subagents.SecurityContext satisfies this interface automatically (Go duck typing).
type FindingRecorder interface {
	AddFinding(findType, severity, detail, foundBy string) string
}

// securityContextKeyType is an unexported key type for context injection.
type securityContextKeyType struct{}

// SecurityContextKey is the context key used to inject a FindingRecorder.
// The orchestrator injects *subagents.SecurityContext under this key.
var SecurityContextKey = securityContextKeyType{}

// SecurityContextInjectorKey is kept for backwards compatibility.
var SecurityContextInjectorKey = SecurityContextKey

// ReportFindingTool lets agents record security vulnerabilities into the shared context.
type ReportFindingTool struct {
	BaseTool
}

func NewReportFindingTool() *ReportFindingTool {
	return &ReportFindingTool{
		BaseTool: BaseTool{
			name:               "report_finding",
			description:        "Record a security vulnerability finding into the shared assessment context. Use during red team engagements to capture SQLi, XSS, IDOR, RCE, SSRF, AuthnBypass, and other vulnerability types.",
			category:           CategorySecurity,
			requiresPermission: false,
			defaultPermission:  PermissionAlways,
		},
	}
}

func (t *ReportFindingTool) OutputFormat() OutputFormat { return FormatText }

func (t *ReportFindingTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"type": {
				"type": "string",
				"description": "Vulnerability type: SQLi, XSS, IDOR, SSRF, RCE, AuthnBypass, InfoDisc, Misconfiguration, etc."
			},
			"severity": {
				"type": "string",
				"enum": ["Critical", "High", "Medium", "Low", "Info"],
				"description": "CVSS-aligned severity level"
			},
			"detail": {
				"type": "string",
				"description": "Full technical description: what was found, how to reproduce, evidence (request/response snippets), impact"
			},
			"found_by": {
				"type": "string",
				"description": "Agent or tester name that discovered this finding"
			}
		},
		"required": ["type", "severity", "detail"]
	}`)
}

func (t *ReportFindingTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	findType, _ := params["type"].(string)
	if findType == "" {
		return &ToolResult{Success: false, Error: "type is required"}, fmt.Errorf("type required")
	}
	severity, _ := params["severity"].(string)
	if severity == "" {
		severity = "Medium"
	}
	detail, _ := params["detail"].(string)
	if detail == "" {
		return &ToolResult{Success: false, Error: "detail is required"}, fmt.Errorf("detail required")
	}
	foundBy, _ := params["found_by"].(string)
	if foundBy == "" {
		foundBy = "agent"
	}

	// Retrieve the FindingRecorder from context.
	recorder, _ := ctx.Value(SecurityContextKey).(FindingRecorder)
	if recorder == nil {
		// Not in a security session — acknowledge without persisting.
		preview := detail
		if len(preview) > 80 {
			preview = preview[:80]
		}
		return &ToolResult{
			Success: true,
			Output:  fmt.Sprintf("[%s][%s] %s — (no security context active; finding not persisted)", severity, findType, preview),
		}, nil
	}

	id := recorder.AddFinding(findType, severity, detail, foundBy)
	return &ToolResult{
		Success: true,
		Output:  fmt.Sprintf("Finding recorded: %s [%s][%s] by %s", id, severity, findType, foundBy),
	}, nil
}
