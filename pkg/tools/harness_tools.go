package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/harness"
)

// ── HarnessInitTool ──────────────────────────────────────────────────────────

type HarnessInitTool struct {
	BaseTool
}

func NewHarnessInitTool() *HarnessInitTool {
	return &HarnessInitTool{
		BaseTool: NewBaseTool(
			"harness_init",
			"Initialize a multi-session project harness with a goal and feature list. Creates .gorkbot/harness/ in the project root.",
			CategoryMeta,
			true,
			PermissionOnce,
		),
	}
}

func (t *HarnessInitTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"goal": {
				"type": "string",
				"description": "The overall project goal"
			},
			"features": {
				"type": "string",
				"description": "JSON array of features: [{\"title\": \"...\", \"description\": \"...\", \"dependencies\": [...], \"priority\": N}]"
			},
			"project_root": {
				"type": "string",
				"description": "Project root directory (defaults to CWD)"
			}
		},
		"required": ["goal", "features"]
	}`)
}

func (t *HarnessInitTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	goal, _ := params["goal"].(string)
	featuresJSON, _ := params["features"].(string)
	projectRoot, _ := params["project_root"].(string)

	if goal == "" {
		return &ToolResult{Success: false, Error: "goal is required"}, nil
	}
	if featuresJSON == "" {
		return &ToolResult{Success: false, Error: "features JSON array is required"}, nil
	}

	if projectRoot == "" {
		var err error
		projectRoot, err = os.Getwd()
		if err != nil {
			return &ToolResult{Success: false, Error: fmt.Sprintf("cannot determine CWD: %v", err)}, nil
		}
	}

	var features []harness.FeatureInput
	if err := json.Unmarshal([]byte(featuresJSON), &features); err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("invalid features JSON: %v", err)}, nil
	}

	store := harness.NewStore(projectRoot)
	init := harness.NewInitializer(store, nil)

	if init.IsInitialized() {
		return &ToolResult{Success: false, Error: "harness already initialized — use harness_boot to resume"}, nil
	}

	fl, err := init.Initialize(ctx, goal, features)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("initialization failed: %v", err)}, nil
	}

	data, _ := json.MarshalIndent(fl, "", "  ")
	return &ToolResult{
		Success:      true,
		Output:       fmt.Sprintf("Project harness initialized with %d features.\n\n%s", len(fl.Features), string(data)),
		OutputFormat: FormatJSON,
	}, nil
}

// ── HarnessBootTool ──────────────────────────────────────────────────────────

type HarnessBootTool struct {
	BaseTool
}

func NewHarnessBootTool() *HarnessBootTool {
	return &HarnessBootTool{
		BaseTool: NewBaseTool(
			"harness_boot",
			"Boot into a project — reads feature list, recent commits, progress log, and suggests next feature. Use this at the start of every session.",
			CategoryMeta,
			false,
			PermissionAlways,
		),
	}
}

func (t *HarnessBootTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"project_root": {
				"type": "string",
				"description": "Project root directory (defaults to CWD)"
			}
		}
	}`)
}

func (t *HarnessBootTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	projectRoot := resolveProjectRoot(params)

	store := harness.NewStore(projectRoot)
	verifier := harness.NewVerifier(store, nil)
	sessionID := fmt.Sprintf("session-%d", time.Now().UnixNano())
	worker := harness.NewWorker(store, verifier, sessionID, nil)

	report, err := worker.Boot(ctx)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("boot failed: %v", err)}, nil
	}

	data, _ := json.MarshalIndent(report, "", "  ")
	return &ToolResult{
		Success:      true,
		Output:       string(data),
		OutputFormat: FormatJSON,
	}, nil
}

// ── HarnessSelectTool ────────────────────────────────────────────────────────

type HarnessSelectTool struct {
	BaseTool
}

func NewHarnessSelectTool() *HarnessSelectTool {
	return &HarnessSelectTool{
		BaseTool: NewBaseTool(
			"harness_select",
			"Select the next feature to work on based on priority and dependency resolution. Automatically starts the selected feature.",
			CategoryMeta,
			false,
			PermissionAlways,
		),
	}
}

func (t *HarnessSelectTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"project_root": {
				"type": "string",
				"description": "Project root directory (defaults to CWD)"
			}
		}
	}`)
}

func (t *HarnessSelectTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	projectRoot := resolveProjectRoot(params)

	store := harness.NewStore(projectRoot)
	verifier := harness.NewVerifier(store, nil)
	sessionID := fmt.Sprintf("session-%d", time.Now().UnixNano())
	worker := harness.NewWorker(store, verifier, sessionID, nil)

	feature, err := worker.SelectFeature()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("select failed: %v", err)}, nil
	}

	if err := worker.StartFeature(feature.ID); err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("start failed: %v", err)}, nil
	}

	data, _ := json.MarshalIndent(feature, "", "  ")
	return &ToolResult{
		Success:      true,
		Output:       fmt.Sprintf("Selected and started feature: %s\n\n%s", feature.Title, string(data)),
		OutputFormat: FormatJSON,
	}, nil
}

// ── HarnessCompleteTool ──────────────────────────────────────────────────────

type HarnessCompleteTool struct {
	BaseTool
}

func NewHarnessCompleteTool() *HarnessCompleteTool {
	return &HarnessCompleteTool{
		BaseTool: NewBaseTool(
			"harness_complete",
			"Complete a feature — runs verification (build/test), commits if passing, updates status. Cannot mark 'passing' without passing verification.",
			CategoryMeta,
			true,
			PermissionOnce,
		),
	}
}

func (t *HarnessCompleteTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"feature_id": {
				"type": "string",
				"description": "Feature ID (e.g. feat-001)"
			},
			"commit_message": {
				"type": "string",
				"description": "Git commit message for this feature"
			},
			"project_root": {
				"type": "string",
				"description": "Project root directory (defaults to CWD)"
			}
		},
		"required": ["feature_id", "commit_message"]
	}`)
}

