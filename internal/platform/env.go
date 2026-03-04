package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Version of the Gorkbot application
const Version = "3.5.3"

// EnvConfig holds the system-specific paths.
type EnvConfig struct {
	ConfigDir string
	LogDir    string
	OS        string
	Arch      string
	IsTermux  bool
}

// GetEnvConfig returns a struct with resolved paths for the current environment.
func GetEnvConfig() (*EnvConfig, error) {
	config := &EnvConfig{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}

	// 1. Detect Termux (Android)
	if os.Getenv("TERMUX_VERSION") != "" {
		config.IsTermux = true
	} else {
		// Fallback check
		if _, err := os.Stat("/data/data/com.termux/files/usr/bin/login"); err == nil {
			config.IsTermux = true
		}
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve user home directory: %w", err)
	}

	// 2. Resolve Paths based on OS/Environment
	switch {
	case config.IsTermux:
		// Termux typically stores user configs relative to home.
		config.ConfigDir = filepath.Join(homeDir, ".config", "gorkbot")
		config.LogDir = filepath.Join(homeDir, ".gorkbot", "logs")

	case config.OS == "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(homeDir, "AppData", "Roaming")
		}
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			localAppData = filepath.Join(homeDir, "AppData", "Local")
		}
		config.ConfigDir = filepath.Join(appData, "gorkbot")
		config.LogDir = filepath.Join(localAppData, "gorkbot", "logs")

	case config.OS == "darwin": // macOS
		config.ConfigDir = filepath.Join(homeDir, "Library", "Application Support", "gorkbot")
		config.LogDir = filepath.Join(homeDir, "Library", "Logs", "gorkbot")

	default: // Linux/Unix standard (XDG)
		configHome := os.Getenv("XDG_CONFIG_HOME")
		if configHome == "" {
			configHome = filepath.Join(homeDir, ".config")
		}
		dataHome := os.Getenv("XDG_DATA_HOME")
		if dataHome == "" {
			dataHome = filepath.Join(homeDir, ".local", "share")
		}
		config.ConfigDir = filepath.Join(configHome, "gorkbot")
		config.LogDir = filepath.Join(dataHome, "gorkbot", "logs")
	}

	// 3. Ensure Directories Exist
	if err := os.MkdirAll(config.ConfigDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config dir: %w", err)
	}
	if err := os.MkdirAll(config.LogDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log dir: %w", err)
	}

	return config, nil
}
