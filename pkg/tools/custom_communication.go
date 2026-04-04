package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"
)

type SendEmailTool struct {
	BaseTool
}

func NewSendEmailTool() *SendEmailTool {
	return &SendEmailTool{
		BaseTool: BaseTool{
			name:               "send_email",
			description:        "Sends an email using standard SMTP (if configured).",
			category:           CategoryCommunication,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *SendEmailTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"to":            map[string]interface{}{"type": "string", "description": "Recipient address"},
			"subject":       map[string]interface{}{"type": "string", "description": "Email subject"},
			"body":          map[string]interface{}{"type": "string", "description": "Email body"},
			"smtp_host":     map[string]interface{}{"type": "string", "description": "SMTP hostname (optional, falls back to SMTP_HOST env var)"},
			"smtp_port":     map[string]interface{}{"type": "number", "description": "SMTP port (optional, default 587 or SMTP_PORT env var)"},
			"smtp_username": map[string]interface{}{"type": "string", "description": "SMTP username (optional, falls back to SMTP_USERNAME env var)"},
			"smtp_password": map[string]interface{}{"type": "string", "description": "SMTP password (optional, falls back to SMTP_PASSWORD env var)"},
			"from":          map[string]interface{}{"type": "string", "description": "Sender address (optional, falls back to SMTP_FROM or smtp_username)"},
		},
		"required": []string{"to", "subject", "body"},
	}
	b, _ := json.Marshal(schema)
	return b
}

func (t *SendEmailTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	to := commGetStringParam(params, "to")
	subject := commGetStringParam(params, "subject")
	body := commGetStringParam(params, "body")
	if to == "" || subject == "" || body == "" {
		return &ToolResult{Success: false, Error: "to, subject, and body are required"}, nil
	}

	smtpHost := getParamOrEnv(params, "smtp_host", "SMTP_HOST")
	smtpPort := getIntParam(params, "smtp_port", parseEnvInt("SMTP_PORT", 587))
	smtpUser := getParamOrEnv(params, "smtp_username", "SMTP_USERNAME")
	smtpPass := getParamOrEnv(params, "smtp_password", "SMTP_PASSWORD")
	fromAddr := getParamOrEnv(params, "from", "SMTP_FROM")
	if fromAddr == "" {
		fromAddr = smtpUser
	}

	if smtpHost == "" || smtpUser == "" || smtpPass == "" || fromAddr == "" {
		return &ToolResult{
			Success: false,
			Error:   "SMTP configuration is incomplete (set SMTP_HOST, SMTP_USERNAME, SMTP_PASSWORD, and SMTP_FROM or pass parameters)",
		}, nil
	}
	if smtpPort <= 0 {
		return &ToolResult{Success: false, Error: "smtp_port must be greater than 0"}, nil
	}
	if !strings.Contains(to, "@") || !strings.Contains(fromAddr, "@") {
		return &ToolResult{Success: false, Error: "invalid email address in to/from"}, nil
	}

	msg := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s",
		fromAddr, to, subject, body,
	)
	addr := net.JoinHostPort(smtpHost, strconv.Itoa(smtpPort))
	auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)

	done := make(chan error, 1)
	go func() {
		done <- smtp.SendMail(addr, auth, fromAddr, []string{to}, []byte(msg))
	}()

	select {
	case <-ctx.Done():
		return &ToolResult{Success: false, Error: "send_email canceled by context"}, nil
	case err := <-done:
		if err != nil {
			return &ToolResult{Success: false, Error: fmt.Sprintf("send_email failed: %v", err)}, nil
		}
	}

	return &ToolResult{Success: true, Output: fmt.Sprintf("Email sent to %s.", to)}, nil
}

type SlackNotifyTool struct {
	BaseTool
}

func NewSlackNotifyTool() *SlackNotifyTool {
	return &SlackNotifyTool{
		BaseTool: BaseTool{
			name:               "slack_post",
			description:        "Posts a message to Slack.",
			category:           CategoryCommunication,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *SlackNotifyTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"channel": map[string]interface{}{"type": "string", "description": "Channel name"},
			"message": map[string]interface{}{"type": "string", "description": "Message content"},
		},
		"required": []string{"channel", "message"},
	}
	b, _ := json.Marshal(schema)
	return b
}

func (t *SlackNotifyTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	message := commGetStringParam(params, "message")
	channel := commGetStringParam(params, "channel")
	webhookURL := getParamOrEnv(params, "webhook_url", "SLACK_WEBHOOK_URL")

	if message == "" {
		return &ToolResult{Success: false, Error: "message is required"}, nil
	}
	if webhookURL == "" {
		return &ToolResult{Success: false, Error: "webhook URL is required (set webhook_url or SLACK_WEBHOOK_URL)"}, nil
	}

	payload := map[string]string{"text": message}
	if channel != "" {
		payload["channel"] = channel
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("failed to marshal payload: %v", err)}, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(b))
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("failed to build request: %v", err)}, nil
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("slack_post failed: %v", err)}, nil
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("slack_post failed: status %d, body: %s", resp.StatusCode, strings.TrimSpace(string(respBody))),
		}, nil
	}

	return &ToolResult{Success: true, Output: "Message posted to Slack webhook."}, nil
}

func commGetStringParam(params map[string]interface{}, key string) string {
	v, _ := params[key].(string)
	return strings.TrimSpace(v)
}

func getParamOrEnv(params map[string]interface{}, paramKey, envKey string) string {
	if v := commGetStringParam(params, paramKey); v != "" {
		return v
	}
	return strings.TrimSpace(os.Getenv(envKey))
}

func getIntParam(params map[string]interface{}, key string, fallback int) int {
	if raw, ok := params[key]; ok {
		switch v := raw.(type) {
		case float64:
			return int(v)
		case int:
			return v
		case string:
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				return n
			}
		}
	}
	return fallback
}

func parseEnvInt(key string, fallback int) int {
	s := strings.TrimSpace(os.Getenv(key))
	if s == "" {
		return fallback
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return n
}
