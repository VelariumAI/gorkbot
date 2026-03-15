// Package auth implements Google OAuth 2.0 Device Authorization Grant (RFC 8628)
// for headless/terminal environments such as Termux on Android.
//
// The Device Flow is the correct OAuth grant type for CLI tools: it requires
// no local web server redirect and no embedded client secret that would need
// constant rotation. Users approve access in any browser, then return to the
// terminal.
//
// Token lifecycle:
//   - Access tokens expire after 1 hour (Google default).
//   - Refresh tokens are long-lived and stored encrypted via KeyManager.
//   - EnsureToken() is idempotent: it validates, refreshes, or re-initiates
//     the device flow as needed before returning a usable access token.
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/velariumai/gorkbot/pkg/security"
)

// ── Constants ────────────────────────────────────────────────────────────────

const (
	// googleDeviceURL is the RFC 8628 device authorization endpoint.
	googleDeviceURL = "https://oauth2.googleapis.com/device/code"
	// googleTokenURL handles both device-code polling and refresh-token exchange.
	googleTokenURL = "https://oauth2.googleapis.com/token"
	// googleRevokeURL revokes a token.
	googleRevokeURL = "https://oauth2.googleapis.com/revoke"

	// defaultPollInterval is the minimum recommended polling interval (Google
	// returns its own interval; we use this as a floor).
	defaultPollInterval = 5 * time.Second
	// deviceCodeExpiry is the default device-code lifetime Google issues.
	defaultDeviceCodeExpiry = 30 * time.Minute

	// tokenFileName is the encrypted token store filename inside configDir.
	tokenFileName = "google_oauth_tokens.enc"
	// clientConfigFileName holds the OAuth client_id (never the secret, since
	// Device Flow for installed apps does not require one).
	clientConfigFileName = "google_client.json"
)

// ── Types ─────────────────────────────────────────────────────────────────────

// TokenSet holds the complete OAuth token response, serialised to disk.
type TokenSet struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type"`
	Scope        string    `json:"scope"`
}

// Valid returns true when the access token exists and is not yet expired.
// A 30-second buffer is applied to avoid races at the margin.
func (t *TokenSet) Valid() bool {
	if t.AccessToken == "" {
		return false
	}
	return time.Now().Add(30 * time.Second).Before(t.ExpiresAt)
}

// Refreshable returns true when a refresh_token is available.
func (t *TokenSet) Refreshable() bool {
	return t.RefreshToken != ""
}

// ClientConfig holds the minimal OAuth client registration data.
// For installed applications, only client_id is strictly required for
// Device Flow — no client_secret is needed (Google issues a "public"
// client for installed apps).
type ClientConfig struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"` // optional, may be empty
}

// DeviceCodeResponse is the initial response from the device authorization endpoint.
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// tokenResponse is the raw token endpoint response (handles both success and error).
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

// ── Client ────────────────────────────────────────────────────────────────────

// Client manages the full Google OAuth token lifecycle for a set of API scopes.
//
// Usage:
//
//	client, _ := auth.NewGoogleClient(configDir, []string{"https://www.googleapis.com/auth/drive.readonly"}, logger)
//	token, authRequired, instructions, err := client.EnsureToken(ctx)
//	if authRequired {
//	    // Surface instructions string to the user in the TUI.
//	}
type Client struct {
	configDir string
	scopes    []string
	keyMgr    *security.KeyManager
	httpCl    *http.Client
	logger    *slog.Logger
	mu        sync.Mutex
}

// NewGoogleClient creates a Client for the given OAuth scopes.
// configDir is the application's config directory (same as platform.HAL ConfigDir).
// logger may be nil (defaults to slog.Default()).
func NewGoogleClient(configDir string, scopes []string, logger *slog.Logger) (*Client, error) {
	if logger == nil {
		logger = slog.Default()
	}
	keyMgr, err := security.NewKeyManager(configDir)
	if err != nil {
		return nil, fmt.Errorf("auth: create key manager: %w", err)
	}
	return &Client{
		configDir: configDir,
		scopes:    scopes,
		keyMgr:    keyMgr,
		httpCl:    &http.Client{Timeout: 30 * time.Second},
		logger:    logger,
	}, nil
}

