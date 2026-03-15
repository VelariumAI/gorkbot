package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/internal/platform"
)

type handler struct {
	ws *WebhookServer
}

// verifyHMAC validates "sha256=<hex>" style signatures (GitHub format).
// Returns true if secret is empty (unauthenticated mode) or the signature matches.
func verifyHMAC(secret string, body []byte, sigHeader string) bool {
	if secret == "" {
		return true
	}
	const prefix = "sha256="
	if !strings.HasPrefix(sigHeader, prefix) {
		return false
	}
	gotHex := strings.TrimPrefix(sigHeader, prefix)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(gotHex), []byte(expected))
}

// handleGitHub processes GitHub webhook events.
func (h *handler) handleGitHub(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MB limit
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	if !verifyHMAC(h.ws.secret, body, r.Header.Get("X-Hub-Signature-256")) {
		h.ws.logger.Warn("webhook: GitHub HMAC verification failed", "remote", r.RemoteAddr)
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	event := r.Header.Get("X-GitHub-Event")
	if event == "" {
		event = "unknown"
	}

	prompt := buildGitHubPrompt(event, json.RawMessage(body))
	w.WriteHeader(http.StatusAccepted)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		result, err := h.ws.runner(ctx, "github", event, prompt)
		if err != nil {
			h.ws.logger.Error("webhook: GitHub handler error", "event", event, "err", err)
			result = fmt.Sprintf("Error processing GitHub %s event: %v", event, err)
		}
		if h.ws.notify != nil {
			h.ws.notify(result)
		}
	}()
}

// handleGeneric processes freeform webhook posts.
// Expected body: {"prompt": "..."} or plain text.
func (h *handler) handleGeneric(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	if !verifyHMAC(h.ws.secret, body, r.Header.Get("X-Webhook-Signature")) {
		h.ws.logger.Warn("webhook: generic HMAC verification failed", "remote", r.RemoteAddr)
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	prompt := strings.TrimSpace(string(body))
	// Try to parse as JSON {"prompt": "..."}
	var payload struct {
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && payload.Prompt != "" {
		prompt = payload.Prompt
	}
	if prompt == "" {
		http.Error(w, "empty prompt", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusAccepted)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		result, err := h.ws.runner(ctx, "generic", "message", prompt)
		if err != nil {
			h.ws.logger.Error("webhook: generic handler error", "err", err)
			result = fmt.Sprintf("Error: %v", err)
		}
		if h.ws.notify != nil {
			h.ws.notify(result)
		}
	}()
}

// handleHealth returns a simple health check response.
func (h *handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok","version":%q}`, platform.Version)
}
