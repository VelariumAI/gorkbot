package telegram

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// StoredConfig is the persisted Telegram configuration
type StoredConfig struct {
	Enabled      bool    `json:"enabled"`
	Token        string  `json:"token"`
	AllowedUsers []int64 `json:"allowed_users,omitempty"`
}

// LoadConfig reads the Telegram config from configDir/telegram.json
func LoadConfig(configDir string) (StoredConfig, error) {
	var cfg StoredConfig
	path := filepath.Join(configDir, "telegram.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	err = json.Unmarshal(data, &cfg)
	return cfg, err
}

// SaveConfig writes the Telegram config to configDir/telegram.json
func SaveConfig(configDir string, cfg StoredConfig) error {
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(configDir, "telegram.json"), data, 0600)
}