// ── Token persistence ─────────────────────────────────────────────────────────

func (c *Client) tokenPath() string { return filepath.Join(c.configDir, tokenFileName) }

// loadToken reads and decrypts the stored TokenSet. Returns nil (no error) if no token exists.
func (c *Client) loadToken() (*TokenSet, error) {
	enc, err := os.ReadFile(c.tokenPath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("auth: read token file: %w", err)
	}
	plain, err := c.keyMgr.Decrypt(strings.TrimSpace(string(enc)))
	if err != nil {
		// Corrupted or key-mismatch — treat as no token.
		c.logger.Warn("auth: token decryption failed, clearing", "err", err)
		_ = os.Remove(c.tokenPath())
		return nil, nil
	}
	var t TokenSet
	if err := json.Unmarshal([]byte(plain), &t); err != nil {
		return nil, fmt.Errorf("auth: parse token: %w", err)
	}
	return &t, nil
}

// saveToken encrypts and persists a TokenSet.
func (c *Client) saveToken(t *TokenSet) error {
	data, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("auth: marshal token: %w", err)
	}
	enc, err := c.keyMgr.Encrypt(string(data))
	if err != nil {
		return fmt.Errorf("auth: encrypt token: %w", err)
	}
	if err := os.MkdirAll(c.configDir, 0o700); err != nil {
		return fmt.Errorf("auth: mkdir configDir: %w", err)
	}
	return os.WriteFile(c.tokenPath(), []byte(enc), 0o600)
}

// ── Client config ─────────────────────────────────────────────────────────────

func (c *Client) clientConfigPath() string { return filepath.Join(c.configDir, clientConfigFileName) }

// loadClientConfig reads the client_id from disk. Returns a helpful error if
// the config does not exist yet.
func (c *Client) loadClientConfig() (*ClientConfig, error) {
	data, err := os.ReadFile(c.clientConfigPath())
	if os.IsNotExist(err) {
		return nil, fmt.Errorf(
			"auth: Google client config not found at %s\n"+
				"Run: /auth notebooklm setup   to configure your Google OAuth client",
			c.clientConfigPath(),
		)
	}
	if err != nil {
		return nil, fmt.Errorf("auth: read client config: %w", err)
	}
	var cfg ClientConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("auth: parse client config: %w", err)
	}
	if cfg.ClientID == "" {
		return nil, fmt.Errorf("auth: client_id is empty in %s", c.clientConfigPath())
	}
	return &cfg, nil
}

// SaveClientConfig writes ClientConfig to disk (called during first-use setup).
func (c *Client) SaveClientConfig(cfg ClientConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("auth: marshal client config: %w", err)
	}
	if err := os.MkdirAll(c.configDir, 0o700); err != nil {
		return fmt.Errorf("auth: mkdir: %w", err)
	}
	return os.WriteFile(c.clientConfigPath(), data, 0o600)
}

// ── EnsureToken ───────────────────────────────────────────────────────────────

// EnsureTokenResult conveys the outcome of EnsureToken.
type EnsureTokenResult struct {
	Token         *TokenSet
	AuthRequired  bool   // true when user interaction is needed
	Instructions  string // human-readable auth steps (set when AuthRequired)
	DeviceCode    string // set during device flow — caller should poll CompleteDeviceFlow
	PollInterval  time.Duration
}

