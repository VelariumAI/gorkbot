package config

import (
	"fmt"
	"strings"
	"time"
)

// GorkbotConfig represents the complete GORKBOT.md configuration
type GorkbotConfig struct {
	// [providers] section - AI provider definitions
	Providers map[string]ProviderConfig `mapstructure:"providers" toml:"providers"`

	// [capabilities] section - Which provider handles which capability
	Capabilities CapabilitiesConfig `mapstructure:"capabilities" toml:"capabilities"`

	// [routing] section - Intent/file-type/directory routing
	Routing RoutingConfig `mapstructure:"routing" toml:"routing"`

	// [specialist] section - Autonomous delegation settings
	Specialist SpecialistConfig `mapstructure:"specialist" toml:"specialist"`

	// [browser] section - Browser automation settings
	Browser BrowserConfig `mapstructure:"browser" toml:"browser"`

	// [mcp] section - Model Context Protocol settings
	MCP MCPConfig `mapstructure:"mcp" toml:"mcp"`

	// [auto_repair] section - Auto-fix settings
	AutoRepair AutoRepairConfig `mapstructure:"auto_repair" toml:"auto_repair"`

	// [context] section - Context optimization
	Context ContextConfig `mapstructure:"context" toml:"context"`

	// [memory] section - Memory and SENSE behavior
	Memory MemoryConfig `mapstructure:"memory" toml:"memory"`

	// [optimization] section - Cost/speed tradeoffs
	Optimization OptimizationConfig `mapstructure:"optimization" toml:"optimization"`

	// [prompts] section - Prompt variant selection
	Prompts PromptsConfig `mapstructure:"prompts" toml:"prompts"`

	// [debug] section - Debug settings
	Debug DebugConfig `mapstructure:"debug" toml:"debug"`

	// [advanced] section - Advanced power user options
	Advanced AdvancedConfig `mapstructure:"advanced" toml:"advanced"`
}

// ProviderConfig defines a single AI provider
type ProviderConfig struct {
	// Basic connection
	APIKey  string `mapstructure:"api_key" toml:"api_key"`
	Model   string `mapstructure:"model" toml:"model"`
	BaseURL string `mapstructure:"base_url" toml:"base_url"` // Optional, for self-hosted

	// Connection settings
	Timeout    int `mapstructure:"timeout" toml:"timeout"` // seconds
	MaxRetries int `mapstructure:"max_retries" toml:"max_retries"`

	// Capability flags
	Thinking struct {
		Enabled bool `mapstructure:"enabled" toml:"enabled"`
		Budget  int  `mapstructure:"budget" toml:"budget"` // tokens
	} `mapstructure:"thinking" toml:"thinking"`

	Vision struct {
		Enabled bool `mapstructure:"enabled" toml:"enabled"`
	} `mapstructure:"vision" toml:"vision"`

	Caching struct {
		Enabled bool `mapstructure:"enabled" toml:"enabled"`
		TTL     int  `mapstructure:"ttl" toml:"ttl"` // seconds
	} `mapstructure:"caching" toml:"caching"`

	NativeTools struct {
		Enabled bool `mapstructure:"enabled" toml:"enabled"`
	} `mapstructure:"native_tools" toml:"native_tools"`

	// Cost tracking
	CostPer1MInput  float64 `mapstructure:"cost_per_1m_input" toml:"cost_per_1m_input"`
	CostPer1MOutput float64 `mapstructure:"cost_per_1m_output" toml:"cost_per_1m_output"`

	// Provider-specific settings
	Extra map[string]interface{} `mapstructure:",remain" toml:",remain"` // Catch-all for provider-specific options
}

// CapabilitiesConfig maps capabilities to providers
type CapabilitiesConfig struct {
	// Which provider handles extended thinking
	ThinkingProvider string `mapstructure:"thinking_provider" toml:"thinking_provider"`

	// Which provider handles coding tasks
	CodingProvider string `mapstructure:"coding_provider" toml:"coding_provider"`

	// Which provider handles vision/image analysis
	VisionProvider string `mapstructure:"vision_provider" toml:"vision_provider"`

	// Which provider handles memory/fact extraction
	MemoryProvider string `mapstructure:"memory_provider" toml:"memory_provider"`

	// Which provider handles specialist delegation
	SpecialistProvider string `mapstructure:"specialist_provider" toml:"specialist_provider"`

	// Default provider for unclassified tasks
	DefaultProvider string `mapstructure:"default_provider" toml:"default_provider"`
}

