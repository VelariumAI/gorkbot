package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

var (
	// Global config instance
	globalConfig *Config
	configMutex  sync.RWMutex

	// Config file watcher
	lastModTime time.Time
	configPath  string
)

// LoadConfig loads or reloads the configuration from file
func LoadConfig(path string) (*Config, error) {
	configMutex.Lock()
	defer configMutex.Unlock()

	if path == "" {
		path = findConfigPath()
	}
	if path == "" {
		return nil, fmt.Errorf("no config file found - please create config.yaml")
	}

	configPath = path

	// Create viper instance
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	// Enable environment variable reading
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	// Record file modification time
	if fi, err := os.Stat(path); err == nil {
		lastModTime = fi.ModTime()
	}

	// Unmarshal into struct
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Apply defaults
	cfg.applyDefaults()

	// Resolve environment variables in credentials
	cfg.resolveEnvVars()

	globalConfig = &cfg
	return &cfg, nil
}

// GetConfig returns the currently loaded global config
func GetConfig() *Config {
	configMutex.RLock()
	defer configMutex.RUnlock()

	if globalConfig == nil {
		panic("config not loaded - call LoadConfig() first")
	}
	return globalConfig
}

// ReloadIfChanged checks if config file has changed and reloads it
func ReloadIfChanged() (*Config, error) {
	configMutex.RLock()
	currentPath := configPath
	configMutex.RUnlock()

	if currentPath == "" {
		return GetConfig(), nil
	}

	// Check file modification time
	fi, err := os.Stat(currentPath)
	if err != nil {
		return GetConfig(), nil // File may have been temporarily unavailable
	}

	if fi.ModTime().After(lastModTime) {
		// File has been modified, reload
		return LoadConfig(currentPath)
	}

	return GetConfig(), nil
}

// WatchConfigFile starts a background goroutine that watches for config file changes
func WatchConfigFile(onReload func(*Config)) {
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			cfg, err := ReloadIfChanged()
			if err != nil {
				// Log error but continue watching
				continue
			}

			if onReload != nil && cfg != GetConfig() {
				onReload(cfg)
			}
		}
	}()
}

// findConfigPath searches for config file in standard locations
func findConfigPath() string {
	locations := []string{
		"config.yaml",
		"./config.yaml",
	}

	// Add home directory location
	if homeDir, err := os.UserHomeDir(); err == nil {
		locations = append(locations,
			filepath.Join(homeDir, ".gorkbot", "config.yaml"),
			filepath.Join(homeDir, ".config", "gorkbot", "config.yaml"),
		)
	}

	// Add project root locations (walk up from current directory)
	if pwd, err := os.Getwd(); err == nil {
		locations = append(locations,
			filepath.Join(pwd, "config.yaml"),
			filepath.Join(filepath.Dir(pwd), "config.yaml"),
			filepath.Join(filepath.Dir(filepath.Dir(pwd)), "config.yaml"),
		)
	}

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			return loc
		}
	}

	return ""
}

// applyDefaults applies default values to config if not specified
func (c *Config) applyDefaults() {
	if c.ConfigVersion == 0 {
		c.ConfigVersion = CurrentConfigVersion
	}

	// Memory defaults
	if c.Memory.DebounceSeconds == 0 {
		c.Memory.DebounceSeconds = DefaultMemoryDebounceSeconds
	}
	if c.Memory.MaxFacts == 0 {
		c.Memory.MaxFacts = DefaultMemoryMaxFacts
	}
	if c.Memory.FactConfidenceThreshold == 0 {
		c.Memory.FactConfidenceThreshold = DefaultMemoryConfidenceThreshold
	}
	if c.Memory.MaxInjectionTokens == 0 {
		c.Memory.MaxInjectionTokens = DefaultMemoryMaxInjectionTokens
	}

	// Subagent defaults
	if c.Subagents.MaxConcurrent == 0 {
		c.Subagents.MaxConcurrent = DefaultSubagentsMaxConcurrent
	}
	if c.Subagents.TimeoutSeconds == 0 {
		c.Subagents.TimeoutSeconds = DefaultSubagentsTimeoutSeconds
	}

	// Skills defaults
	if c.Skills.ContainerPath == "" {
		c.Skills.ContainerPath = DefaultSkillsContainerPath
	}

	// Channel defaults
	if c.Channels.LanggraphURL == "" {
		c.Channels.LanggraphURL = "http://localhost:2024"
	}
	if c.Channels.GatewayURL == "" {
		c.Channels.GatewayURL = "http://localhost:8001"
	}

	// Enable tools by default if not specified
	for i := range c.Tools {
		if !c.Tools[i].Enabled {
			c.Tools[i].Enabled = true
		}
	}
}

// resolveEnvVars resolves environment variable references in config
// Values starting with $ are treated as environment variable names
func (c *Config) resolveEnvVars() {
	// Resolve model API keys
	for i := range c.Models {
		if strings.HasPrefix(c.Models[i].APIKey, "$") {
			envVar := strings.TrimPrefix(c.Models[i].APIKey, "$")
			c.Models[i].APIKey = os.Getenv(envVar)
		}
	}

	// Resolve channel tokens
	if strings.HasPrefix(c.Channels.Feishu.AppSecret, "$") {
		envVar := strings.TrimPrefix(c.Channels.Feishu.AppSecret, "$")
		c.Channels.Feishu.AppSecret = os.Getenv(envVar)
	}
	if strings.HasPrefix(c.Channels.Slack.BotToken, "$") {
		envVar := strings.TrimPrefix(c.Channels.Slack.BotToken, "$")
		c.Channels.Slack.BotToken = os.Getenv(envVar)
	}
	if strings.HasPrefix(c.Channels.Slack.AppToken, "$") {
		envVar := strings.TrimPrefix(c.Channels.Slack.AppToken, "$")
		c.Channels.Slack.AppToken = os.Getenv(envVar)
	}
	if strings.HasPrefix(c.Channels.Telegram.BotToken, "$") {
		envVar := strings.TrimPrefix(c.Channels.Telegram.BotToken, "$")
		c.Channels.Telegram.BotToken = os.Getenv(envVar)
	}

	// Resolve guardrails provider secret if present
	for key, val := range c.Guardrails.CustomFields {
		if str, ok := val.(string); ok && strings.HasPrefix(str, "$") {
			envVar := strings.TrimPrefix(str, "$")
			c.Guardrails.CustomFields[key] = os.Getenv(envVar)
		}
	}
}

// SaveConfig writes the current configuration to YAML.
func SaveConfig(cfg *Config, path string) error {
	if path == "" {
		path = configPath
	}
	if path == "" {
		return fmt.Errorf("no config path specified")
	}

	if cfg == nil {
		return fmt.Errorf("config is nil")
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// ValidateConfig checks if config is valid
func ValidateConfig(cfg *Config) error {
	// Check required fields
	if len(cfg.Models) == 0 {
		return fmt.Errorf("at least one model must be configured")
	}

	for _, model := range cfg.Models {
		if model.Name == "" {
			return fmt.Errorf("model name cannot be empty")
		}
		if model.Use == "" {
			return fmt.Errorf("model 'use' path cannot be empty for model %q", model.Name)
		}
		if model.APIKey == "" {
			return fmt.Errorf("model %q requires api_key (can be env var like $OPENAI_API_KEY)", model.Name)
		}
	}

	// Check config version compatibility
	if cfg.ConfigVersion > CurrentConfigVersion {
		return fmt.Errorf("config version %d is newer than supported version %d", cfg.ConfigVersion, CurrentConfigVersion)
	}

	return nil
}
