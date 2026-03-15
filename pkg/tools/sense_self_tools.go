package tools

// sense_self_tools.go — SENSE Self-Knowledge Tool Suite
//
// Exposes SENSE capabilities directly to the AI as callable tools:
//
//   sense_discovery  — machine-readable JSON document of all registered tools
//   sense_check      — run the autonomous trace analyzer
//   sense_evolve     — generate SKILL.md invariant files from failure patterns
//   sense_sanitize   — validate a proposed parameter set through the middleware
//
// These tools complement the /self slash commands.  Having them as tools lets
// the AI invoke the SENSE layer as part of multi-step agentic tasks without
// requiring a human to type a slash command first.
//
// All tools use the configDir stored in the registry's context key.
// The tool permission model is:
//   - sense_discovery  → always (read-only, safe)
//   - sense_check      → always (read-only, safe)
//   - sense_evolve     → once   (writes files when --dry-run=false)
//   - sense_sanitize   → always (pure validation, no side-effects)

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/sense"
)

// ── sense_discovery ──────────────────────────────────────────────────────────

// SenseDiscoveryTool produces the machine-readable JSON discovery document.
type SenseDiscoveryTool struct {
	BaseTool
}

// NewSenseDiscoveryTool creates the discovery tool.
func NewSenseDiscoveryTool() *SenseDiscoveryTool {
	return &SenseDiscoveryTool{
		BaseTool: NewBaseTool(
			"sense_discovery",
			"Dump a machine-readable JSON Discovery Document listing all registered "+
				"Gorkbot tools (name, description, category, parameter schema, permission "+
				"level) and all known CLI flags. Use this before constructing any tool call "+
				"to verify the tool name and required parameters.",
			CategoryMeta,
			false,
			PermissionAlways,
		),
	}
}

func (t *SenseDiscoveryTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"compact": {
				"type": "boolean",
				"description": "If true, output compact JSON instead of pretty-printed. Default false."
			},
			"category": {
				"type": "string",
				"description": "Filter tools by category (e.g. 'file', 'shell', 'meta'). Empty = all."
			}
		}
	}`)
}

func (t *SenseDiscoveryTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	reg, ok := ctx.Value(registryContextKey).(*Registry)
	if !ok || reg == nil {
		return &ToolResult{Success: false, Error: "registry not available in context"}, nil
	}

	compact, _ := params["compact"].(bool)
	categoryFilter, _ := params["category"].(string)

	// Build tool descriptors from live registry.
	toolList := reg.List()
	sort.Slice(toolList, func(i, j int) bool {
		return toolList[i].Name() < toolList[j].Name()
	})

	descs := make([]sense.ToolDescriptor, 0, len(toolList))
	catCounts := make(map[string]int)
	for _, tool := range toolList {
		cat := string(tool.Category())
		if categoryFilter != "" && !strings.EqualFold(cat, categoryFilter) {
			continue
		}
		descs = append(descs, sense.ToolDescriptor{
			Name:               tool.Name(),
			Description:        tool.Description(),
			Category:           cat,
			Parameters:         tool.Parameters(),
			RequiresPermission: tool.RequiresPermission(),
			DefaultPermission:  string(tool.DefaultPermission()),
			OutputFormat:       string(tool.OutputFormat()),
		})
		catCounts[cat]++
	}

	doc := sense.DiscoveryDoc{
		SchemaVersion:  sense.DiscoveryVersion,
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
		Application:    "Gorkbot",
		ToolCount:      len(descs),
		CategoryCounts: catCounts,
		Tools:          descs,
		Flags:          sense.KnownCLIFlags(),
		SENSEVersion:   sense.SENSEVersion,
	}

	var (
		b   []byte
		err error
	)
	if compact {
		b, err = json.Marshal(doc)
	} else {
		b, err = json.MarshalIndent(doc, "", "  ")
	}
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("marshal failed: %v", err)}, nil
	}
	return &ToolResult{
		Success:      true,
		Output:       string(b),
		OutputFormat: FormatJSON,
	}, nil
}

func (t *SenseDiscoveryTool) OutputFormat() OutputFormat { return FormatJSON }

// ── sense_check ──────────────────────────────────────────────────────────────

// SenseCheckTool runs the autonomous trace analyzer.
type SenseCheckTool struct {
	BaseTool
}

// NewSenseCheckTool creates the trace analysis tool.
func NewSenseCheckTool() *SenseCheckTool {
	return &SenseCheckTool{
		BaseTool: NewBaseTool(
			"sense_check",
			"Run the SENSE autonomous trace analyzer on the stored execution logs. "+
				"Classifies failures into Neural Hallucinations, Tool Failures, and Context "+
				"Overflows. Returns a Markdown analysis report with failure counts and "+
				"top failure patterns by tool.",
			CategoryMeta,
			false,
			PermissionAlways,
		),
	}
}

func (t *SenseCheckTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"config_dir": {
				"type": "string",
				"description": "Optional override for the config directory. Uses the default if empty."
			}
		}
	}`)
}

