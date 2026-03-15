package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// DiscordSender is the minimal interface satisfied by *discord.Manager.
// Declaring it here avoids a circular import between pkg/tools and
// pkg/channels/discord; the concrete type is injected from main.go.
type DiscordSender interface {
	SendToChannel(channelID, text string) error
}

// DiscordSendTool lets the agent proactively push a message to any Discord
// channel that the bot has access to.
//
// Typical use: the user says "post X to my #announcements channel" and
// provides the channel ID; the agent calls this tool to deliver it.
type DiscordSendTool struct {
	BaseTool
	sender DiscordSender
}

// NewDiscordSendTool creates the tool wired to sender.
// Pass nil when Discord is not configured; the tool will still register
// but return a clear error message if the agent tries to use it.
func NewDiscordSendTool(sender DiscordSender) *DiscordSendTool {
	return &DiscordSendTool{
		BaseTool: NewBaseTool(
			"discord_send",
			"Send a message to a Discord channel on behalf of the user. "+
				"Use this when the user explicitly asks to notify, post, or "+
				"message someone on Discord. Requires the channel snowflake ID.",
			CategoryCommunication,
			true,
			PermissionOnce,
		),
		sender: sender,
	}
}

func (t *DiscordSendTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"channel_id": {
				"type": "string",
				"description": "Discord channel snowflake ID. Right-click a channel and select 'Copy Channel ID' (requires Developer Mode enabled in Discord settings)."
			},
			"message": {
				"type": "string",
				"description": "Message content to send. Supports Discord markdown: **bold**, *italic*, inline code, and fenced code blocks."
			}
		},
		"required": ["channel_id", "message"]
	}`)
}

func (t *DiscordSendTool) Execute(_ context.Context, params map[string]interface{}) (*ToolResult, error) {
	channelID, _ := params["channel_id"].(string)
	message, _ := params["message"].(string)

	if channelID == "" {
		return &ToolResult{Success: false, Error: "channel_id is required"}, nil
	}
	if message == "" {
		return &ToolResult{Success: false, Error: "message is required"}, nil
	}
	if t.sender == nil {
		return &ToolResult{
			Success: false,
			Error:   "Discord integration is not configured (DISCORD_BOT_TOKEN not set)",
		}, nil
	}

	if err := t.sender.SendToChannel(channelID, message); err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("discord_send failed: %v", err),
		}, nil
	}

	return &ToolResult{
		Success: true,
		Output:  fmt.Sprintf("Message delivered to Discord channel %s.", channelID),
	}, nil
}
