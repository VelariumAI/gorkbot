package vcseclient

import "time"

// Config controls VCSE client behavior.
type Config struct {
	BaseURL string
	Timeout time.Duration
	Enabled bool
}

// DefaultConfig returns default local VCSE settings.
func DefaultConfig() Config {
	return Config{
		BaseURL: "http://127.0.0.1:8000",
		Timeout: 250 * time.Millisecond,
		Enabled: false,
	}
}