func (t *SenseCheckTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	configDir := resolveConfigDir(ctx, params)
	if configDir == "" {
		return &ToolResult{
			Success: false,
			Error:   "config directory not available — tool cannot locate trace files",
		}, nil
	}

	traceDir := filepath.Join(configDir, "sense", "traces")
	if _, err := os.Stat(traceDir); os.IsNotExist(err) {
		return &ToolResult{
			Success: true,
			Output: fmt.Sprintf(
				"No trace directory found at `%s`.\n\n"+
					"Traces are written automatically during tool execution.\n"+
					"Run some tool-using tasks first, then re-run sense_check.",
				traceDir,
			),
		}, nil
	}

	analyzer := sense.NewTraceAnalyzer(traceDir)
	report, err := analyzer.Analyze()
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("trace analysis failed: %v", err),
		}, nil
	}

	return &ToolResult{
		Success:      true,
		Output:       report.Summary,
		OutputFormat: FormatText,
	}, nil
}

func (t *SenseCheckTool) OutputFormat() OutputFormat { return FormatText }

// ── sense_evolve ─────────────────────────────────────────────────────────────

// SenseEvolveTool converts failure patterns into SKILL.md files.
type SenseEvolveTool struct {
	BaseTool
}

// NewSenseEvolveTool creates the evolutionary pipeline tool.
func NewSenseEvolveTool() *SenseEvolveTool {
	return &SenseEvolveTool{
		BaseTool: NewBaseTool(
			"sense_evolve",
			"Run the SENSE evolutionary pipeline: analyse trace logs, identify recurring "+
				"failure patterns, and generate SKILL.md invariant files that teach the AI "+
				"to avoid those patterns in future sessions. "+
				"Defaults to dry-run mode — set dry_run=false to write files.",
			CategoryMeta,
			true, // requires permission when dry_run=false (file writes)
			PermissionOnce,
		),
	}
}

