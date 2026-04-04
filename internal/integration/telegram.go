package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

// TelegramConnector provides Telegram Bot API integration.
type TelegramConnector struct {
	botToken      string
	apiURL        string
	httpClient    *http.Client
	messageChan   chan *Message
	stopChan      chan struct{}
	logger        *slog.Logger
	pollInterval  time.Duration
	offset        int64 // For getUpdates long polling
}

// NewTelegramConnector creates a new Telegram connector.
func NewTelegramConnector(botToken string, logger *slog.Logger) *TelegramConnector {
	if logger == nil {
		logger = slog.Default()
	}

	return &TelegramConnector{
		botToken:     botToken,
		apiURL:       "https://api.telegram.org/bot" + botToken,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		messageChan:  make(chan *Message, 100),
		stopChan:     make(chan struct{}),
		logger:       logger,
		pollInterval: 5 * time.Second,
	}
}

// Name returns "telegram".
func (tc *TelegramConnector) Name() string {
	return "telegram"
}

// Start begins polling for messages from Telegram.
func (tc *TelegramConnector) Start(ctx context.Context) error {
	tc.logger.Info("starting Telegram connector", "bot_token", maskToken(tc.botToken))

	// Verify bot is accessible
	if err := tc.IsHealthy(ctx); !err {
		return fmt.Errorf("telegram bot not accessible")
	}

	// Start polling goroutine
	go tc.pollMessages(ctx)
	return nil
}

// Stop gracefully shuts down the Telegram connector.
func (tc *TelegramConnector) Stop(ctx context.Context) error {
	tc.logger.Info("stopping Telegram connector")
	close(tc.stopChan)
	return nil
}

// Send sends a message back to a Telegram user.
func (tc *TelegramConnector) Send(ctx context.Context, resp *Response) error {
	data := url.Values{
		"chat_id": {resp.SourceID},
		"text":    {resp.Text},
	}

	// Add parse_mode from metadata if provided
	if parseMode, ok := resp.Metadata["parse_mode"]; ok {
		data.Set("parse_mode", parseMode)
	}

	reqURL := tc.apiURL + "/sendMessage?" + data.Encode()
	httpResp, err := tc.httpClient.Post(reqURL, "application/x-www-form-urlencoded", nil)
	if err != nil {
		tc.logger.Error("failed to send Telegram message", "error", err, "chat_id", resp.SourceID)
		return err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API error: status %d", httpResp.StatusCode)
	}

	tc.logger.Info("sent Telegram message", "chat_id", resp.SourceID)
	return nil
}

// MessageChan returns the channel for incoming messages.
func (tc *TelegramConnector) MessageChan() <-chan *Message {
	return tc.messageChan
}

// IsHealthy checks if the Telegram bot is accessible.
func (tc *TelegramConnector) IsHealthy(ctx context.Context) bool {
	reqURL := tc.apiURL + "/getMe"
	httpResp, err := tc.httpClient.Get(reqURL)
	if err != nil {
		tc.logger.Error("telegram health check failed", "error", err)
		return false
	}
	defer httpResp.Body.Close()
	return httpResp.StatusCode == http.StatusOK
}

// pollMessages continuously polls Telegram for new messages.
func (tc *TelegramConnector) pollMessages(ctx context.Context) {
	ticker := time.NewTicker(tc.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-tc.stopChan:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			tc.fetchUpdates()
		}
	}
}

// fetchUpdates retrieves updates from Telegram using getUpdates.
func (tc *TelegramConnector) fetchUpdates() {
	data := url.Values{
		"offset": {fmt.Sprintf("%d", tc.offset)},
		"timeout": {"10"}, // Long polling timeout
	}

	reqURL := tc.apiURL + "/getUpdates?" + data.Encode()
	httpResp, err := tc.httpClient.Get(reqURL)
	if err != nil {
		tc.logger.Error("failed to fetch Telegram updates", "error", err)
		return
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		tc.logger.Error("telegram getUpdates error", "status", httpResp.StatusCode)
		return
	}

	var apiResp struct {
		OK     bool `json:"ok"`
		Result []struct {
			UpdateID int64 `json:"update_id"`
			Message  struct {
				MessageID int64 `json:"message_id"`
				Chat      struct {
					ID int64 `json:"id"`
				} `json:"chat"`
				From struct {
					Username string `json:"username"`
				} `json:"from"`
				Date int64  `json:"date"`
				Text string `json:"text"`
			} `json:"message"`
		} `json:"result"`
	}

	if err := json.NewDecoder(httpResp.Body).Decode(&apiResp); err != nil {
		tc.logger.Error("failed to decode Telegram response", "error", err)
		return
	}

	if !apiResp.OK {
		return // No updates or error
	}

	for _, update := range apiResp.Result {
		if update.Message.Text == "" {
			continue // Skip non-text messages
		}

		msg := &Message{
			ID:        fmt.Sprintf("tg_%d", update.Message.MessageID),
			Source:    "telegram",
			SourceID:  fmt.Sprintf("%d", update.Message.Chat.ID),
			Username:  update.Message.From.Username,
			Text:      update.Message.Text,
			Timestamp: time.Unix(update.Message.Date, 0),
			Metadata: map[string]string{
				"message_id": fmt.Sprintf("%d", update.Message.MessageID),
				"chat_id":    fmt.Sprintf("%d", update.Message.Chat.ID),
			},
		}

		select {
		case tc.messageChan <- msg:
			tc.logger.Info("received Telegram message", "from", msg.Username, "text_len", len(msg.Text))
		case <-time.After(5 * time.Second):
			tc.logger.Warn("message channel full, dropping update")
		}

		// Update offset for next poll
		tc.offset = update.UpdateID + 1
	}
}

// maskToken returns a masked version of the bot token for logging.
func maskToken(token string) string {
	if len(token) < 10 {
		return "***"
	}
	return token[:5] + "..." + token[len(token)-3:]
}
