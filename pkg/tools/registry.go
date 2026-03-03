package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/velariumai/gorkbot/pkg/scheduler"
	"github.com/velariumai/gorkbot/pkg/usercommands"
)

// contextKey is an unexported type for context keys to avoid collisions.
type contextKey struct{ name string }

// registryContextKey is the context key for the tool registry.
var registryContextKey = &contextKey{"registry"}

// orchestratorContextKey is the context key for the orchestrator.
var orchestratorContextKey = &contextKey{"orchestrator"}

// OrchestratorContextKey returns the context key for the orchestrator.
func OrchestratorContextKey() interface{} {
	return orchestratorContextKey
}

// PermissionHandler is a callback for requesting permission from the user
type PermissionHandler func(toolName string, params map[string]interface{}) PermissionLevel

// Registry manages all available tools
type Registry struct {
	tools              map[string]Tool
	permissionMgr      *PermissionManager
	analytics          *Analytics
	aiProvider         interface{} // AI provider for Task tool (set externally)
	consultantProvider interface{} // AI provider for Consultant tool (set externally)
	mu                 sync.RWMutex
	sessionPerms       map[string]bool // tools allowed for this session
	permissionHandler  PermissionHandler
	configDir          string              // config directory for persisting dynamic tools
	pendingRebuild     []string            // tools that need a Go rebuild for permanent integration
	disabledCategories   map[ToolCategory]bool // categories disabled via /settings
	schedulerInst        *scheduler.Scheduler    // optional: injected into ctx before tool execution
	userCmdLoader        *usercommands.Loader    // optional: injected into ctx before tool execution
	contextStats         ContextStatsReporter    // optional: injected into ctx for context_stats tool
	introspectionRep     IntrospectionReporter   // optional: injected into ctx for query_* tools
	goalLedger           GoalLedgerAccessor      // optional: injected into ctx for goal tools
	colonyRunner         func(ctx context.Context, sys, prompt string) (string, error) // runner for colony_debate tool
	// securityBriefFn returns a formatted brief of the current security assessment context.
	// Used by redteam agents to inject shared findings into their system prompts.
	securityBriefFn func() string
	// pipelineRunner executes an agent synchronously, returning its output.
	// Wired from the orchestrator so the run_pipeline tool can execute multi-step pipelines.
	pipelineRunner func(ctx context.Context, agentType, task string) (string, error)
}

// NewRegistry creates a new tool registry
func NewRegistry(permissionMgr *PermissionManager) *Registry {
	return &Registry{
		tools:              make(map[string]Tool),
		permissionMgr:      permissionMgr,
		analytics:          nil, // Will be set separately
		sessionPerms:       make(map[string]bool),
		disabledCategories: make(map[ToolCategory]bool),
	}
}

// SetCategoryEnabled enables or disables all tools in a category.
// Disabled tools return an error when executed rather than running.
func (r *Registry) SetCategoryEnabled(cat ToolCategory, enabled bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !enabled {
		r.disabledCategories[cat] = true
	} else {
		delete(r.disabledCategories, cat)
	}
}

// IsCategoryEnabled returns true when the category is not disabled.
func (r *Registry) IsCategoryEnabled(cat ToolCategory) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return !r.disabledCategories[cat]
}

// Categories returns all unique categories present in the registry.
func (r *Registry) Categories() []ToolCategory {
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := make(map[ToolCategory]bool)
	for _, t := range r.tools {
		seen[t.Category()] = true
	}
	cats := make([]ToolCategory, 0, len(seen))
	for c := range seen {
		cats = append(cats, c)
	}
	return cats
}

// SetPermissionHandler sets the callback for user permission requests
func (r *Registry) SetPermissionHandler(handler PermissionHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.permissionHandler = handler
}

// SetAnalytics sets the analytics tracker for the registry
func (r *Registry) SetAnalytics(analytics *Analytics) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.analytics = analytics
}

// SetAIProvider sets the AI provider for tools that need it (e.g., Task tool)
func (r *Registry) SetAIProvider(provider interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.aiProvider = provider
}

