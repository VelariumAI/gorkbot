package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

// ConfigLoader handles loading and watching GORKBOT.md configuration
type ConfigLoader struct {
	path       string
	config     *GorkbotConfig
	mu         sync.RWMutex
	viper      *viper.Viper
	watchers   []func(*GorkbotConfig)
	logger     *slog.Logger
	stopWatch  chan struct{}
	watchError error
}

// NewConfigLoader creates a new configuration loader
func NewConfigLoader(path string, logger *slog.Logger) (*ConfigLoader, error) {
	if logger == nil {
		logger = slog.Default()
	}

	cl := &ConfigLoader{
		path:      path,
		logger:    logger,
		watchers:  make([]func(*GorkbotConfig), 0),
		stopWatch: make(chan struct{}),
	}

	// Load initial configuration
	if err := cl.Load(); err != nil {
		return nil, err
	}

	return cl, nil
}

// Load reads and parses GORKBOT.md configuration
func (cl *ConfigLoader) Load() error {
	// Resolve path - look in multiple locations if just filename
	configPath, err := cl.resolvePath()
	if err != nil {
		return err
	}

	// Create Viper instance
	v := viper.New()
	v.SetConfigFile(configPath)

	// Set file type based on extension
	ext := filepath.Ext(configPath)
	switch ext {
	case ".toml":
		v.SetConfigType("toml")
	case ".yaml", ".yml":
		v.SetConfigType("yaml")
	case ".json":
		v.SetConfigType("json")
	default:
		// Default to TOML for GORKBOT.md
		v.SetConfigType("toml")
	}

	// Enable environment variable substitution
	v.AutomaticEnv()

	// Read configuration
	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse into struct
	config := &GorkbotConfig{}
	if err := v.Unmarshal(config); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Validate configuration
	if err := ValidateConfig(config); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	// Apply defaults
	if err := ApplyDefaults(config); err != nil {
		return fmt.Errorf("failed to apply defaults: %w", err)
	}

	cl.mu.Lock()
	cl.config = config
	cl.viper = v
	cl.mu.Unlock()

	cl.logger.Info("configuration loaded", slog.String("path", configPath))
	return nil
}

// resolvePath finds the config file in standard locations
func (cl *ConfigLoader) resolvePath() (string, error) {
	// If path is absolute, use it directly
	if filepath.IsAbs(cl.path) {
		if _, err := os.Stat(cl.path); err == nil {
			return cl.path, nil
		}
		return "", fmt.Errorf("config file not found: %s", cl.path)
	}

	// Check common locations
	locations := []string{
		cl.path,
		filepath.Join(os.Getenv("HOME"), ".config", "gorkbot", cl.path),
		filepath.Join(os.Getenv("HOME"), ".gorkbot", cl.path),
		filepath.Join(".", cl.path),
	}

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			return loc, nil
		}
	}

	return "", fmt.Errorf("config file not found in any location")
}

// Get returns the current configuration (thread-safe)
func (cl *ConfigLoader) Get() *GorkbotConfig {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return cl.config
}

// Watch starts watching the config file for changes
func (cl *ConfigLoader) Watch() error {
	if cl.viper == nil {
		return errors.New("no configuration loaded yet")
	}

	cl.viper.OnConfigChange(func(e fsnotify.Event) {
		cl.logger.Info("config file changed", slog.String("path", e.Name))

		// Reload configuration
		if err := cl.Load(); err != nil {
			cl.mu.Lock()
			cl.watchError = err
			cl.mu.Unlock()
			cl.logger.Error("failed to reload config", slog.String("error", err.Error()))
			return
		}

		// Notify watchers
		config := cl.Get()
		for _, watcher := range cl.watchers {
			go watcher(config)
		}
	})

	cl.viper.WatchConfig()
	return nil
}

// StopWatch stops watching the config file
func (cl *ConfigLoader) StopWatch() {
	close(cl.stopWatch)
}

// OnChange registers a callback for config changes
func (cl *ConfigLoader) OnChange(callback func(*GorkbotConfig)) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cl.watchers = append(cl.watchers, callback)
}

// GetProvider returns configuration for a specific provider
func (cl *ConfigLoader) GetProvider(name string) (*ProviderConfig, error) {
	config := cl.Get()
	if config == nil {
		return nil, errors.New("no configuration loaded")
	}

	provider, ok := config.Providers[name]
	if !ok {
		return nil, fmt.Errorf("provider not configured: %s", name)
	}

	return &provider, nil
}

// GetAllProviders returns all configured providers
func (cl *ConfigLoader) GetAllProviders() map[string]ProviderConfig {
	config := cl.Get()
	if config == nil {
		return make(map[string]ProviderConfig)
	}
	return config.Providers
}

// IsProviderConfigured checks if a provider is configured
func (cl *ConfigLoader) IsProviderConfigured(name string) bool {
	config := cl.Get()
	if config == nil {
		return false
	}
	_, ok := config.Providers[name]
	return ok
}

// GetCapabilityProvider returns which provider handles a capability
func (cl *ConfigLoader) GetCapabilityProvider(capability string) (string, error) {
	config := cl.Get()
	if config == nil {
		return "", errors.New("no configuration loaded")
	}

	var provider string
	switch capability {
	case "thinking":
		provider = config.Capabilities.ThinkingProvider
	case "coding":
		provider = config.Capabilities.CodingProvider
	case "vision":
		provider = config.Capabilities.VisionProvider
	case "memory":
		provider = config.Capabilities.MemoryProvider
	case "specialist":
		provider = config.Capabilities.SpecialistProvider
	default:
		provider = config.Capabilities.DefaultProvider
	}

	if provider == "" {
		return "", fmt.Errorf("no provider configured for capability: %s", capability)
	}

	return provider, nil
}

