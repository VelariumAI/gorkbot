package discord

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// StoredConfig is the on-disk Discord integration configuration.
//
// Security note: The bot token is intentionally absent from this file.
// It is always loaded from the DISCORD_BOT_TOKEN environment variable so
// that credentials are never written to disk or committed to version control.
type StoredConfig struct {
	Enabled       bool     `json:"enabled"`
	AllowedUsers  []string `json:"allowed_users,omitempty"`  // Snowflake IDs; empty = all users
	AllowedGuilds []string `json:"allowed_guilds,omitempty"` // Guild IDs; empty = all guilds
}

const configFilename = "discord.json"

// LoadConfig reads the Discord config from configDir/discord.json.
// Returns a zero-value config (Enabled: false) when the file does not exist.
func LoadConfig(configDir string) (StoredConfig, error) {
	var cfg StoredConfig
	data, err := os.ReadFile(filepath.Join(configDir, configFilename))
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	return cfg, json.Unmarshal(data, &cfg)
}

// SaveConfig writes cfg to configDir/discord.json with 0600 permissions.
func SaveConfig(configDir string, cfg StoredConfig) error {
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(configDir, configFilename), data, 0600)
}