// SetConsultantProvider sets the Consultant AI provider
func (r *Registry) SetConsultantProvider(provider interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.consultantProvider = provider
}

// GetAIProvider returns the AI provider
func (r *Registry) GetAIProvider() interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.aiProvider
}

// GetConsultantProvider returns the Consultant AI provider
func (r *Registry) GetConsultantProvider() interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.consultantProvider
}

// ResolveConsultantProvider resolves the consultant to use for a task.
// It checks the orchestrator's live secondary model first (so the model
// selector and failover cascade are always respected), then falls back to the
// startup-time cached consultantProvider.
func (r *Registry) ResolveConsultantProvider(ctx context.Context, task string) interface{} {
	// Primary: ask the live orchestrator for the current secondary model.
	// This ensures /model selections and provider failover are honoured.
	type resolver interface {
		ResolveConsultant(ctx context.Context, task string) interface{}
	}
	orch := ctx.Value(orchestratorContextKey)
	if orch != nil {
		if res, ok := orch.(resolver); ok {
			if p := res.ResolveConsultant(ctx, task); p != nil {
				return p
			}
		}
	}

	// Fallback: use the startup-cached provider (set via SetConsultantProvider).
	r.mu.RLock()
	explicit := r.consultantProvider
	r.mu.RUnlock()
	return explicit
}

// GetPermissionManager returns the permission manager
func (r *Registry) GetPermissionManager() *PermissionManager {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.permissionMgr
}

// GetAnalytics returns the analytics tracker
func (r *Registry) GetAnalytics() *Analytics {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.analytics
}

// SetConfigDir sets the config directory used for dynamic tool persistence.
func (r *Registry) SetConfigDir(dir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.configDir = dir
}

// GetConfigDir returns the configured config directory.
func (r *Registry) GetConfigDir() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.configDir
}

// SetScheduler injects a scheduler instance that will be available to tools via context.
func (r *Registry) SetScheduler(s *scheduler.Scheduler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.schedulerInst = s
}

// SetUserCmdLoader injects a usercommands.Loader that will be available to tools via context.
func (r *Registry) SetUserCmdLoader(l *usercommands.Loader) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.userCmdLoader = l
}

// SetContextStatsReporter injects a ContextStatsReporter (typically the orchestrator's
// ContextManager) so the context_stats tool can query live token usage.
func (r *Registry) SetContextStatsReporter(rep ContextStatsReporter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.contextStats = rep
}

// SetIntrospectionReporter injects an IntrospectionReporter (typically the Orchestrator)
// so the query_* self-introspection tools can surface internal system state.
func (r *Registry) SetIntrospectionReporter(rep IntrospectionReporter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.introspectionRep = rep
}

// SetGoalLedger injects a GoalLedgerAccessor so the goal management tools
// can persist cross-session goals.
func (r *Registry) SetGoalLedger(gl GoalLedgerAccessor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.goalLedger = gl
}

// SetSecurityBriefFn injects a function that returns the current security assessment context brief.
// Used by redteam agents to inject shared findings into their system prompts.
func (r *Registry) SetSecurityBriefFn(fn func() string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.securityBriefFn = fn
}

// GetSecurityBrief returns the current security context brief, or "" if not set.
func (r *Registry) GetSecurityBrief() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.securityBriefFn != nil {
		return r.securityBriefFn()
	}
	return ""
}

// SetPipelineRunner injects a synchronous agent execution function used by the run_pipeline tool.
// The function takes (ctx, agentType, task) and returns the agent's output string.
func (r *Registry) SetPipelineRunner(fn func(ctx context.Context, agentType, task string) (string, error)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pipelineRunner = fn
}

// GetPipelineRunner returns the pipeline runner function, or nil if not set.
func (r *Registry) GetPipelineRunner() func(ctx context.Context, agentType, task string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.pipelineRunner
}