func (t *HarnessCompleteTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	featureID, _ := params["feature_id"].(string)
	commitMsg, _ := params["commit_message"].(string)
	projectRoot := resolveProjectRoot(params)

	if featureID == "" || commitMsg == "" {
		return &ToolResult{Success: false, Error: "feature_id and commit_message are required"}, nil
	}

	store := harness.NewStore(projectRoot)
	verifier := harness.NewVerifier(store, nil)
	sessionID := fmt.Sprintf("session-%d", time.Now().UnixNano())
	worker := harness.NewWorker(store, verifier, sessionID, nil)

	report, err := worker.CompleteFeature(ctx, featureID, commitMsg)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("complete failed: %v", err)}, nil
	}

	data, _ := json.MarshalIndent(report, "", "  ")
	status := "PASSED"
	if !report.Passed {
		status = "FAILED"
	}
	return &ToolResult{
		Success:      true,
		Output:       fmt.Sprintf("Verification %s for %s\n\n%s", status, featureID, string(data)),
		OutputFormat: FormatJSON,
	}, nil
}

// ── HarnessStatusTool ────────────────────────────────────────────────────────

type HarnessStatusTool struct {
	BaseTool
}

func NewHarnessStatusTool() *HarnessStatusTool {
	return &HarnessStatusTool{
		BaseTool: NewBaseTool(
			"harness_status",
			"Show current project harness status — all features with their statuses, active feature, progress summary.",
			CategoryMeta,
			false,
			PermissionAlways,
		),
	}
}

func (t *HarnessStatusTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"project_root": {
				"type": "string",
				"description": "Project root directory (defaults to CWD)"
			}
		}
	}`)
}

func (t *HarnessStatusTool) Execute(_ context.Context, params map[string]interface{}) (*ToolResult, error) {
	projectRoot := resolveProjectRoot(params)
	store := harness.NewStore(projectRoot)

	fl, err := store.LoadFeatureList()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("load failed: %v", err)}, nil
	}

	state, _ := store.LoadState()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Project: %s\nGoal: %s\n\n", fl.ProjectName, fl.Goal))

	var passing, failing, inProgress int
	for _, f := range fl.Features {
		marker := "  "
		if state != nil && f.ID == state.ActiveFeatureID {
			marker = "→ "
		}
		sb.WriteString(fmt.Sprintf("%s[%s] %s — %s\n", marker, f.Status, f.ID, f.Title))
		switch f.Status {
		case harness.StatusPassing:
			passing++
		case harness.StatusFailing, harness.StatusIncomplete:
			failing++
		case harness.StatusInProgress:
			inProgress++
		}
	}

	sb.WriteString(fmt.Sprintf("\nTotal: %d | Passing: %d | Failing: %d | In Progress: %d",
		len(fl.Features), passing, failing, inProgress))

	return &ToolResult{
		Success:      true,
		Output:       sb.String(),
		OutputFormat: FormatText,
	}, nil
}

// ── HarnessUpdateTool ────────────────────────────────────────────────────────

type HarnessUpdateTool struct {
	BaseTool
}

func NewHarnessUpdateTool() *HarnessUpdateTool {
	return &HarnessUpdateTool{
		BaseTool: NewBaseTool(
			"harness_update",
			"Update a feature's metadata (title, description, status). Cannot set status to 'passing' — use harness_complete for that.",
			CategoryMeta,
			true,
			PermissionOnce,
		),
	}
}

func (t *HarnessUpdateTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"feature_id": {
				"type": "string",
				"description": "Feature ID to update"
			},
			"status": {
				"type": "string",
				"description": "New status (failing, incomplete, in_progress, skipped). Cannot be 'passing'."
			},
			"title": {
				"type": "string",
				"description": "New title"
			},
			"description": {
				"type": "string",
				"description": "New description"
			},
			"project_root": {
				"type": "string",
				"description": "Project root directory (defaults to CWD)"
			}
		},
		"required": ["feature_id"]
	}`)
}

