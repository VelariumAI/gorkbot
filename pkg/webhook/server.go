// Package webhook implements an HTTP ingestion server that lets external systems
// (GitHub, CI/CD, monitoring, Zapier, etc.) trigger agent actions via POST.
package webhook

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// WebhookRunner is called for each incoming event. source is e.g. "github",
// eventType is e.g. "push". Returns the agent's response.
type WebhookRunner func(ctx context.Context, source, eventType, prompt string) (string, error)

// NotifyFunc posts a result string to an external channel (Discord, Telegram, etc.).
type NotifyFunc func(result string)

// WebhookServer listens on addr and dispatches POST events to runner.
type WebhookServer struct {
	addr    string
	secret  string // HMAC-SHA256 secret; "" = unauthenticated
	mux     *http.ServeMux
	httpSrv *http.Server
	runner  WebhookRunner
	notify  NotifyFunc
	logger  *slog.Logger
}

// NewWebhookServer constructs a WebhookServer. secret may be empty to disable
// HMAC verification (not recommended in production).
func NewWebhookServer(addr, secret string, runner WebhookRunner, notify NotifyFunc, logger *slog.Logger) *WebhookServer {
	if logger == nil {
		logger = slog.Default()
	}
	ws := &WebhookServer{
		addr:   addr,
		secret: secret,
		runner: runner,
		notify: notify,
		logger: logger,
		mux:    http.NewServeMux(),
	}

	h := &handler{ws: ws}
	ws.mux.HandleFunc("/webhook/github", h.handleGitHub)
	ws.mux.HandleFunc("/webhook/generic", h.handleGeneric)
	ws.mux.HandleFunc("/webhook/health", h.handleHealth)

	ws.httpSrv = &http.Server{
		Addr:         addr,
		Handler:      ws.mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}
	return ws
}

// Start begins listening. Returns immediately; the server runs in a goroutine.
func (ws *WebhookServer) Start(ctx context.Context) error {
	go func() {
		ws.logger.Info("webhook server starting", "addr", ws.addr)
		if err := ws.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			ws.logger.Error("webhook server error", "err", err)
		}
	}()
	go func() {
		<-ctx.Done()
		_ = ws.Stop()
	}()
	return nil
}

// Stop shuts down the HTTP server gracefully.
func (ws *WebhookServer) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ws.logger.Info("webhook server stopping")
	return ws.httpSrv.Shutdown(ctx)
}