// SetColonyRunner sets the runner function used by the colony_debate tool and
// registers (or replaces) the tool immediately so it is available to agents.
func (r *Registry) SetColonyRunner(fn func(ctx context.Context, sys, prompt string) (string, error)) {
	r.mu.Lock()
	r.colonyRunner = fn
	r.mu.Unlock()
	// Register or replace so that a call before RegisterDefaultTools also works.
	r.RegisterOrReplace(NewColonyDebateTool(fn))
}

// Register adds a tool to the registry
func (r *Registry) Register(tool Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[tool.Name()]; exists {
		return fmt.Errorf("tool %s already registered", tool.Name())
	}

	r.tools[tool.Name()] = tool
	return nil
}

// RegisterOrReplace adds or replaces a tool in the registry (used for dynamic tools).
func (r *Registry) RegisterOrReplace(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name()] = tool
}

// Get retrieves a tool by name
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, exists := r.tools[name]
	return tool, exists
}

// List returns all registered tools
func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}
	return tools
}

// ListByCategory returns tools in a specific category
func (r *Registry) ListByCategory(category ToolCategory) []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var tools []Tool
	for _, tool := range r.tools {
		if tool.Category() == category {
			tools = append(tools, tool)
		}
	}
	return tools
}

// Execute executes a tool with permission checking and analytics tracking
func (r *Registry) Execute(ctx context.Context, req *ToolRequest) (*ToolResult, error) {
	startTime := time.Now()

	// Normalize tool name and params before lookup
	normalizedName := normalizeToolName(req.ToolName)
	normalizedParams := NormalizeToolParams(normalizedName, req.Parameters)

	// Get the tool using normalized name
	tool, exists := r.Get(normalizedName)
	if !exists {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("tool not found: %s", req.ToolName),
		}, fmt.Errorf("tool not found: %s", req.ToolName)
	}

	// Check category disabled
	if !r.IsCategoryEnabled(tool.Category()) {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("tool category %q is disabled — enable it in /settings", string(tool.Category())),
		}, nil
	}

	// Check permissions
	if tool.RequiresPermission() {
		allowed, err := r.checkPermission(normalizedName, normalizedParams)
		if err != nil {
			return &ToolResult{
				Success: false,
				Error:   fmt.Sprintf("permission error: %v", err),
			}, err
		}
		if !allowed {
			return &ToolResult{
				Success: false,
				Error:   "permission denied",
			}, fmt.Errorf("permission denied for tool: %s", normalizedName)
		}
	}

	// Add registry to context for meta tools that need it
	ctxWithRegistry := context.WithValue(ctx, registryContextKey, r)

	// Inject scheduler if available
	if r.schedulerInst != nil {
		ctxWithRegistry = context.WithValue(ctxWithRegistry, scheduler.SchedulerKey, r.schedulerInst)
	}
	// Inject user command loader if available
	if r.userCmdLoader != nil {
		ctxWithRegistry = context.WithValue(ctxWithRegistry, UserCommandLoaderKey, r.userCmdLoader)
	}
	// Inject context stats reporter if available
	if r.contextStats != nil {
		ctxWithRegistry = context.WithValue(ctxWithRegistry, ContextStatsKey, r.contextStats)
	}
	// Inject introspection reporter if available
	if r.introspectionRep != nil {
		ctxWithRegistry = context.WithValue(ctxWithRegistry, IntrospectionKey, r.introspectionRep)
	}
	// Inject goal ledger if available
	if r.goalLedger != nil {
		ctxWithRegistry = context.WithValue(ctxWithRegistry, GoalLedgerKey, r.goalLedger)
	}

	// Execute the tool with normalized params
	result, err := tool.Execute(ctxWithRegistry, normalizedParams)

	// Record analytics
	duration := time.Since(startTime)
	success := err == nil && result != nil && result.Success
	if r.analytics != nil {
		r.analytics.RecordExecution(normalizedName, success, duration)
	}

	return result, err
}