func (t *HarnessUpdateTool) Execute(_ context.Context, params map[string]interface{}) (*ToolResult, error) {
	featureID, _ := params["feature_id"].(string)
	if featureID == "" {
		return &ToolResult{Success: false, Error: "feature_id is required"}, nil
	}

	newStatus, _ := params["status"].(string)
	newTitle, _ := params["title"].(string)
	newDesc, _ := params["description"].(string)

	// Enforce: cannot set passing via update
	if newStatus == string(harness.StatusPassing) {
		return &ToolResult{Success: false, Error: "cannot set status to 'passing' — use harness_complete which runs verification"}, nil
	}

	projectRoot := resolveProjectRoot(params)
	store := harness.NewStore(projectRoot)

	fl, err := store.LoadFeatureList()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("load failed: %v", err)}, nil
	}

	found := false
	for i := range fl.Features {
		if fl.Features[i].ID == featureID {
			if newStatus != "" {
				fl.Features[i].Status = harness.FeatureStatus(newStatus)
			}
			if newTitle != "" {
				fl.Features[i].Title = newTitle
			}
			if newDesc != "" {
				fl.Features[i].Description = newDesc
			}
			fl.Features[i].UpdatedAt = time.Now()
			found = true
			break
		}
	}

	if !found {
		return &ToolResult{Success: false, Error: fmt.Sprintf("feature %q not found", featureID)}, nil
	}

	fl.UpdatedAt = time.Now()
	if err := store.SaveFeatureList(fl); err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("save failed: %v", err)}, nil
	}

	return &ToolResult{
		Success: true,
		Output:  fmt.Sprintf("Feature %s updated successfully", featureID),
	}, nil
}

// ── VerifyFeatureTool ────────────────────────────────────────────────────────

type VerifyFeatureTool struct {
	BaseTool
}

func NewVerifyFeatureTool() *VerifyFeatureTool {
	return &VerifyFeatureTool{
		BaseTool: NewBaseTool(
			"verify_feature",
			"Run verification checks (build, test, custom commands) for a feature WITHOUT changing its status. Use this to check readiness before harness_complete.",
			CategoryMeta,
			true,
			PermissionOnce,
		),
	}
}

func (t *VerifyFeatureTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"feature_id": {
				"type": "string",
				"description": "Feature ID to verify"
			},
			"project_root": {
				"type": "string",
				"description": "Project root directory (defaults to CWD)"
			}
		},
		"required": ["feature_id"]
	}`)
}

func (t *VerifyFeatureTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	featureID, _ := params["feature_id"].(string)
	if featureID == "" {
		return &ToolResult{Success: false, Error: "feature_id is required"}, nil
	}

	projectRoot := resolveProjectRoot(params)
	store := harness.NewStore(projectRoot)

	fl, err := store.LoadFeatureList()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("load failed: %v", err)}, nil
	}

	var feature *harness.Feature
	for i := range fl.Features {
		if fl.Features[i].ID == featureID {
			feature = &fl.Features[i]
			break
		}
	}
	if feature == nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("feature %q not found", featureID)}, nil
	}

	verifier := harness.NewVerifier(store, nil)
	report, err := verifier.Verify(ctx, feature)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("verification error: %v", err)}, nil
	}

	data, _ := json.MarshalIndent(report, "", "  ")
	return &ToolResult{
		Success:      true,
		Output:       string(data),
		OutputFormat: FormatJSON,
	}, nil
}

// ── VerifyReportTool ─────────────────────────────────────────────────────────

type VerifyReportTool struct {
	BaseTool
}

func NewVerifyReportTool() *VerifyReportTool {
	return &VerifyReportTool{
		BaseTool: NewBaseTool(
			"verify_report",
			"Read the last verification report for a feature.",
			CategoryMeta,
			false,
			PermissionAlways,
		),
	}
}

func (t *VerifyReportTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"feature_id": {
				"type": "string",
				"description": "Feature ID to read report for"
			},
			"project_root": {
				"type": "string",
				"description": "Project root directory (defaults to CWD)"
			}
		},
		"required": ["feature_id"]
	}`)
}

func (t *VerifyReportTool) Execute(_ context.Context, params map[string]interface{}) (*ToolResult, error) {
	featureID, _ := params["feature_id"].(string)
	if featureID == "" {
		return &ToolResult{Success: false, Error: "feature_id is required"}, nil
	}

	projectRoot := resolveProjectRoot(params)
	store := harness.NewStore(projectRoot)

	report, err := store.LoadVerificationReport(featureID)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("no report found: %v", err)}, nil
	}

	data, _ := json.MarshalIndent(report, "", "  ")
	return &ToolResult{
		Success:      true,
		Output:       string(data),
		OutputFormat: FormatJSON,
	}, nil
}

// ── Registration ─────────────────────────────────────────────────────────────

func RegisterHarnessTools(reg *Registry) {
	_ = reg.Register(NewHarnessInitTool())
	_ = reg.Register(NewHarnessBootTool())
	_ = reg.Register(NewHarnessSelectTool())
	_ = reg.Register(NewHarnessCompleteTool())
	_ = reg.Register(NewHarnessStatusTool())
	_ = reg.Register(NewHarnessUpdateTool())
	_ = reg.Register(NewVerifyFeatureTool())
	_ = reg.Register(NewVerifyReportTool())
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func resolveProjectRoot(params map[string]interface{}) string {
	if root, ok := params["project_root"].(string); ok && root != "" {
		return root
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}