// GetFallbackChain returns the fallback provider chain
func (cl *ConfigLoader) GetFallbackChain(provider string) []string {
	config := cl.Get()
	if config == nil {
		return []string{}
	}

	key := provider + "_fallback"
	return config.Routing.Fallback[key]
}

// ExportDefaults creates a default GORKBOT.md template
func ExportDefaults(outputPath string) error {
	template := `# Gorkbot v3.0 Configuration
# Configuration-driven, provider-agnostic, intelligently unified

# [providers] — Define all AI providers you want to use
[providers]

[providers.anthropic]
api_key = "${ANTHROPIC_API_KEY}"
model = "claude-opus-4"
timeout = 60
max_retries = 3
thinking.enabled = true
thinking.budget = 50000
vision.enabled = true
caching.enabled = true
caching.ttl = 3600
native_tools.enabled = true
cost_per_1m_input = 3.0
cost_per_1m_output = 15.0

[providers.openai]
api_key = "${OPENAI_API_KEY}"
model = "gpt-5"
timeout = 60
max_retries = 3
vision.enabled = true
native_tools.enabled = true
cost_per_1m_input = 5.0
cost_per_1m_output = 15.0

[providers.google]
api_key = "${GOOGLE_API_KEY}"
model = "gemini-2.0"
timeout = 60
max_retries = 3
vision.enabled = true
cost_per_1m_input = 1.25
cost_per_1m_output = 5.0

[providers.ollama]
base_url = "http://localhost:11434/api"
model = "llama2-70b"
timeout = 120
max_retries = 2
cost_per_1m_input = 0.0
cost_per_1m_output = 0.0

# [capabilities] — Which provider handles which capability
[capabilities]
thinking_provider = "anthropic"
coding_provider = "openai"
vision_provider = "google"
memory_provider = "anthropic"
specialist_provider = "anthropic"
default_provider = "anthropic"

# [routing] — Intent/file/directory routing rules
[routing]

[routing.intent_class]
refactoring = "anthropic"
optimization = "openai"
debugging = "anthropic"
testing = "openai"
architecture = "anthropic"

[routing.file_type]
"*.py" = "anthropic"
"*.js" = "openai"
"*.go" = "anthropic"
"*.rs" = "openai"

[routing.fallback]
anthropic_fallback = ["openai", "google", "ollama"]
openai_fallback = ["anthropic", "google"]
google_fallback = ["anthropic", "openai"]

[routing.cost_optimize]
budget_per_day = 50.0
cost_weight = 0.3
speed_weight = 0.7

# [specialist] — Autonomous delegation settings
[specialist]
provider = "anthropic"
thinking_budget = 100000
complexity_threshold = 8
files_threshold = 20
duration_threshold = 300
autonomy_level = "validated"
enabled = true

# [browser] — Browser automation settings
[browser]
enabled = true
headless = true
vision_provider = "google"
timeout = 30
screenshots.enabled = true
screenshots.quality = 80
screenshots.max_width = 1280

# [mcp] — Model Context Protocol settings
[mcp]
enabled = true
servers = ["jira", "aws", "github"]
auto_approve.enabled = true
auto_approve.tools = ["read_file", "list_directory"]

# [auto_repair] — Auto-fixing settings
[auto_repair]
enabled = true
linters = ["go vet", "tsc", "ruff"]
use_hashline = true
max_attempts = 3

# [context] — Context optimization
[context]
caching.enabled = true
caching.ttl = 3600
compression_strategy = "semantic"
compression_level = 5
use_ast = true
max_window = 150000

# [memory] — Memory and SENSE behavior
[memory]
enabled = true
decay_function = "exponential"
decay_half_life = 7
auto_sync = true
fact_provider = "anthropic"

# [optimization] — Cost/speed tradeoffs
[optimization]
daily_budget = 100.0
cost_weight = 0.3
speed_weight = 0.7
prefer_cheap = false
batching.enabled = true
batching.size = 5

# [prompts] — Prompt variant selection
[prompts]
default = "generic"
per_provider.anthropic = "claude_thinking"
per_provider.openai = "gpt5"
per_provider.google = "gemini3"

# [debug] — Debug settings
[debug]
enabled = false
log_level = "info"
log_file = "~/.config/gorkbot/gorkbot.log"
verbose_routing = false
dump_apis = false

# [advanced] — Power user options
[advanced]
headers = { "X-Custom-Header" = "value" }
provider_timeouts.anthropic = 60
provider_timeouts.openai = 60
provider_timeouts.google = 30
retry_policy.max_attempts = 3
retry_policy.backoff_base = 1000  # milliseconds
`

	return os.WriteFile(outputPath, []byte(template), 0644)
}

// ChangelogEntry represents a config change
type ChangelogEntry struct {
	Timestamp time.Time
	Path      string
	OldValue  interface{}
	NewValue  interface{}
}

// GetChangelog returns recent configuration changes (stub for future use)
func (cl *ConfigLoader) GetChangelog() []ChangelogEntry {
	// Future: Track configuration changes
	return []ChangelogEntry{}
}

// Reload reloads configuration from file
func (cl *ConfigLoader) Reload() error {
	cl.logger.Info("manually reloading configuration")
	return cl.Load()
}

// WatchError returns the last watch error, if any
func (cl *ConfigLoader) WatchError() error {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return cl.watchError
}
