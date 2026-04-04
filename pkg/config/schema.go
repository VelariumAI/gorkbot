package config

// Config represents the complete application configuration
type Config struct {
	ConfigVersion int                 `mapstructure:"config_version" yaml:"config_version"`
	Models        []ModelConfig       `mapstructure:"models" yaml:"models"`
	Tools         []ToolConfig        `mapstructure:"tools" yaml:"tools"`
	ToolGroups    []ToolGroup         `mapstructure:"tool_groups" yaml:"tool_groups"`
	Sandbox       SandboxConfig       `mapstructure:"sandbox" yaml:"sandbox"`
	Skills        SkillsConfig        `mapstructure:"skills" yaml:"skills"`
	Memory        MemoryConfig        `mapstructure:"memory" yaml:"memory"`
	Summarization SummarizationConfig `mapstructure:"summarization" yaml:"summarization"`
	Subagents     SubagentsConfig     `mapstructure:"subagents" yaml:"subagents"`
	Channels      ChannelsConfig      `mapstructure:"channels" yaml:"channels"`
	Guardrails    GuardrailsConfig    `mapstructure:"guardrails" yaml:"guardrails"`
}

// ModelConfig represents a single LLM model configuration
type ModelConfig struct {
	Name             string                 `mapstructure:"name" yaml:"name"`
	DisplayName      string                 `mapstructure:"display_name" yaml:"display_name"`
	Use              string                 `mapstructure:"use" yaml:"use"` // Reflection path, e.g., "pkg.models:OpenAIChatModel"
	Model            string                 `mapstructure:"model" yaml:"model"`
	APIKey           string                 `mapstructure:"api_key" yaml:"api_key"` // May start with $ENVVAR
	MaxTokens        int                    `mapstructure:"max_tokens" yaml:"max_tokens"`
	Temperature      float64                `mapstructure:"temperature" yaml:"temperature"`
	SupportsThinking bool                   `mapstructure:"supports_thinking" yaml:"supports_thinking"`
	SupportsVision   bool                   `mapstructure:"supports_vision" yaml:"supports_vision"`
	UseResponsesAPI  bool                   `mapstructure:"use_responses_api" yaml:"use_responses_api"`
	OutputVersion    string                 `mapstructure:"output_version" yaml:"output_version"`
	BaseURL          string                 `mapstructure:"base_url" yaml:"base_url"`
	CustomFields     map[string]interface{} `mapstructure:",remain" yaml:",inline"`
}

// ToolConfig represents a tool configuration
type ToolConfig struct {
	Use     string `mapstructure:"use" yaml:"use"` // Reflection path
	Group   string `mapstructure:"group" yaml:"group"`
	Enabled bool   `mapstructure:"enabled" yaml:"enabled"`
}

// ToolGroup represents a logical grouping of tools
type ToolGroup struct {
	Name        string   `mapstructure:"name" yaml:"name"`
	Description string   `mapstructure:"description" yaml:"description"`
	Tools       []string `mapstructure:"tools" yaml:"tools"`
}

// SandboxConfig represents sandbox execution configuration
type SandboxConfig struct {
	Enabled      bool                   `mapstructure:"enabled" yaml:"enabled"`
	Use          string                 `mapstructure:"use" yaml:"use"` // Reflection path, e.g., "pkg.sandbox.local:LocalSandboxProvider"
	CustomFields map[string]interface{} `mapstructure:",remain" yaml:",inline"`
}

// SkillsConfig represents skills system configuration
type SkillsConfig struct {
	Path          string `mapstructure:"path" yaml:"path"`                     // Host path to skills directory
	ContainerPath string `mapstructure:"container_path" yaml:"container_path"` // Virtual path in agent context
}

// MemoryConfig represents memory system configuration
type MemoryConfig struct {
	Enabled                 bool    `mapstructure:"enabled" yaml:"enabled"`
	InjectionEnabled        bool    `mapstructure:"injection_enabled" yaml:"injection_enabled"`
	StoragePath             string  `mapstructure:"storage_path" yaml:"storage_path"`
	DebounceSeconds         int     `mapstructure:"debounce_seconds" yaml:"debounce_seconds"`
	ModelName               string  `mapstructure:"model_name" yaml:"model_name"` // Empty = use default
	MaxFacts                int     `mapstructure:"max_facts" yaml:"max_facts"`
	FactConfidenceThreshold float64 `mapstructure:"fact_confidence_threshold" yaml:"fact_confidence_threshold"`
	MaxInjectionTokens      int     `mapstructure:"max_injection_tokens" yaml:"max_injection_tokens"`
}

