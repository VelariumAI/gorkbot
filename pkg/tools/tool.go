package tools

import (
	"context"
	"encoding/json"
)

// OutputFormat defines what format tool output is in
type OutputFormat string

const (
	// FormatText - Human-readable plain text
	FormatText OutputFormat = "text"
	// FormatJSON - Structured JSON
	FormatJSON OutputFormat = "json"
	// FormatHTML - Raw HTML
	FormatHTML OutputFormat = "html"
	// FormatList - Key-value pairs
	FormatList OutputFormat = "list"
	// FormatError - Error message only
	FormatError OutputFormat = "error"
)

// PermissionLevel defines how tools can be executed
type PermissionLevel string

const (
	// PermissionAlways - Tool always allowed, no confirmation needed
	PermissionAlways PermissionLevel = "always"

	// PermissionSession - Allowed for current session only
	PermissionSession PermissionLevel = "session"

	// PermissionOnce - Ask for confirmation each time
	PermissionOnce PermissionLevel = "once"

	// PermissionNever - Tool is disabled
	PermissionNever PermissionLevel = "never"
)

// ToolCategory categorizes tools for organization
type ToolCategory string

const (
	CategoryShell         ToolCategory = "shell"
	CategoryFile          ToolCategory = "file"
	CategoryGit           ToolCategory = "git"
	CategoryWeb           ToolCategory = "web"
	CategorySystem        ToolCategory = "system"
	CategoryCommunication ToolCategory = "communication"
	CategoryMeta          ToolCategory = "meta"
	CategoryCustom        ToolCategory = "custom"
	CategoryAI            ToolCategory = "ai"
	CategoryDatabase      ToolCategory = "database"
	CategoryNetwork       ToolCategory = "network"
	CategoryMedia         ToolCategory = "media"
	CategoryAndroid       ToolCategory = "android"
	CategoryPackage       ToolCategory = "package"
	CategorySecurity      ToolCategory = "security"
)

// Tool defines the interface all tools must implement
type Tool interface {
	// Name returns the unique identifier for this tool
	Name() string

	// Description returns a human-readable description
	Description() string

	// Category returns the tool's category
	Category() ToolCategory

	// Parameters returns JSON schema for the tool's parameters
	Parameters() json.RawMessage

	// Execute runs the tool with given parameters
	Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error)

	// RequiresPermission indicates if this tool needs user approval
	RequiresPermission() bool

	// DefaultPermission returns the default permission level
	DefaultPermission() PermissionLevel

	// OutputFormat returns the format of the tool's output
	OutputFormat() OutputFormat
}

// CapabilityRequirer is an optional interface a Tool can implement to declare
// the external binaries and Python packages it needs at runtime.
// The Registry checks these before executing the tool and returns a clear
// "not installed" error instead of running the tool and getting a cryptic
// failure deep in subprocess output.
//
// Tools should only declare hard requirements — binaries or packages without
// which the tool cannot function at all.  Optional enhancements should not
// be listed here.
type CapabilityRequirer interface {
	// RequiredBinaries returns CLI tool names that must be present in PATH.
	// Example: []string{"nmap"} for the nmap_scan tool.
	RequiredBinaries() []string

	// RequiredPythonPackages returns Python package import names that must be
	// importable.  Use the import name, not the pip name (e.g. "google.genai"
	// not "google-genai").
	RequiredPythonPackages() []string
}

// ToolResult represents the result of tool execution
type ToolResult struct {
	Success      bool                   `json:"success"`
	Output       string                 `json:"output"`
	Error        string                 `json:"error,omitempty"`
	Data         map[string]interface{} `json:"data,omitempty"`
	OutputFormat OutputFormat           `json:"output_format"` // What format the output is in
	AuthRequired bool                   `json:"auth_required,omitempty"`
	AuthType     string                 `json:"auth_type,omitempty"`
}

// ToolRequest represents a request to execute a tool
type ToolRequest struct {
	ToolName   string                 `json:"tool"`
	Parameters map[string]interface{} `json:"parameters"`
	RequestID  string                 `json:"request_id"`
	AgentID    string                 `json:"agent_id"` // "grok" or "gemini"
}

// ToolDefinition provides metadata about a tool for AI models
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Category    string          `json:"category"`
	Parameters  json.RawMessage `json:"parameters"`
	Examples    []ToolExample   `json:"examples,omitempty"`
	// NEW FIELDS for better AI guidance
	WhenToUse string `json:"when_to_use,omitempty"` // When to prefer this tool
	Returns   string `json:"returns,omitempty"`     // What the output contains
	Safety    string `json:"safety,omitempty"`      // Security notes
}

// ToolExample shows how to use a tool
type ToolExample struct {
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// NewBaseTool creates a new BaseTool instance
func NewBaseTool(name, description string, category ToolCategory, requiresPerm bool, defaultPerm PermissionLevel) BaseTool {
	return BaseTool{
		name:               name,
		description:        description,
		category:           category,
		requiresPermission: requiresPerm,
		defaultPermission:  defaultPerm,
	}
}

// BaseTool provides common functionality for tools
type BaseTool struct {
	name               string
	description        string
	category           ToolCategory
	requiresPermission bool
	defaultPermission  PermissionLevel
}

func (b *BaseTool) Name() string {
	return b.name
}

func (b *BaseTool) Description() string {
	return b.description
}

func (b *BaseTool) Category() ToolCategory {
	return b.category
}

func (b *BaseTool) RequiresPermission() bool {
	return b.requiresPermission
}

func (b *BaseTool) DefaultPermission() PermissionLevel {
	return b.defaultPermission
}

func (b *BaseTool) OutputFormat() OutputFormat {
	return FormatText
}

// IsFileModifier returns true if the given tool name is known to modify workspace files.
func IsFileModifier(toolName string) bool {
	switch toolName {
	case "write_file", "edit_file", "edit_file_hashed", "delete_file", "run_bash",
		"bash", "structured_bash", "privileged_execute", "execute_command":
		return true
	default:
		return false
	}
}