// checkPermission checks if a tool is allowed to execute
func (r *Registry) checkPermission(toolName string, params map[string]interface{}) (bool, error) {
	// Check session permissions first
	if allowed, exists := r.sessionPerms[toolName]; exists && allowed {
		return true, nil
	}

	// Check persistent permissions
	perm := r.permissionMgr.GetPermission(toolName)

	if perm == PermissionAlways {
		return true, nil
	} else if perm == PermissionNever {
		return false, nil
	} else if perm == PermissionSession {
		r.sessionPerms[toolName] = true
		return true, nil
	}

	// PermissionOnce or unknown - ask user
	if r.permissionHandler == nil {
		return false, fmt.Errorf("user confirmation required but no handler set")
	}

	// Ask user
	level := r.permissionHandler(toolName, params)

	switch level {
	case PermissionAlways:
		r.permissionMgr.SetPermission(toolName, PermissionAlways)
		return true, nil
	case PermissionSession:
		r.sessionPerms[toolName] = true
		return true, nil
	case PermissionOnce:
		return true, nil
	case PermissionNever:
		r.permissionMgr.SetPermission(toolName, PermissionNever)
		return false, nil
	default:
		return false, nil
	}
}

// GrantSessionPermission grants permission for the current session
func (r *Registry) GrantSessionPermission(toolName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessionPerms[toolName] = true
}

// RevokeSessionPermission revokes session permission
func (r *Registry) RevokeSessionPermission(toolName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessionPerms, toolName)
}

// ClearSessionPermissions clears all session permissions
func (r *Registry) ClearSessionPermissions() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessionPerms = make(map[string]bool)
}

// GetDefinitions returns tool definitions for AI models
func (r *Registry) GetDefinitions() []ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	definitions := make([]ToolDefinition, 0, len(r.tools))
	for _, tool := range r.tools {
		definitions = append(definitions, ToolDefinition{
			Name:        tool.Name(),
			Description: tool.Description(),
			Category:    string(tool.Category()),
			Parameters:  tool.Parameters(),
			// New fields - will be populated by tools that implement them
			WhenToUse: "",
			Returns:   "",
			Safety:    "",
		})
	}
	return definitions
}

// GetSystemPrompt generates the system prompt with tool definitions
func (r *Registry) GetSystemPrompt() string {
	definitions := r.GetDefinitions()
	if len(definitions) == 0 {
		return ""
	}

	var sb strings.Builder

	// Add tool selection guidance at the top
	sb.WriteString("### TOOL SELECTION GUIDE:\n")
	sb.WriteString("Use this guide to choose the right tool for your task:\n")
	sb.WriteString("- Need to find web resources on a topic? → Use 'web_search'\n")
	sb.WriteString("- Need clean text content from a URL? → Use 'web_fetch' (returns clean text, not HTML)\n")
	sb.WriteString("- Need to extract specific content from a URL using CSS/XPath? → Use 'scrapling_fetch'\n")
	sb.WriteString("- Need to run shell commands? → Use 'bash'\n")
	sb.WriteString("- Need to read a file? → Use 'read_file'\n")
	sb.WriteString("- Need to search for text in files? → Use 'grep_content'\n")
	sb.WriteString("- Need to make HTTP requests with custom headers/body? → Use 'http_request'\n")
	sb.WriteString("- Need to download a file? → Use 'download_file'\n\n")

	sb.WriteString("### AVAILABLE TOOLS:\n")
	for _, def := range definitions {
		sb.WriteString(fmt.Sprintf("#### %s\n", def.Name))
		sb.WriteString(fmt.Sprintf("- Description: %s\n", def.Description))

		// Add WhenToUse if present
		if def.WhenToUse != "" {
			sb.WriteString(fmt.Sprintf("- When to use: %s\n", def.WhenToUse))
		}

		// Add Returns if present
		if def.Returns != "" {
			sb.WriteString(fmt.Sprintf("- Returns: %s\n", def.Returns))
		}

		// Add Safety notes if present
		if def.Safety != "" {
			sb.WriteString(fmt.Sprintf("- Safety: %s\n", def.Safety))
		}

		sb.WriteString(fmt.Sprintf("- Parameters: %s\n\n", string(def.Parameters)))
	}

	sb.WriteString("### HOW TO USE A TOOL:\n")
	sb.WriteString("CRITICAL: Output tool calls ONLY as markdown JSON code blocks (```json ... ```). " +
		"Do NOT use [TOOL_CALL] tags, XML/HTML tags, arrow syntax (=>), or any other format. " +
		"The system can ONLY parse the JSON block format shown below.\n\n")
	sb.WriteString("Correct format — use this EXACTLY:\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"tool\": \"tool_name\",\n")
	sb.WriteString("  \"parameters\": {\n")
	sb.WriteString("    \"param1\": \"value1\"\n")
	sb.WriteString("  }\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n")
	sb.WriteString("To call multiple tools, output multiple separate JSON blocks. " +
		"Never wrap tool calls in [TOOL_CALL] tags or any other delimiter.\n\n")

	return sb.String()
}