// EnsureToken returns a valid access token, refreshing or initiating the Device
// Flow as needed. When AuthRequired is true, the caller must display Instructions
// to the user and then call CompleteDeviceFlow to await completion.
func (c *Client) EnsureToken(ctx context.Context) (EnsureTokenResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	tok, err := c.loadToken()
	if err != nil {
		return EnsureTokenResult{}, err
	}

	// Case 1: valid token — return immediately.
	if tok != nil && tok.Valid() {
		c.logger.Debug("auth: token valid, using cached")
		return EnsureTokenResult{Token: tok}, nil
	}

	// Case 2: expired but refreshable.
	if tok != nil && tok.Refreshable() {
		c.logger.Info("auth: access token expired, refreshing")
		refreshed, err := c.refreshToken(ctx, tok)
		if err == nil {
			if err := c.saveToken(refreshed); err != nil {
				c.logger.Warn("auth: could not save refreshed token", "err", err)
			}
			return EnsureTokenResult{Token: refreshed}, nil
		}
		c.logger.Warn("auth: refresh failed, initiating device flow", "err", err)
	}

	// Case 3: no token or refresh failed — start device flow.
	cfg, err := c.loadClientConfig()
	if err != nil {
		// No client config: return setup instructions.
		return EnsureTokenResult{
			AuthRequired: true,
			Instructions: c.setupInstructions(),
		}, nil
	}

	dcResp, err := c.requestDeviceCode(ctx, cfg)
	if err != nil {
		return EnsureTokenResult{}, fmt.Errorf("auth: device code request: %w", err)
	}

	pollInterval := time.Duration(dcResp.Interval) * time.Second
	if pollInterval < defaultPollInterval {
		pollInterval = defaultPollInterval
	}

	instructions := fmt.Sprintf(
		"**Google Authentication Required**\n\n"+
			"1. Visit: **%s**\n"+
			"2. Enter code: **%s**\n"+
			"3. Sign in with your Google account and grant access.\n\n"+
			"Waiting for authorization… (run `/auth notebooklm poll` to check)\n"+
			"Code expires in %d minutes.",
		dcResp.VerificationURL,
		dcResp.UserCode,
		dcResp.ExpiresIn/60,
	)

	return EnsureTokenResult{
		AuthRequired: true,
		Instructions: instructions,
		DeviceCode:   dcResp.DeviceCode,
		PollInterval: pollInterval,
	}, nil
}

// CompleteDeviceFlow polls the token endpoint until the user completes auth or
// ctx is cancelled. It saves the resulting token and returns it.
func (c *Client) CompleteDeviceFlow(ctx context.Context, deviceCode string, interval time.Duration) (*TokenSet, error) {
	cfg, err := c.loadClientConfig()
	if err != nil {
		return nil, err
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			tok, pending, err := c.pollDeviceToken(ctx, cfg, deviceCode)
			if err != nil {
				return nil, err
			}
			if pending {
				continue
			}
			c.mu.Lock()
			saveErr := c.saveToken(tok)
			c.mu.Unlock()
			if saveErr != nil {
				c.logger.Warn("auth: could not save device flow token", "err", saveErr)
			}
			return tok, nil
		}
	}
}