// SummarizationConfig represents context summarization configuration
type SummarizationConfig struct {
	Enabled               bool             `mapstructure:"enabled" yaml:"enabled"`
	Trigger               interface{}      `mapstructure:"trigger" yaml:"trigger"` // Can be list or single
	Keep                  TriggerCondition `mapstructure:"keep" yaml:"keep"`
	TrimTokensToSummarize int              `mapstructure:"trim_tokens_to_summarize" yaml:"trim_tokens_to_summarize"`
	SummaryPrompt         string           `mapstructure:"summary_prompt" yaml:"summary_prompt"`
	ModelName             string           `mapstructure:"model_name" yaml:"model_name"`
}

// TriggerCondition represents a trigger condition for summarization
type TriggerCondition struct {
	Type      string `mapstructure:"type" yaml:"type"`           // "tokens", "messages", "fraction"
	Threshold int    `mapstructure:"threshold" yaml:"threshold"` // For tokens
	Keep      int    `mapstructure:"keep" yaml:"keep"`           // Messages/items to keep
}

// SubagentsConfig represents subagent system configuration
type SubagentsConfig struct {
	Enabled        bool `mapstructure:"enabled" yaml:"enabled"`
	MaxConcurrent  int  `mapstructure:"max_concurrent" yaml:"max_concurrent"`
	TimeoutSeconds int  `mapstructure:"timeout_seconds" yaml:"timeout_seconds"`
}

// ChannelsConfig represents IM channel configuration
type ChannelsConfig struct {
	LanggraphURL string                `mapstructure:"langgraph_url" yaml:"langgraph_url"`
	GatewayURL   string                `mapstructure:"gateway_url" yaml:"gateway_url"`
	Session      ChannelSessionConfig  `mapstructure:"session" yaml:"session"`
	Feishu       FeishuChannelConfig   `mapstructure:"feishu" yaml:"feishu"`
	Slack        SlackChannelConfig    `mapstructure:"slack" yaml:"slack"`
	Telegram     TelegramChannelConfig `mapstructure:"telegram" yaml:"telegram"`
}

// ChannelSessionConfig represents default session configuration for channels
type ChannelSessionConfig struct {
	AssistantID string                 `mapstructure:"assistant_id" yaml:"assistant_id"`
	Config      map[string]interface{} `mapstructure:"config" yaml:"config"`
	Context     map[string]interface{} `mapstructure:"context" yaml:"context"`
}

// FeishuChannelConfig represents Feishu (Lark) channel configuration
type FeishuChannelConfig struct {
	Enabled   bool   `mapstructure:"enabled" yaml:"enabled"`
	AppID     string `mapstructure:"app_id" yaml:"app_id"`
	AppSecret string `mapstructure:"app_secret" yaml:"app_secret"`
}

// SlackChannelConfig represents Slack channel configuration
type SlackChannelConfig struct {
	Enabled  bool   `mapstructure:"enabled" yaml:"enabled"`
	BotToken string `mapstructure:"bot_token" yaml:"bot_token"`
	AppToken string `mapstructure:"app_token" yaml:"app_token"`
}

// TelegramChannelConfig represents Telegram channel configuration
type TelegramChannelConfig struct {
	Enabled  bool   `mapstructure:"enabled" yaml:"enabled"`
	BotToken string `mapstructure:"bot_token" yaml:"bot_token"`
}

// GuardrailsConfig represents guardrails system configuration
type GuardrailsConfig struct {
	Enabled      bool                   `mapstructure:"enabled" yaml:"enabled"`
	Use          string                 `mapstructure:"use" yaml:"use"` // Reflection path
	CustomFields map[string]interface{} `mapstructure:",remain" yaml:",inline"`
}

// Current config version - bump when schema changes
const CurrentConfigVersion = 2

// Default values
const (
	DefaultMemoryDebounceSeconds     = 30
	DefaultMemoryMaxFacts            = 100
	DefaultMemoryConfidenceThreshold = 0.7
	DefaultMemoryMaxInjectionTokens  = 2000
	DefaultSubagentsMaxConcurrent    = 3
	DefaultSubagentsTimeoutSeconds   = 900 // 15 minutes
	DefaultSkillsContainerPath       = "/mnt/skills"
)

// GetModelConfig returns the model configuration for the given name
func (c *Config) GetModelConfig(name string) *ModelConfig {
	for i := range c.Models {
		if c.Models[i].Name == name {
			return &c.Models[i]
		}
	}
	return nil
}

// GetDefaultModel returns the first model (default)
func (c *Config) GetDefaultModel() *ModelConfig {
	if len(c.Models) > 0 {
		return &c.Models[0]
	}
	return nil
}

// GetToolsByGroup returns tools filtered by group
func (c *Config) GetToolsByGroup(group string) []ToolConfig {
	var tools []ToolConfig
	for _, tool := range c.Tools {
		if tool.Group == group && tool.Enabled {
			tools = append(tools, tool)
		}
	}
	return tools
}

// GetEnabledTools returns all enabled tools
func (c *Config) GetEnabledTools() []ToolConfig {
	var tools []ToolConfig
	for _, tool := range c.Tools {
		if tool.Enabled {
			tools = append(tools, tool)
		}
	}
	return tools
}