func (t *SenseEvolveTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"dry_run": {
				"type": "boolean",
				"description": "If true (default), show what would be written without writing files."
			},
			"min_evidence": {
				"type": "integer",
				"description": "Minimum failure count to generate a SKILL file (default: 2)."
			},
			"config_dir": {
				"type": "string",
				"description": "Optional override for the config directory."
			}
		}
	}`)
}

func (t *SenseEvolveTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	// Parse dry_run (defaults to true — mandatory safety default).
	dryRun := true
	if v, ok := params["dry_run"].(bool); ok {
		dryRun = v
	}

	// Parse min_evidence.
	minEvidence := 2
	switch v := params["min_evidence"].(type) {
	case float64:
		minEvidence = int(v)
	case string:
		if n, err := strconv.Atoi(v); err == nil {
			minEvidence = n
		}
	}

	configDir := resolveConfigDir(ctx, params)
	if configDir == "" {
		return &ToolResult{
			Success: false,
			Error:   "config directory not available",
		}, nil
	}

	traceDir := filepath.Join(configDir, "sense", "traces")
	skillsDir := filepath.Join(configDir, "sense", "skills")

	// Run trace analysis.
	analyzer := sense.NewTraceAnalyzer(traceDir)
	report, err := analyzer.Analyze()
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("trace analysis failed: %v", err),
		}, nil
	}

	if len(report.FailureEvents) == 0 {
		return &ToolResult{
			Success: true,
			Output:  "No failure events found in traces — nothing to evolve.\n\n" + report.Summary,
		}, nil
	}

	// Run evolution.
	cfg := sense.DefaultEvolverConfig(skillsDir)
	cfg.DryRun = dryRun
	cfg.MinEvidence = minEvidence

	evolver := sense.NewSkillEvolver(cfg)
	result, err := evolver.Evolve(report)
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("SENSE evolution failed: %v", err),
		}, nil
	}

	return &ToolResult{
		Success:      true,
		Output:       result.Summary,
		OutputFormat: FormatText,
	}, nil
}

func (t *SenseEvolveTool) OutputFormat() OutputFormat { return FormatText }

// ── sense_sanitize ───────────────────────────────────────────────────────────

// SenseSanitizeTool validates a parameter set through the stabilization middleware.
type SenseSanitizeTool struct {
	BaseTool
}

// NewSenseSanitizeTool creates the parameter validation tool.
func NewSenseSanitizeTool() *SenseSanitizeTool {
	return &SenseSanitizeTool{
		BaseTool: NewBaseTool(
			"sense_sanitize",
			"Validate a proposed set of tool parameters through the SENSE stabilization "+
				"middleware without executing any tool. Returns OK if all parameters pass "+
				"the three invariants (control-char rejection, path sandboxing, "+
				"resource-name validation) or a detailed violation report if any fail. "+
				"Use this to pre-check parameters before calling potentially dangerous tools.",
			CategoryMeta,
			false,
			PermissionAlways,
		),
	}
}

func (t *SenseSanitizeTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"params": {
				"type": "object",
				"description": "The parameter map to validate (as a JSON object)."
			}
		},
		"required": ["params"]
	}`)
}

func (t *SenseSanitizeTool) Execute(_ context.Context, params map[string]interface{}) (*ToolResult, error) {
	rawParams, ok := params["params"].(map[string]interface{})
	if !ok {
		return &ToolResult{
			Success: false,
			Error:   "params must be a JSON object",
		}, nil
	}

	sanitizer, err := sense.NewInputSanitizer()
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("sanitizer init failed: %v", err),
		}, nil
	}

	if err := sanitizer.SanitizeParams(rawParams); err != nil {
		return &ToolResult{
			Success: true, // the TOOL succeeded; the parameter FAILED validation
			Output:  fmt.Sprintf("❌ SENSE Stabilizer: parameter rejected\n\n%s", err.Error()),
		}, nil
	}

	return &ToolResult{
		Success: true,
		Output:  "✅ SENSE Stabilizer: all parameters passed — control-char, path-sandbox, and resource-name invariants satisfied.",
	}, nil
}

func (t *SenseSanitizeTool) OutputFormat() OutputFormat { return FormatText }

// ── Context helper ────────────────────────────────────────────────────────────

// resolveConfigDir returns the config directory, preferring an explicit
// "config_dir" param, then falling back to the registry's stored configDir.
func resolveConfigDir(ctx context.Context, params map[string]interface{}) string {
	if v, ok := params["config_dir"].(string); ok && v != "" {
		return v
	}
	reg, ok := ctx.Value(registryContextKey).(*Registry)
	if !ok || reg == nil {
		return ""
	}
	return reg.GetConfigDir()
}