// GetCapabilityProvider returns the provider configured for a given capability.
func (c CapabilitiesConfig) GetCapabilityProvider(capability string) (string, error) {
	switch strings.ToLower(capability) {
	case "thinking":
		if c.ThinkingProvider != "" {
			return c.ThinkingProvider, nil
		}
	case "coding":
		if c.CodingProvider != "" {
			return c.CodingProvider, nil
		}
	case "vision":
		if c.VisionProvider != "" {
			return c.VisionProvider, nil
		}
	case "memory":
		if c.MemoryProvider != "" {
			return c.MemoryProvider, nil
		}
	case "specialist":
		if c.SpecialistProvider != "" {
			return c.SpecialistProvider, nil
		}
	case "default":
		if c.DefaultProvider != "" {
			return c.DefaultProvider, nil
		}
	}

	if c.DefaultProvider != "" {
		return c.DefaultProvider, nil
	}
	return "", fmt.Errorf("no provider configured for capability: %s", capability)
}

// RoutingConfig defines routing rules
type RoutingConfig struct {
	// Intent-based routing: "refactoring" -> "anthropic"
	IntentClass map[string]string `mapstructure:"intent_class" toml:"intent_class"`

	// File-type routing: "*.py" -> "anthropic"
	FileType map[string]string `mapstructure:"file_type" toml:"file_type"`

	// Directory routing: "/src/**" -> "openai"
	Directory map[string]string `mapstructure:"directory" toml:"directory"`

	// Fallback chains: "anthropic_fallback" -> ["openai", "google", "grok"]
	Fallback map[string][]string `mapstructure:"fallback" toml:"fallback"`

	// Cost optimization settings
	CostOptimize struct {
		BudgetPerDay float64 `mapstructure:"budget_per_day" toml:"budget_per_day"` // $
		CostWeight   float64 `mapstructure:"cost_weight" toml:"cost_weight"`       // 0-1
		SpeedWeight  float64 `mapstructure:"speed_weight" toml:"speed_weight"`     // 0-1
	} `mapstructure:"cost_optimize" toml:"cost_optimize"`
}

// SpecialistConfig configures autonomous task delegation
type SpecialistConfig struct {
	// Which provider the specialist uses
	Provider string `mapstructure:"provider" toml:"provider"`

	// Thinking budget for specialist
	ThinkingBudget int `mapstructure:"thinking_budget" toml:"thinking_budget"`

	// Delegation thresholds
	ComplexityThreshold int `mapstructure:"complexity_threshold" toml:"complexity_threshold"` // 1-10
	FilesThreshold      int `mapstructure:"files_threshold" toml:"files_threshold"`           // number of files
	DurationThreshold   int `mapstructure:"duration_threshold" toml:"duration_threshold"`     // seconds

	// Autonomy level: "supervised", "validated", or "autonomous"
	AutonomyLevel string `mapstructure:"autonomy_level" toml:"autonomy_level"`

	// Whether specialist is enabled
	Enabled bool `mapstructure:"enabled" toml:"enabled"`
}

// BrowserConfig configures browser automation
type BrowserConfig struct {
	// Enable/disable browser
	Enabled bool `mapstructure:"enabled" toml:"enabled"`

	// Run headless (no visual window)
	Headless bool `mapstructure:"headless" toml:"headless"`

	// Vision provider for screenshot analysis
	VisionProvider string `mapstructure:"vision_provider" toml:"vision_provider"`

	// Timeout for browser operations
	Timeout int `mapstructure:"timeout" toml:"timeout"` // seconds

	// Screenshot settings
	Screenshots struct {
		Enabled  bool `mapstructure:"enabled" toml:"enabled"`
		Quality  int  `mapstructure:"quality" toml:"quality"` // 1-100
		MaxWidth int  `mapstructure:"max_width" toml:"max_width"`
	} `mapstructure:"screenshots" toml:"screenshots"`
}

// MCPConfig configures Model Context Protocol
type MCPConfig struct {
	// Enable/disable MCP
	Enabled bool `mapstructure:"enabled" toml:"enabled"`

	// Which servers to load
	Servers []string `mapstructure:"servers" toml:"servers"`

	// Auto-approval patterns
	AutoApprove struct {
		Enabled bool     `mapstructure:"enabled" toml:"enabled"`
		Tools   []string `mapstructure:"tools" toml:"tools"`
	} `mapstructure:"auto_approve" toml:"auto_approve"`
}

// AutoRepairConfig configures auto-fixing of syntax errors
type AutoRepairConfig struct {
	// Enable/disable auto-repair
	Enabled bool `mapstructure:"enabled" toml:"enabled"`

	// Linters/compilers to use
	Linters []string `mapstructure:"linters" toml:"linters"` // ["go vet", "tsc", "ruff"]

	// Use file hashing to prevent stale edit failures
	UseHashline bool `mapstructure:"use_hashline" toml:"use_hashline"`

	// Max attempts to fix
	MaxAttempts int `mapstructure:"max_attempts" toml:"max_attempts"`
}