// GetSystemPromptNative generates a tool listing for the native function-calling path.
// Unlike GetSystemPrompt it omits the "HOW TO USE A TOOL" JSON-block section because
// the model already receives structured tool schemas via the API tools field.
func (r *Registry) GetSystemPromptNative() string {
	definitions := r.GetDefinitions()
	if len(definitions) == 0 {
		return ""
	}

	var sb strings.Builder

	// Add tool selection guidance
	sb.WriteString("### TOOL SELECTION GUIDE:\n")
	sb.WriteString("Use this guide to choose the right tool for your task:\n")
	sb.WriteString("- Need to find web resources on a topic? → Use 'web_search'\n")
	sb.WriteString("- Need clean text content from a URL? → Use 'web_fetch' (returns clean text, not HTML)\n")
	sb.WriteString("- Need to extract specific content from a URL using CSS/XPath? → Use 'scrapling_fetch'\n")
	sb.WriteString("- Need to run shell commands? → Use 'bash'\n")
	sb.WriteString("- Need to read a file? → Use 'read_file'\n")
	sb.WriteString("- Need to search for text in files? → Use 'grep_content'\n")
	sb.WriteString("- Need to make HTTP requests with custom headers/body? → Use 'http_request'\n")
	sb.WriteString("- Need to download a file? → Use 'download_file'\n\n")

	sb.WriteString("### AVAILABLE TOOLS:\n")
	for _, def := range definitions {
		sb.WriteString(fmt.Sprintf("#### %s\n", def.Name))
		sb.WriteString(fmt.Sprintf("- Description: %s\n", def.Description))

		// Add WhenToUse if present
		if def.WhenToUse != "" {
			sb.WriteString(fmt.Sprintf("- When to use: %s\n", def.WhenToUse))
		}

		// Add Returns if present
		if def.Returns != "" {
			sb.WriteString(fmt.Sprintf("- Returns: %s\n", def.Returns))
		}

		// Add Safety notes if present
		if def.Safety != "" {
			sb.WriteString(fmt.Sprintf("- Safety: %s\n", def.Safety))
		}

		sb.WriteString(fmt.Sprintf("- Parameters: %s\n\n", string(def.Parameters)))
	}
	// No "HOW TO USE A TOOL" block — native function calling handles invocation.
	return sb.String()
}