// RevokeToken revokes and deletes the stored token.
func (c *Client) RevokeToken(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	tok, err := c.loadToken()
	if err != nil || tok == nil {
		return nil // nothing to revoke
	}

	// Best-effort revoke at Google's endpoint.
	body := url.Values{"token": {tok.AccessToken}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, googleRevokeURL, strings.NewReader(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpCl.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}

	return os.Remove(c.tokenPath())
}

// Status returns a formatted one-liner describing the current token state.
func (c *Client) Status() string {
	tok, err := c.loadToken()
	if err != nil {
		return "⚠️  token read error"
	}
	if tok == nil {
		return "❌ not authenticated"
	}
	if tok.Valid() {
		remaining := time.Until(tok.ExpiresAt).Truncate(time.Minute)
		return fmt.Sprintf("✅ authenticated (expires in %s)", remaining)
	}
	if tok.Refreshable() {
		return "🔄 token expired, will auto-refresh on next use"
	}
	return "❌ token expired, re-authentication required"
}

// GetAccessToken returns the raw access token string if valid, else "".
// This is a non-blocking read suitable for injecting into subprocess environments.
func (c *Client) GetAccessToken() string {
	tok, err := c.loadToken()
	if err != nil || tok == nil {
		return ""
	}
	if tok.Valid() {
		return tok.AccessToken
	}
	return ""
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

func (c *Client) requestDeviceCode(ctx context.Context, cfg *ClientConfig) (*DeviceCodeResponse, error) {
	params := url.Values{
		"client_id": {cfg.ClientID},
		"scope":     {strings.Join(c.scopes, " ")},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleDeviceURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpCl.Do(req)
	if err != nil {
		return nil, fmt.Errorf("device code request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device code endpoint: %d %s", resp.StatusCode, body)
	}

	var dcr DeviceCodeResponse
	if err := json.Unmarshal(body, &dcr); err != nil {
		return nil, fmt.Errorf("parse device code response: %w", err)
	}
	return &dcr, nil
}

// pollDeviceToken polls once. Returns (token, stillPending, error).
func (c *Client) pollDeviceToken(ctx context.Context, cfg *ClientConfig, deviceCode string) (*TokenSet, bool, error) {
	params := url.Values{
		"client_id":   {cfg.ClientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}
	if cfg.ClientSecret != "" {
		params.Set("client_secret", cfg.ClientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpCl.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var tr tokenResponse
	_ = json.Unmarshal(body, &tr)

	switch tr.Error {
	case "":
		// Success.
	case "authorization_pending":
		return nil, true, nil
	case "slow_down":
		// Google asks us to slow down; wait an extra interval cycle.
		return nil, true, nil
	case "access_denied":
		return nil, false, fmt.Errorf("auth: user denied access")
	case "expired_token":
		return nil, false, fmt.Errorf("auth: device code expired; restart authentication")
	default:
		return nil, false, fmt.Errorf("auth: token error %q: %s", tr.Error, tr.ErrorDesc)
	}

	tok := &TokenSet{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
		TokenType:    tr.TokenType,
		Scope:        tr.Scope,
	}
	return tok, false, nil
}

func (c *Client) refreshToken(ctx context.Context, tok *TokenSet) (*TokenSet, error) {
	cfg, err := c.loadClientConfig()
	if err != nil {
		return nil, err
	}

	params := url.Values{
		"client_id":     {cfg.ClientID},
		"refresh_token": {tok.RefreshToken},
		"grant_type":    {"refresh_token"},
	}
	if cfg.ClientSecret != "" {
		params.Set("client_secret", cfg.ClientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpCl.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("parse refresh response: %w", err)
	}
	if tr.Error != "" {
		return nil, fmt.Errorf("refresh error %q: %s", tr.Error, tr.ErrorDesc)
	}

	// Google may not return a new refresh_token; keep the old one.
	newRefresh := tr.RefreshToken
	if newRefresh == "" {
		newRefresh = tok.RefreshToken
	}
	return &TokenSet{
		AccessToken:  tr.AccessToken,
		RefreshToken: newRefresh,
		ExpiresAt:    time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
		TokenType:    tr.TokenType,
		Scope:        tok.Scope,
	}, nil
}

// ── Setup instructions ────────────────────────────────────────────────────────

func (c *Client) setupInstructions() string {
	return fmt.Sprintf(
		"**Google OAuth Setup Required**\n\n"+
			"To use Google Drive sources in NotebookLM, you need a Google Cloud OAuth client.\n\n"+
			"**Steps:**\n"+
			"1. Go to https://console.cloud.google.com/apis/credentials\n"+
			"2. Click **\"Create Credentials\"** → **\"OAuth 2.0 Client ID\"**\n"+
			"3. Application type: **\"Desktop app\"** (Installed application)\n"+
			"4. Click **Create** → note your client_id and client_secret\n"+
			"5. Run: `/auth notebooklm setup <client_id> [client_secret]`\n\n"+
			"   Or create the file manually at:\n"+
			"   **%s**\n\n"+
			"   Contents: {\"client_id\": \"YOUR_CLIENT_ID.apps.googleusercontent.com\"}\n\n"+
			"6. Run `/auth notebooklm login` to authenticate.\n\n"+
			"**Note:** Enable the Google Drive API at:\n"+
			"https://console.cloud.google.com/apis/library/drive.googleapis.com",
		c.clientConfigPath(),
	)
}

// ── Scopes ────────────────────────────────────────────────────────────────────

// DriveReadOnlyScope is the OAuth scope for reading Google Drive files.
const DriveReadOnlyScope = "https://www.googleapis.com/auth/drive.readonly"

// DocsReadOnlyScope is the OAuth scope for reading Google Docs content.
const DocsReadOnlyScope = "https://www.googleapis.com/auth/documents.readonly"

// NotebookLMScopes returns the recommended scopes for NotebookLM Drive integration.
func NotebookLMScopes() []string {
	return []string{DriveReadOnlyScope, DocsReadOnlyScope}
}
