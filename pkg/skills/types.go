// Package skills implements the skill registry and workflow management for Gorkbot.
// Skills are composable units of functionality with metadata and semantic discovery.
package skills

import (
	"time"
)

// SkillManifest defines a skill (from .gorkskill.yaml).
type SkillManifest struct {
	// Metadata
	Name          string `yaml:"name" json:"name"`                                         // Skill identifier
	SchemaVersion string `yaml:"schema_version,omitempty" json:"schema_version,omitempty"` // Skill schema version (default: 1.0)
	Version       string `yaml:"version" json:"version"`                                   // Semantic version
	Description   string `yaml:"description" json:"description"`                           // Human-readable description
	Author        string `yaml:"author,omitempty" json:"author"`                           // Skill author
	License       string `yaml:"license,omitempty" json:"license"`                         // License (MIT, Apache-2.0, etc.)

	// Semantic info (for discovery)
	Keywords  []string `yaml:"keywords,omitempty" json:"keywords"`   // Search keywords
	Category  string   `yaml:"category,omitempty" json:"category"`   // Category (e.g., "data", "ai", "devops")
	Tags      []string `yaml:"tags,omitempty" json:"tags"`           // Additional tags
	Thumbnail string   `yaml:"thumbnail,omitempty" json:"thumbnail"` // Icon URL

	// Skill definition
	Tools        []SkillTool   `yaml:"tools" json:"tools"`                         // Required tools
	Prompts      []SkillPrompt `yaml:"prompts,omitempty" json:"prompts"`           // Skill prompts
	Workflows    []Workflow    `yaml:"workflows,omitempty" json:"workflows"`       // Workflows
	Dependencies []string      `yaml:"dependencies,omitempty" json:"dependencies"` // Depends on other skills

	// Permissions (Task 5.4)
	Permissions []SkillPermission `yaml:"permissions,omitempty" json:"permissions,omitempty"` // Permission rules

	// Metadata
	Created  time.Time `yaml:"created,omitempty" json:"created"`
	Modified time.Time `yaml:"modified,omitempty" json:"modified"`
	Enabled  bool      `yaml:"enabled,omitempty" json:"enabled"`
}

// SkillPermission declares a permission rule that a skill requires (Task 5.4).
type SkillPermission struct {
	Tool    string `yaml:"tool" json:"tool"`                 // Tool name (e.g., "write_file")
	Pattern string `yaml:"pattern,omitempty" json:"pattern"` // Optional pattern for param matching
	Level   string `yaml:"level" json:"level"`               // "always", "session", "once", or "never"
}

// SkillTool describes a tool used by the skill.
type SkillTool struct {
	Name     string                 `yaml:"name" json:"name"`                   // Tool name
	Required bool                   `yaml:"required,omitempty" json:"required"` // Is required
	Config   map[string]interface{} `yaml:"config,omitempty" json:"config"`     // Tool config
}

// SkillPrompt is a prompt template within a skill.
type SkillPrompt struct {
	Name     string `yaml:"name" json:"name"`                 // Prompt identifier
	Template string `yaml:"template" json:"template"`         // Template with {{vars}}
	Context  string `yaml:"context,omitempty" json:"context"` // Context description
}

// Workflow defines a composition of steps.
type Workflow struct {
	Name        string         `yaml:"name" json:"name"`                         // Workflow identifier
	Description string         `yaml:"description,omitempty" json:"description"` // Description
	Trigger     string         `yaml:"trigger,omitempty" json:"trigger"`         // Trigger condition
	Steps       []WorkflowStep `yaml:"steps" json:"steps"`                       // Execution steps
	Retry       *RetryPolicy   `yaml:"retry,omitempty" json:"retry"`             // Retry strategy
	Timeout     string         `yaml:"timeout,omitempty" json:"timeout"`         // Execution timeout
}

// WorkflowStep is a single step in a workflow.
type WorkflowStep struct {
	ID        string                 `yaml:"id" json:"id"`                         // Step identifier
	Type      string                 `yaml:"type" json:"type"`                     // "tool", "prompt", "condition", "fork"
	Name      string                 `yaml:"name,omitempty" json:"name"`           // Display name
	Tool      string                 `yaml:"tool,omitempty" json:"tool"`           // Tool to execute
	Params    map[string]interface{} `yaml:"params,omitempty" json:"params"`       // Parameters
	OnSuccess string                 `yaml:"onSuccess,omitempty" json:"onSuccess"` // Next step on success
	OnFailure string                 `yaml:"onFailure,omitempty" json:"onFailure"` // Next step on failure
	Condition string                 `yaml:"condition,omitempty" json:"condition"` // Condition to evaluate
}

// RetryPolicy defines retry behavior.
type RetryPolicy struct {
	MaxAttempts int    `yaml:"maxAttempts" json:"maxAttempts"`   // Maximum attempts
	Backoff     string `yaml:"backoff,omitempty" json:"backoff"` // "exponential", "linear"
	MaxWait     string `yaml:"maxWait,omitempty" json:"maxWait"` // Max wait between retries
}

// SkillMetadata represents a skill with computed metadata.
type SkillMetadata struct {
	Manifest         *SkillManifest
	FilePath         string    // Path to .gorkskill.yaml
	SearchScore      float64   // Relevance score for search
	UsageCount       int       // Number of uses
	LastUsed         time.Time // Last execution time
	AvgExecutionTime int64     // Average execution time in ms
}

// SearchResult represents a skill search result.
type SearchResult struct {
	Skill      *SkillMetadata
	Score      float64
	MatchType  string // "keyword", "semantic", "category"
	MatchField string // Which field matched
}

// Registry manages available skills.
type Registry interface {
	// Register adds a new skill.
	Register(manifest *SkillManifest, filePath string) error

	// Unregister removes a skill.
	Unregister(name string) error

	// Get retrieves a skill by name.
	Get(name string) *SkillMetadata

	// List returns all skills.
	List() []*SkillMetadata

	// Search finds skills by keyword or semantic similarity.
	Search(query string, limit int) []*SearchResult

	// Enable/Disable skill
	Enable(name string) error
	Disable(name string) error

	// GetWorkflow retrieves a workflow from a skill.
	GetWorkflow(skillName, workflowName string) *Workflow

	// ResolveTools returns all tools needed for a skill and its dependencies.
	ResolveTools(skillName string) ([]string, error)
}