// RegisterDefaultTools registers all built-in tools
func (r *Registry) RegisterDefaultTools() error {
	tools := []Tool{
		// Shell tools
		NewBashTool(),

		// File tools
		NewReadFileTool(),
		NewWriteFileTool(),
		NewEditFileTool(),
		NewMultiEditFileTool(),
		NewListDirectoryTool(),
		NewSearchFilesTool(),
		NewGrepContentTool(),
		NewFileInfoTool(),
		NewDeleteFileTool(),

		// Hashline file tools (hash-validated edits — prevents stale-line failures)
		NewReadFileHashedTool(),
		NewEditFileHashedTool(),
		NewASTGrepTool(),

		// Git tools
		NewGitStatusTool(),
		NewGitDiffTool(),
		NewGitLogTool(),
		NewGitCommitTool(),
		NewGitPushTool(),
		NewGitPullTool(),

		// Web tools
		NewWebFetchTool(),
		NewHttpRequestTool(),
		NewCheckPortTool(),
		NewDownloadFileTool(),
		NewXPullTool(),

		// System tools
		NewListProcessesTool(),
		NewKillProcessTool(),
		NewEnvVarTool(),
		NewSystemInfoTool(),
		NewDiskUsageTool(),

		// Meta tools
		NewCreateToolTool(),
		NewModifyToolTool(),
		NewListToolsTool(),
		NewToolInfoTool(),
		NewAuditToolCallTool(),
		NewContextStatsTool(),

		// Task management
		NewTodoWriteTool(),
		NewTodoReadTool(),
		NewCompleteTool(),

		// Skills
		NewConsultationTool(),
		NewWebSearchTool(),
		NewWebReaderTool(),
		NewDOCXTool(),
		NewXLSXTool(),
		NewPDFTool(),
		NewPPTXTool(),
		NewFrontendDesignTool(),

		// AI / ML tools
		NewAIImageGenerateTool(),
		NewAISummarizeAudioTool(),
		NewMLModelRunTool(),

		// Background agent tools (parallel sub-agent execution)
		NewSpawnAgentTool(),
		NewCollectAgentTool(),
		NewListAgentsTool(),

		// Database tools
		NewDBQueryTool(),
		NewDBMigrateTool(),

		// Network tools
		NewNetworkScanTool(),
		NewSocketConnectTool(),
		NewNetworkEscapeProxyTool(),

		// Media tools
		NewImageProcessTool(),
		NewMediaConvertTool(),

		// Android / Termux tools — android_control.go (granular device control)
		NewAdbScreenshotTool(),
		NewAdbShellTool(),
		NewAppCatalogTool(),
		NewAppControlTool(),
		NewAppStatusTool(),
		NewScreenCaptureTool(),
		NewScreenshotTool(),
		NewScreenrecordTool(),
		NewCaptureScreenHackTool(),
		NewUiDumpTool(),
		NewDeviceInfoTool(),
		NewContextStateTool(),
		NewKillAppTool(),
		NewLaunchAppTool(),
		NewVisionCaptureTool(),
		NewVisionAnalyzeTool(),
		NewLiveVisionTool(),
		NewScreenAnalyzeTool(),
		NewManageDepsTool(),
		NewTermuxControlTool(),
		NewSaveStateTool(),
		NewStartHealthMonitorTool(),
		NewBrowserScrapeTool(),
		NewBrowserControlTool(),

		// Android / Termux tools — other sources
		NewSensorReadTool(),
		NewNotificationSendTool(),
		NewIntentBroadcastTool(),
		NewLogcatDumpTool(),
		NewClipboardManagerTool(),
		NewNotificationListenerTool(),
		NewAccessibilityQueryTool(),
		NewApkDecompileTool(),
		NewSqliteExplorerTool(),
		NewTermuxApiBridgeTool(),

		// Worktree tools (isolated git environments for agents)
		NewCreateWorktreeTool(),
		NewListWorktreesTool(),
		NewRemoveWorktreeTool(),
		NewIntegrateWorktreeTool(),

		// DevOps & Cloud tools
		NewDockerManagerTool(),
		NewK8sKubectlTool(),
		NewAwsS3SyncTool(),
		NewGitBlameAnalyzeTool(),
		NewNgrokTunnelTool(),
		NewCiTriggerTool(),

		// Security tools (basic)
		NewNmapScanTool(),
		NewPacketCaptureTool(),
		NewWifiAnalyzerTool(),
		NewShodanQueryTool(),
		NewMetasploitRpcTool(),
		NewSslValidatorTool(),

		// Security tools — comprehensive pentesting suite
		NewMasscanRunTool(),
		NewDnsEnumTool(),
		NewArpScanRunTool(),
		NewTracerouteRunTool(),
		NewNiktoScanTool(),
		NewGobusterScanTool(),
		NewFfufRunTool(),
		NewSqlmapScanTool(),
		NewWafw00fRunTool(),
		NewHttpHeaderAuditTool(),
		NewJwtDecodeTool(),
		NewHydraRunTool(),
		NewHashcatRunTool(),
		NewJohnRunTool(),
		NewHashIdentifyTool(),
		NewSearchsploitQueryTool(),
		NewCveLookupTool(),
		NewEnum4linuxRunTool(),
		NewSmbmapRunTool(),
		NewSuidCheckTool(),
		NewSudoCheckTool(),
		NewLinpeasRunTool(),
		NewStringsAnalyzeTool(),
		NewHexdumpFileTool(),
		NewNetstatAnalysisTool(),
		NewSubfinderRunTool(),
		NewNucleiScanTool(),
		NewTotpGenerateTool(),

		// Media & Content tools
		NewFfmpegProTool(),
		NewAudioTranscribeTool(),
		NewTtsGenerateTool(),
		NewImageOcrBatchTool(),
		NewVideoSummarizeTool(),
		NewMemeGeneratorTool(),

		// Data Science & Knowledge tools
		NewCsvPivotTool(),
		NewPlotGenerateTool(),
		NewArxivSearchTool(),
		NewWebArchiveTool(),
		NewWhoisLookupTool(),

		// Personal & Life tools
		NewCalendarManageTool(),
		NewEmailClientTool(),
		NewContactSyncTool(),
		NewSmartHomeApiTool(),

		// Meta & Maintenance tools
		NewCronManagerTool(),
		NewBackupRestoreTool(),
		NewSystemMonitorTool(),

		// Package management
		NewPkgInstallTool(),

		// SENSE tools
		NewCode2WorldTool(),
		NewRecordEngramTool(),

		// Self-introspection tools (query internal intelligence systems)
		NewQueryRoutingStatsTool(),
		NewQueryHeuristicsTool(),
		NewQueryMemoryStateTool(),
		NewQuerySystemStateTool(),

		// Prospective memory: cross-session goal ledger
		NewAddGoalTool(),
		NewCloseGoalTool(),
		NewListGoalsTool(),

		// Security assessment: shared findings context
		NewReportFindingTool(),

		// Agentic pipeline: multi-step coordinated agent execution
		NewRunPipelineTool(),

		// Vision tools (MediaProjection companion + Grok Vision API)
		NewVisionInstallTool(),
		NewADBSetupTool(), // ADB diagnostics only — not used for capture
		NewVisionScreenTool(),
		NewVisionCaptureOnlyTool(),
		NewVisionFileTool(),
		NewVisionOCRTool(),
		NewVisionFindTool(),
		NewVisionWatchTool(),

		// Sprint 1 - New tools from nanoclaw/opencrabs port
		NewDocParserTool(),
		NewCodeExecTool(),
		NewRebuildTool(),
		NewScheduleTaskTool(),
		NewListScheduledTasksTool(),
		NewCancelScheduledTaskTool(),
		NewPauseResumeScheduledTaskTool(),
		NewDefineCommandTool(),

		// Scrapling web scraping tools
		NewScraplingFetchTool(),
		NewScraplingStealthTool(),
		NewScraplingDynamicTool(),
		NewScraplingExtractTool(),
		NewScraplingSearchTool(),

		// Jupyter notebook tool
		NewJupyterTool(),
	}

	for _, tool := range tools {
		if err := r.Register(tool); err != nil {
			return fmt.Errorf("failed to register %s: %w", tool.Name(), err)
		}
	}

	// Colony debate tool — runner wired later via SetColonyRunner
	if r.colonyRunner != nil {
		if err := r.Register(NewColonyDebateTool(r.colonyRunner)); err != nil {
			return fmt.Errorf("failed to register colony_debate: %w", err)
		}
	}

	// CCI retrieval tools — mcp_context_* suite (Tier 3 cold memory, Tier 2 specialists)
	RegisterCCITools(r)

	return nil
}
