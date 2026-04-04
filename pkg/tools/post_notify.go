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
	"sync"
	"time"
)

// TelegramSender is the minimal interface for sending a Telegram message.
// Satisfied by *telegram.BotAPI wrapper or any adapter.
type TelegramSender interface {
	SendMessage(chatID int64, text string) error
}

// NotificationRouter fans a message out to all configured backends.
// Fields are optional — nil means "not configured".
// HARDCODED THROTTLE: Prevents notification spam (user requirement).
type NotificationRouter struct {
	Discord             DiscordSender
	Telegram            TelegramSender
	DefaultDiscordChan  string
	DefaultTelegramChat int64

	// Throttle: Hard limit on notification frequency
	throttleMu       sync.RWMutex
	lastNotification map[string]time.Time // key = notification type/content hash

	// HARD LIMITS (user-specified - DO NOT MODIFY WITHOUT USER APPROVAL)
	// Minimum time between ANY notifications (global)
	MinGlobalCooldown time.Duration
	// Minimum time between notifications of the same type
	MinTypeCooldown time.Duration
}

const (
	// DefaultMinGlobalCooldown: Absolute minimum between ANY two notifications
	// User said "notifying every several minutes is ridiculous"
	// This ensures max 8 notifications per hour (1 every 7.5 minutes minimum)
	DefaultMinGlobalCooldown = 7*time.Minute + 30*time.Second

	// DefaultMinTypeCooldown: Minimum between notifications of same type
	// Prevents identical warnings from repeated checks
	DefaultMinTypeCooldown = 30 * time.Minute
)

// NewNotificationRouter creates a NotificationRouter.
// Pass nil for any backend that is not configured.
// Default throttles are HARD LIMITS to prevent notification spam.
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
		lastNotification:    make(map[string]time.Time),
		MinGlobalCooldown:   DefaultMinGlobalCooldown, // 7.5 min between ANY notifications
		MinTypeCooldown:     DefaultMinTypeCooldown,   // 30 min between same-type notifications
	}
}

// checkThrottle returns empty string if notification is allowed, or error message if throttled.
func (r *NotificationRouter) checkThrottle(text string) string {
	r.throttleMu.Lock()
	defer r.throttleMu.Unlock()

	now := time.Now()

	// Check global cooldown: ANY notification type
	if lastGlobal, exists := r.lastNotification["__global__"]; exists {
		elapsed := now.Sub(lastGlobal)
		if elapsed < r.MinGlobalCooldown {
			waitTime := r.MinGlobalCooldown - elapsed
			return fmt.Sprintf("THROTTLED (global): Next notification allowed in %.0f seconds (limit: 1 per %.0f seconds)",
				waitTime.Seconds(), r.MinGlobalCooldown.Seconds())
		}
	}

	// Check type-specific cooldown: same message content
	// Hash the message to group similar notifications
	msgHash := fmt.Sprintf("msg:%d", hashString(text))
	if lastType, exists := r.lastNotification[msgHash]; exists {
		elapsed := now.Sub(lastType)
		if elapsed < r.MinTypeCooldown {
			waitTime := r.MinTypeCooldown - elapsed
			return fmt.Sprintf("THROTTLED (type): This notification was recently sent. Next allowed in %.0f seconds",
				waitTime.Seconds())
		}
	}

	// Notification is allowed - update both clocks
	r.lastNotification["__global__"] = now
	r.lastNotification[msgHash] = now

	return ""
}

// hashString returns a simple hash of the string
func hashString(s string) int {
	h := 0
	for _, c := range s {
		h = h*31 + int(c)
	}
	if h < 0 {
		h = -h
	}
	return h
}

// Send delivers text to all enabled backends. Returns a multi-error string if
// any backend fails; continues to other backends regardless.
// THROTTLED: respects global and type-specific cooldowns.
func (r *NotificationRouter) Send(text string) error {
	// Check throttle FIRST before doing any work
	if throttleMsg := r.checkThrottle(text); throttleMsg != "" {
		return fmt.Errorf("%s", throttleMsg)
	}
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
		// Check if this is a throttle error (not a backend failure)
		errMsg := err.Error()
		if strings.Contains(errMsg, "THROTTLED") {
			// Throttling is not a failure - it's working as designed
			return &ToolResult{
				Success: true,
				Output:  errMsg + " (throttle protection active - this prevents notification spam)",
			}, nil
		}
		// Actual backend failures are reported as errors
		return &ToolResult{
			Success: false,
			Error:   errMsg,
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

// SetThrottleLimits configures the notification frequency limits.
// CAUTION: These are HARD LIMITS to prevent notification spam.
// globalCooldown: Minimum time between ANY two notifications (across all types)
// typeCooldown: Minimum time between notifications of the same type
// Per user request: "notifying every several minutes is ridiculous"
// Recommended: globalCooldown >= 7 minutes, typeCooldown >= 30 minutes
func (r *NotificationRouter) SetThrottleLimits(globalCooldown, typeCooldown time.Duration) {
	r.throttleMu.Lock()
	defer r.throttleMu.Unlock()
	r.MinGlobalCooldown = globalCooldown
	r.MinTypeCooldown = typeCooldown
}

// GetThrottleStats returns current throttle state for debugging
func (r *NotificationRouter) GetThrottleStats() map[string]interface{} {
	r.throttleMu.RLock()
	defer r.throttleMu.RUnlock()

	globalLast, hasGlobal := r.lastNotification["__global__"]
	nextGlobal := time.Time{}
	if hasGlobal {
		nextGlobal = globalLast.Add(r.MinGlobalCooldown)
	}

	return map[string]interface{}{
		"last_notification":        globalLast,
		"next_allowed":             nextGlobal,
		"global_cooldown":          r.MinGlobalCooldown.String(),
		"type_cooldown":            r.MinTypeCooldown.String(),
		"pending_notifications":    len(r.lastNotification) - 1, // exclude __global__
		"time_until_next":          time.Until(nextGlobal).String(),
		"notification_limit_perhr": 60 / (int(r.MinGlobalCooldown.Minutes()) / 60),
	}
}