// ContextConfig configures context optimization
type ContextConfig struct {
	// Enable prompt caching
	Caching struct {
		Enabled bool `mapstructure:"enabled" toml:"enabled"`
		TTL     int  `mapstructure:"ttl" toml:"ttl"` // seconds
	} `mapstructure:"caching" toml:"caching"`

	// Compression strategy: "none", "semantic", "selective", "aggressive"
	CompressionStrategy string `mapstructure:"compression_strategy" toml:"compression_strategy"`

	// Max compression level (1-10)
	CompressionLevel int `mapstructure:"compression_level" toml:"compression_level"`

	// Use Tree-sitter for AST navigation
	UseAST bool `mapstructure:"use_ast" toml:"use_ast"`

	// Max context window (tokens)
	MaxWindow int `mapstructure:"max_window" toml:"max_window"`
}

// MemoryConfig configures memory and SENSE behavior
type MemoryConfig struct {
	// Enable memory system
	Enabled bool `mapstructure:"enabled" toml:"enabled"`

	// Decay function: "linear", "exponential", "logarithmic"
	DecayFunction string `mapstructure:"decay_function" toml:"decay_function"`

	// Decay half-life (days)
	DecayHalfLife int `mapstructure:"decay_half_life" toml:"decay_half_life"`

	// Auto-sync specialist insights
	AutoSync bool `mapstructure:"auto_sync" toml:"auto_sync"`

	// Fact extraction provider
	FactProvider string `mapstructure:"fact_provider" toml:"fact_provider"`
}

// OptimizationConfig configures cost/speed tradeoffs
type OptimizationConfig struct {
	// Daily budget (USD)
	DailyBudget float64 `mapstructure:"daily_budget" toml:"daily_budget"`

	// Cost vs speed weighting (0-1 each, sum to 1)
	CostWeight  float64 `mapstructure:"cost_weight" toml:"cost_weight"`
	SpeedWeight float64 `mapstructure:"speed_weight" toml:"speed_weight"`

	// Use cheaper models when possible
	PreferCheap bool `mapstructure:"prefer_cheap" toml:"prefer_cheap"`

	// Batch similar tasks for optimization
	Batching struct {
		Enabled bool `mapstructure:"enabled" toml:"enabled"`
		Size    int  `mapstructure:"size" toml:"size"`
	} `mapstructure:"batching" toml:"batching"`
}

// PromptsConfig configures prompt variant selection
type PromptsConfig struct {
	// Default variant: "generic", "nextgen", "gpt5", "gemini3", "claude_thinking", "xs"
	Default string `mapstructure:"default" toml:"default"`

	// Override per provider
	PerProvider map[string]string `mapstructure:"per_provider" toml:"per_provider"`

	// Custom prompt templates
	Custom map[string]string `mapstructure:"custom" toml:"custom"`
}

// DebugConfig configures debug settings
type DebugConfig struct {
	// Enable debug logging
	Enabled bool `mapstructure:"enabled" toml:"enabled"`

	// Log level: "debug", "info", "warn", "error"
	LogLevel string `mapstructure:"log_level" toml:"log_level"`

	// Log file path
	LogFile string `mapstructure:"log_file" toml:"log_file"`

	// Verbose provider selection logging
	VerboseRouting bool `mapstructure:"verbose_routing" toml:"verbose_routing"`

	// Dump all API requests/responses
	DumpAPIs bool `mapstructure:"dump_apis" toml:"dump_apis"`
}

// AdvancedConfig for power user options
type AdvancedConfig struct {
	// Custom headers for API calls
	Headers map[string]string `mapstructure:"headers" toml:"headers"`

	// Custom timeouts per provider
	ProviderTimeouts map[string]int `mapstructure:"provider_timeouts" toml:"provider_timeouts"`

	// Retry policy
	RetryPolicy struct {
		MaxAttempts int           `mapstructure:"max_attempts" toml:"max_attempts"`
		BackoffBase time.Duration `mapstructure:"backoff_base" toml:"backoff_base"`
	} `mapstructure:"retry_policy" toml:"retry_policy"`

	// Custom routing rules (power user overrides)
	CustomRules []CustomRoutingRule `mapstructure:"custom_rules" toml:"custom_rules"`
}

// CustomRoutingRule allows power users to define custom routing
type CustomRoutingRule struct {
	// Match criteria
	Match struct {
		Intent   string `mapstructure:"intent" toml:"intent"`
		Pattern  string `mapstructure:"pattern" toml:"pattern"` // regex
		Provider string `mapstructure:"provider" toml:"provider"`
		Keywords string `mapstructure:"keywords" toml:"keywords"` // comma-separated
	} `mapstructure:"match" toml:"match"`

	// Action
	UseProvider string  `mapstructure:"use_provider" toml:"use_provider"`
	UseThinking bool    `mapstructure:"use_thinking" toml:"use_thinking"`
	MaxBudget   float64 `mapstructure:"max_budget" toml:"max_budget"`
}
