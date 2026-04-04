package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// DiscordConnector provides Discord webhook integration for receiving and sending messages.
type DiscordConnector struct {
	webhookURL   string
	listenPort   int
	httpClient   *http.Client
	messageChan  chan *Message
	stopChan     chan struct{}
	logger       *slog.Logger
	httpServer   *http.Server
	authorToken  string // Optional: for bot API calls (not just webhooks)
}

// NewDiscordConnector creates a new Discord connector.
func NewDiscordConnector(webhookURL string, listenPort int, logger *slog.Logger) *DiscordConnector {
	if logger == nil {
		logger = slog.Default()
	}

	return &DiscordConnector{
		webhookURL:  webhookURL,
		listenPort:  listenPort,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		messageChan: make(chan *Message, 100),
		stopChan:    make(chan struct{}),
		logger:      logger,
	}
}

// Name returns "discord".
func (dc *DiscordConnector) Name() string {
	return "discord"
}

// Start begins listening for Discord webhook events.
func (dc *DiscordConnector) Start(ctx context.Context) error {
	dc.logger.Info("starting Discord connector", "listen_port", dc.listenPort)

	// Setup HTTP server for webhook
	mux := http.NewServeMux()
	mux.HandleFunc("/discord/webhook", dc.handleWebhook)

	dc.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", dc.listenPort),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Start server in goroutine
	go func() {
		dc.logger.Info("Discord webhook server listening", "addr", dc.httpServer.Addr)
		if err := dc.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			dc.logger.Error("Discord webhook server error", "error", err)
		}
	}()

	return nil
}

// Stop gracefully shuts down the Discord connector.
func (dc *DiscordConnector) Stop(ctx context.Context) error {
	dc.logger.Info("stopping Discord connector")
	if dc.httpServer != nil {
		dc.httpServer.Shutdown(ctx)
	}
	close(dc.stopChan)
	return nil
}

// Send sends a message via Discord webhook.
func (dc *DiscordConnector) Send(ctx context.Context, resp *Response) error {
	payload := map[string]interface{}{
		"content": resp.Text,
	}

	// Add embed if metadata contains embed info
	if embed, ok := resp.Metadata["embed"]; ok {
		var embedData interface{}
		json.Unmarshal([]byte(embed), &embedData)
		payload["embeds"] = []interface{}{embedData}
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	httpResp, err := dc.httpClient.Post(dc.webhookURL, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		dc.logger.Error("failed to send Discord message", "error", err)
		return err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return fmt.Errorf("discord webhook error: status %d", httpResp.StatusCode)
	}

	dc.logger.Info("sent Discord message")
	return nil
}

// MessageChan returns the channel for incoming messages.
func (dc *DiscordConnector) MessageChan() <-chan *Message {
	return dc.messageChan
}

// IsHealthy checks if the Discord webhook is accessible.
func (dc *DiscordConnector) IsHealthy(ctx context.Context) bool {
	// Simple health check: try to send a test message
	testPayload := map[string]string{"content": "health_check"}
	jsonData, _ := json.Marshal(testPayload)

	httpResp, err := dc.httpClient.Post(dc.webhookURL, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		dc.logger.Error("discord health check failed", "error", err)
		return false
	}
	defer httpResp.Body.Close()

	return httpResp.StatusCode >= 200 && httpResp.StatusCode < 300
}

// handleWebhook processes incoming Discord webhook events.
func (dc *DiscordConnector) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var webhookEvent struct {
		Type int `json:"type"`
		Data struct {
			Content string `json:"content"`
			Author  struct {
				ID       string `json:"id"`
				Username string `json:"username"`
			} `json:"author"`
			ChannelID string `json:"channel_id"`
			ID        string `json:"id"`
			Timestamp string `json:"timestamp"`
		} `json:"data"`
	}

	if err := json.NewDecoder(r.Body).Decode(&webhookEvent); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	// Type 0 = PING (Discord interaction), Type 1 = MESSAGE_CREATE
	if webhookEvent.Type == 0 {
		// Respond to PING
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"type": 1})
		return
	}

	if webhookEvent.Type != 1 || webhookEvent.Data.Content == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Parse timestamp
	timestamp, _ := time.Parse(time.RFC3339, webhookEvent.Data.Timestamp)

	msg := &Message{
		ID:        webhookEvent.Data.ID,
		Source:    "discord",
		SourceID:  webhookEvent.Data.Author.ID,
		Username:  webhookEvent.Data.Author.Username,
		Text:      webhookEvent.Data.Content,
		Timestamp: timestamp,
		Metadata: map[string]string{
			"channel_id": webhookEvent.Data.ChannelID,
			"author_id":  webhookEvent.Data.Author.ID,
		},
	}

	select {
	case dc.messageChan <- msg:
		dc.logger.Info("received Discord message", "from", msg.Username, "text_len", len(msg.Text))
		w.WriteHeader(http.StatusOK)
	case <-time.After(5 * time.Second):
		dc.logger.Warn("message channel full, dropping webhook event")
		http.Error(w, "server busy", http.StatusServiceUnavailable)
	}
}
