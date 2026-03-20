// Package tools — post_notify.go
//
// PostNotifyTool lets the AI agent proactively push a notification to one or
// more configured channels (Discord, Telegram) without waiting for a user to
// ask a question. Ported from build-your-own-openclaw Step 14
// (post-message-back) into idiomatic Go.
//
// This is distinct from discord_send.go (which requires the user to supply a
// channel ID every time). PostNotifyTool uses pre-configured default
// destinations injected at construction time, making it suitable for use
// inside cron jobs, scheduled tasks, and background agents where no
// interactive user is present.
//
// Wire up in main.go:
//
//	notifier := tools.NewNotificationRouter(
//	    discordManager,    // implements DiscordSender; nil to disable
//	    telegramBot,       // implements TelegramSender; nil to disable
//	    "123456789",       // default Discord channel ID (empty to disable)
//	    9876543210,        // default Telegram chat ID (0 to disable)
//	)
//	registry.Register(tools.NewPostNotifyTool(notifier))
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// TelegramSender is the minimal interface for sending a Telegram message.
// Satisfied by *telegram.BotAPI wrapper or any adapter.
type TelegramSender interface {
	SendMessage(chatID int64, text string) error
}

// NotificationRouter fans a message out to all configured backends.
// Fields are optional — nil means "not configured".
type NotificationRouter struct {
	Discord           DiscordSender
	Telegram          TelegramSender
	DefaultDiscordChan string
	DefaultTelegramChat int64
}

// NewNotificationRouter creates a NotificationRouter.
// Pass nil for any backend that is not configured.
func NewNotificationRouter(
	discord DiscordSender,
	telegram TelegramSender,
	defaultDiscordChan string,
	defaultTelegramChat int64,
) *NotificationRouter {
	return &NotificationRouter{
		Discord:             discord,
		Telegram:            telegram,
		DefaultDiscordChan:  defaultDiscordChan,
		DefaultTelegramChat: defaultTelegramChat,
	}
}

// Send delivers text to all enabled backends. Returns a multi-error string if
// any backend fails; continues to other backends regardless.
func (r *NotificationRouter) Send(text string) error {
	var errs []string

	if r.Discord != nil && r.DefaultDiscordChan != "" {
		if err := r.Discord.SendToChannel(r.DefaultDiscordChan, text); err != nil {
			errs = append(errs, fmt.Sprintf("discord: %v", err))
		}
	}

	if r.Telegram != nil && r.DefaultTelegramChat != 0 {
		if err := r.Telegram.SendMessage(r.DefaultTelegramChat, text); err != nil {
			errs = append(errs, fmt.Sprintf("telegram: %v", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("post_notify partial failure: %s", strings.Join(errs, "; "))
	}
	return nil
}

// IsConfigured returns true when at least one backend is ready to deliver.
func (r *NotificationRouter) IsConfigured() bool {
	return (r.Discord != nil && r.DefaultDiscordChan != "") ||
		(r.Telegram != nil && r.DefaultTelegramChat != 0)
}

// ─────────────────────────────────────────────────────────────────────────────

// PostNotifyTool is the Tool implementation that exposes NotificationRouter to
// the AI agent.
type PostNotifyTool struct {
	BaseTool
	router *NotificationRouter
}

// NewPostNotifyTool creates the tool. router may be nil — the tool registers
// successfully but returns a clear error if the agent tries to use it without
// any configured backends.
func NewPostNotifyTool(router *NotificationRouter) *PostNotifyTool {
	return &PostNotifyTool{
		BaseTool: NewBaseTool(
			"post_notify",
			"Proactively push a notification message to configured channels "+
				"(Discord and/or Telegram). Use this to inform the user of "+
				"completed background tasks, alerts, or autonomous findings "+
				"without waiting for a user prompt. Suitable for cron jobs "+
				"and background agents.",
			CategoryCommunication,
			false,
			PermissionSession,
		),
		router: router,
	}
}

// SetRouter replaces the notification router after construction.
// Useful for wiring in chat IDs that are only known after bot startup.
func (t *PostNotifyTool) SetRouter(r *NotificationRouter) { t.router = r }

func (t *PostNotifyTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"message": {
				"type": "string",
				"description": "The notification text to deliver. Supports markdown formatting."
			},
			"priority": {
				"type": "string",
				"enum": ["normal", "urgent"],
				"description": "Delivery priority. 'urgent' prepends a warning emoji. Default: normal."
			}
		},
		"required": ["message"]
	}`)
}

func (t *PostNotifyTool) Execute(_ context.Context, params map[string]interface{}) (*ToolResult, error) {
	message, _ := params["message"].(string)
	priority, _ := params["priority"].(string)

	if message == "" {
		return &ToolResult{Success: false, Error: "message is required"}, nil
	}

	if t.router == nil || !t.router.IsConfigured() {
		return &ToolResult{
			Success: false,
			Error: "post_notify: no notification backends are configured " +
				"(DISCORD_BOT_TOKEN / default channel or Telegram bot token / chat ID required)",
		}, nil
	}

	text := message
	if priority == "urgent" {
		text = "⚠️ **URGENT** ⚠️\n\n" + message
	}

	if err := t.router.Send(text); err != nil {
		return &ToolResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	backends := t.router.activeBackendNames()
	return &ToolResult{
		Success: true,
		Output: fmt.Sprintf("Notification delivered via %s.",
			strings.Join(backends, " and ")),
	}, nil
}

// activeBackendNames returns a list of enabled backend names for user feedback.
func (r *NotificationRouter) activeBackendNames() []string {
	var names []string
	if r.Discord != nil && r.DefaultDiscordChan != "" {
		names = append(names, "Discord")
	}
	if r.Telegram != nil && r.DefaultTelegramChat != 0 {
		names = append(names, "Telegram")
	}
	if len(names) == 0 {
		return []string{"(none)"}
	}
	return names
}
