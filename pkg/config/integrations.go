// Package config provides persistent configuration helpers.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const integrationsFile = "integrations.json"

// IntegrationSettings stores configurable integration env vars on disk so they
// survive across sessions without requiring manual .env edits.
//
// Keys are the canonical env var names (e.g. "BUDGET_SESSION_USD").
// An empty string means "not configured" (uses OS env var if set).
type IntegrationSettings struct {
	Values map[string]string `json:"values"`
}

// IntegrationKeys is the canonical list of env vars managed by this package,
// in the order shown in the Settings UI.
var IntegrationKeys = []struct {
	Key         string
	Label       string
	Description string
	Sensitive   bool // mask value in UI
}{
	{
		Key:         "BUDGET_SESSION_USD",
		Label:       "Session Budget (USD)",
		Description: "Block spending above this per session (0 = off)",
	},
	{
		Key:         "BUDGET_DAILY_USD",
		Label:       "Daily Budget (USD)",
		Description: "Block spending above this per 24h (0 = off)",
	},
	{
		Key:         "WEBHOOK_PORT",
		Label:       "Webhook Port",
		Description: "HTTP port for incoming webhooks (empty = disabled)",
	},
	{
		Key:         "WEBHOOK_SECRET",
		Label:       "Webhook HMAC Secret",
		Description: "HMAC-SHA256 secret for GitHub webhook verification",
		Sensitive:   true,
	},
	{
		Key:         "WEBHOOK_NOTIFY_DISCORD",
		Label:       "Webhook → Discord Channel",
		Description: "Discord channel ID to post webhook results into",
	},
	{
		Key:         "WEBHOOK_NOTIFY_TELEGRAM",
		Label:       "Webhook → Telegram Chat",
		Description: "Telegram chat ID to post webhook results into",
	},
	{
		Key:         "SCHEDULER_NOTIFY_DISCORD",
		Label:       "Scheduler → Discord Channel",
		Description: "Discord channel ID for scheduled task results",
	},
	{
		Key:         "SCHEDULER_NOTIFY_TELEGRAM",
		Label:       "Scheduler → Telegram Chat",
		Description: "Telegram chat ID for scheduled task results",
	},
}

// LoadIntegrations reads integrations.json from configDir.
// Returns an empty settings struct (not an error) when the file does not exist.
func LoadIntegrations(configDir string) (*IntegrationSettings, error) {
	s := &IntegrationSettings{Values: make(map[string]string)}
	path := filepath.Join(configDir, integrationsFile)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(data, s)
}

// Save writes the settings to configDir/integrations.json atomically.
func (s *IntegrationSettings) Save(configDir string) error {
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(configDir, integrationsFile), data, 0600)
}

// Apply sets each non-empty value as an env var, but only when the env var is
// not already set in the process environment (env takes precedence over config).
func (s *IntegrationSettings) Apply() {
	for k, v := range s.Values {
		if v != "" && os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}

// Get returns the current effective value: OS env var if set, else stored value.
func (s *IntegrationSettings) Get(key string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return s.Values[key]
}

// Set updates the in-memory value, persists to disk, and applies via os.Setenv.
func (s *IntegrationSettings) Set(configDir, key, value string) error {
	if s.Values == nil {
		s.Values = make(map[string]string)
	}
	s.Values[key] = value
	// Apply immediately so in-process code sees the change.
	os.Setenv(key, value)
	return s.Save(configDir)
}
