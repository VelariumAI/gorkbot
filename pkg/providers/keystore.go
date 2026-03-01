// Package providers manages API key storage and provider lifecycle for all AI backends.
package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/velariumai/gorkbot/pkg/ai"
)

// Provider constants identify the supported AI providers.
const (
	ProviderXAI       = "xai"
	ProviderGoogle    = "google"
	ProviderAnthropic = "anthropic"
	ProviderOpenAI    = "openai"
	ProviderMiniMax    = "minimax"
	ProviderOpenRouter = "openrouter"
)

// allProviders is the canonical order of providers.
var allProviders = []string{
	ProviderXAI,
	ProviderGoogle,
	ProviderAnthropic,
	ProviderOpenAI,
	ProviderMiniMax,
	ProviderOpenRouter,
}

// envVarNames maps provider ID → env var name.
var envVarNames = map[string]string{
	ProviderXAI:       "XAI_API_KEY",
	ProviderGoogle:    "GEMINI_API_KEY",
	ProviderAnthropic: "ANTHROPIC_API_KEY",
	ProviderOpenAI:    "OPENAI_API_KEY",
	ProviderMiniMax:    "MINIMAX_API_KEY",
	ProviderOpenRouter: "OPENROUTER_API_KEY",
}

// KeyStatus indicates the validation state of an API key.
type KeyStatus int

const (
	KeyStatusMissing    KeyStatus = iota // No key configured
	KeyStatusUnverified                  // Key set but not yet validated
	KeyStatusValid                       // Key validated successfully
	KeyStatusInvalid                     // Key validated and rejected
)

func (ks KeyStatus) String() string {
	switch ks {
	case KeyStatusValid:
		return "valid"
	case KeyStatusInvalid:
		return "invalid"
	case KeyStatusUnverified:
		return "unverified"
	default:
		return "missing"
	}
}

// ProviderKey holds a single provider's key and its validation status.
type ProviderKey struct {
	Key       string    `json:"key"`
	Status    KeyStatus `json:"status"`
	LastCheck time.Time `json:"last_check,omitempty"`
}

// ProviderStatus is a lightweight snapshot used for UI rendering.
type ProviderStatus struct {
	Provider string
	Status   KeyStatus
}

// updateChan sends provider status change notifications.
type updateChan = chan string

// KeyStore persists API keys to disk and seeds from environment variables.
type KeyStore struct {
	mu      sync.RWMutex
	keys    map[string]ProviderKey
	path    string // ~/.config/gorkbot/api_keys.json
	updates []updateChan
}

type keyStoreFile struct {
	Keys map[string]ProviderKey `json:"keys"`
}

// NewKeyStore creates a KeyStore, loading from disk and seeding from env vars.
func NewKeyStore(configDir string) *KeyStore {
	ks := &KeyStore{
		keys: make(map[string]ProviderKey),
		path: filepath.Join(configDir, "api_keys.json"),
	}
	ks.load()
	ks.seedFromEnv()
	return ks
}

// load reads persisted keys from disk (best-effort; ignores missing file).
func (ks *KeyStore) load() {
	data, err := os.ReadFile(ks.path)
	if err != nil {
		return
	}
	var f keyStoreFile
	if err := json.Unmarshal(data, &f); err != nil {
		return
	}
	if f.Keys != nil {
		ks.keys = f.Keys
	}
}

// seedFromEnv fills in any missing keys from environment variables.
// Env vars take precedence only when the key is not already in the store.
func (ks *KeyStore) seedFromEnv() {
	for provider, envVar := range envVarNames {
		val := os.Getenv(envVar)
		if val == "" {
			continue
		}
		existing, ok := ks.keys[provider]
		if !ok || existing.Key == "" {
			ks.keys[provider] = ProviderKey{Key: val, Status: KeyStatusUnverified}
		}
	}
}

// save persists current keys to disk at 0600.
func (ks *KeyStore) save() error {
	if err := os.MkdirAll(filepath.Dir(ks.path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(keyStoreFile{Keys: ks.keys}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ks.path, data, 0600)
}

// Get returns the key and status for the given provider.
func (ks *KeyStore) Get(provider string) (string, KeyStatus) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	pk, ok := ks.keys[provider]
	if !ok {
		return "", KeyStatusMissing
	}
	return pk.Key, pk.Status
}

// GetKey returns just the API key for the given provider (implements discovery.KeyGetter).
func (ks *KeyStore) GetKey(provider string) string {
	key, _ := ks.Get(provider)
	return key
}

// Set stores a new key for the provider (marks as Unverified) and saves to disk.
func (ks *KeyStore) Set(provider, key string) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	ks.keys[provider] = ProviderKey{Key: key, Status: KeyStatusUnverified}
	if err := ks.save(); err != nil {
		return fmt.Errorf("keystore save failed: %w", err)
	}
	ks.notifyLocked(provider)
	return nil
}

// SetStatus updates the status of an existing key (e.g., after validation).
func (ks *KeyStore) SetStatus(provider string, status KeyStatus) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	pk := ks.keys[provider]
	pk.Status = status
	pk.LastCheck = time.Now()
	ks.keys[provider] = pk
	_ = ks.save()
	ks.notifyLocked(provider)
}

// Validate pings the provider to check whether the key is valid.
// It updates and persists the status as Valid or Invalid.
func (ks *KeyStore) Validate(ctx context.Context, provider string, prov ai.AIProvider, logger *slog.Logger) error {
	key, _ := ks.Get(provider)
	if key == "" {
		return fmt.Errorf("no key set for provider %q", provider)
	}
	err := prov.Ping(ctx)
	if err != nil {
		ks.SetStatus(provider, KeyStatusInvalid)
		if logger != nil {
			logger.Warn("keystore: key validation failed", "provider", provider, "error", err)
		}
		return fmt.Errorf("key invalid: %w", err)
	}
	ks.SetStatus(provider, KeyStatusValid)
	if logger != nil {
		logger.Info("keystore: key validated", "provider", provider)
	}
	return nil
}

// StatusLine returns all providers with their current status.
func (ks *KeyStore) StatusLine() []ProviderStatus {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	var out []ProviderStatus
	for _, p := range allProviders {
		pk, ok := ks.keys[p]
		if !ok {
			out = append(out, ProviderStatus{Provider: p, Status: KeyStatusMissing})
		} else {
			out = append(out, ProviderStatus{Provider: p, Status: pk.Status})
		}
	}
	return out
}

// FormatStatus returns a human-readable status summary.
func (ks *KeyStore) FormatStatus() string {
	var sb strings.Builder
	for _, ps := range ks.StatusLine() {
		icon := "✗"
		switch ps.Status {
		case KeyStatusValid:
			icon = "●"
		case KeyStatusUnverified:
			icon = "?"
		}
		sb.WriteString(fmt.Sprintf("  %-12s %s %s\n", ps.Provider, icon, ps.Status))
	}
	return sb.String()
}

// Subscribe returns a channel that receives provider names whenever a key changes.
func (ks *KeyStore) Subscribe() chan string {
	ch := make(chan string, 5)
	ks.mu.Lock()
	ks.updates = append(ks.updates, ch)
	ks.mu.Unlock()
	return ch
}

// notifyLocked sends the provider name to all update subscribers (call with lock held).
func (ks *KeyStore) notifyLocked(provider string) {
	for _, ch := range ks.updates {
		select {
		case ch <- provider:
		default:
		}
	}
}

// AllProviders returns the canonical list of provider IDs.
func AllProviders() []string {
	return append([]string(nil), allProviders...)
}
