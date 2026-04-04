package cache

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	geminiCacheBase   = "https://generativelanguage.googleapis.com/v1beta"
	geminiCacheMinTTL = 5 * time.Minute  // minimum sensible TTL
	geminiCacheTTL    = 60 * time.Minute // default 1-hour TTL (Gemini default)
)

// geminiCachedContent is the request body for POST /v1beta/cachedContents.
type geminiCachedContent struct {
	Model             string          `json:"model"`
	Contents          []geminiContent `json:"contents,omitempty"`
	SystemInstruction *geminiSysInst  `json:"systemInstruction,omitempty"`
	TTL               string          `json:"ttl,omitempty"`
	DisplayName       string          `json:"displayName,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiSysInst struct {
	Parts []geminiPart `json:"parts"`
}

// geminiCacheResponse is the minimal response from the cachedContents API.
type geminiCacheResponse struct {
	Name       string `json:"name"`       // "cachedContents/{id}"
	ExpireTime string `json:"expireTime"` // RFC 3339
}

// GeminiCacheClient manages the Gemini CachedContent lifecycle for one session.
// It creates the cache on the first call that clears the token minimum,
// refreshes the TTL when it would expire before the next turn, and deletes it
// on Close(). Safe for concurrent use.
type GeminiCacheClient struct {
	mu         sync.Mutex
	apiKey     string
	model      string
	name       string    // "cachedContents/{id}", empty until created
	expiresAt  time.Time // local estimate of server-side expiry
	httpClient *http.Client
}

// NewGeminiCacheClient creates a client for the given API key and model.
// The model string should match the Gemini model used for generation
// (e.g. "gemini-2.5-flash"). A cache created for one model cannot be
// referenced from a different model.
func NewGeminiCacheClient(apiKey, model string) *GeminiCacheClient {
	// Normalise model name to the "models/{id}" format Gemini requires.
	if !strings.HasPrefix(model, "models/") {
		model = "models/" + model
	}
	return &GeminiCacheClient{
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// CurrentName returns the cached content resource name, or "" if not yet
// created. Must be called with the advisor mutex held (via Advisor methods).
func (g *GeminiCacheClient) CurrentName() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.name
}

// SetName stores a resource name received from the API.
func (g *GeminiCacheClient) SetName(name string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.name = name
	g.expiresAt = time.Now().Add(geminiCacheTTL)
}

// Invalidate clears the stored cache name so the next turn creates a new one.
// Called when the system prompt has changed.
func (g *GeminiCacheClient) Invalidate() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.name = ""
	g.expiresAt = time.Time{}
}

// Create uploads systemPrompt as a Gemini CachedContent and stores the
// returned resource name. Returns ("", nil) silently when the content is below
// the model-specific token minimum so callers need not handle that case.
//
// Minimum token floors by model (from Gemini docs, 2025):
//   - gemini-2.5-flash / gemini-2.5-pro: 1 024
//   - gemini-3.x:                        4 096 (pro preview)
func (g *GeminiCacheClient) Create(ctx context.Context, systemPrompt string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Skip if already cached and not expired.
	if g.name != "" && time.Now().Before(g.expiresAt.Add(-geminiCacheMinTTL)) {
		return g.name, nil
	}

	// Rough token floor check (1 024 tokens ≈ 4 096 chars).
	if len(systemPrompt) < 4096 {
		return "", nil // below minimum; skip silently
	}

	body := geminiCachedContent{
		Model: g.model,
		SystemInstruction: &geminiSysInst{
			Parts: []geminiPart{{Text: systemPrompt}},
		},
		TTL:         fmt.Sprintf("%.0fs", geminiCacheTTL.Seconds()),
		DisplayName: "gorkbot-session-cache",
	}

	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("gemini cache: marshal: %w", err)
	}

	url := fmt.Sprintf("%s/cachedContents?key=%s", geminiCacheBase, g.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("gemini cache: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini cache: POST: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		// Non-fatal: log and continue without caching.
		slog.Warn("gemini cachedContents create failed",
			"status", resp.StatusCode,
			"body", string(raw))
		return "", nil
	}

	var result geminiCacheResponse
	if err := json.Unmarshal(raw, &result); err != nil || result.Name == "" {
		return "", nil // malformed response; degrade gracefully
	}

	g.name = result.Name
	g.expiresAt = time.Now().Add(geminiCacheTTL)
	slog.Info("Gemini cache created", "name", g.name, "ttl", geminiCacheTTL)
	return g.name, nil
}

// RefreshIfNeeded extends the TTL of the current cache entry when it will
// expire within the next 10 minutes. Should be called at the start of each
// turn to prevent stale cache references mid-session.
func (g *GeminiCacheClient) RefreshIfNeeded(ctx context.Context) {
	g.mu.Lock()
	name := g.name
	expires := g.expiresAt
	g.mu.Unlock()

	if name == "" || time.Now().Before(expires.Add(-10*time.Minute)) {
		return // not created yet, or still fresh
	}

	// PATCH the TTL to reset the expiry clock.
	body := map[string]string{"ttl": fmt.Sprintf("%.0fs", geminiCacheTTL.Seconds())}
	data, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/%s?key=%s&updateMask=ttl", geminiCacheBase, name, g.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(data))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		g.mu.Lock()
		g.expiresAt = time.Now().Add(geminiCacheTTL)
		g.mu.Unlock()
		slog.Debug("Gemini cache TTL refreshed", "name", name)
	}
}

// Delete removes the cached content from Gemini's servers. Called on session
// end to avoid leaving orphaned entries (they cost storage).
func (g *GeminiCacheClient) Delete(ctx context.Context) {
	g.mu.Lock()
	name := g.name
	g.name = ""
	g.mu.Unlock()

	if name == "" {
		return
	}

	url := fmt.Sprintf("%s/%s?key=%s", geminiCacheBase, name, g.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return
	}
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
	slog.Info("Gemini cache deleted", "name", name)
}
