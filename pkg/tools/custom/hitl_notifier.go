package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// HitlNotifierTool - Background monitor with SEVERE cooldowns enforced
// CRITICAL: Notification spam was causing issues, now with 2-minute minimum between alerts
type HitlNotifierTool struct {
	BaseTool
}

func NewHitlNotifierTool() *HitlNotifierTool {
	return &HitlNotifierTool{
		BaseTool: BaseTool{
			name:              "hitl_notifier",
			description:       "Background monitor: Polls for active HITL requests and fires notification with 2-MINUTE COOLDOWN (SEVERE throttle to prevent spam). Non-blocking. Run as start_background_process.",
			category:          CategorySystem,
			requiresPermission: true,
			defaultPermission: PermissionSession,
		},
	}
}

func (t *HitlNotifierTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"duration": map[string]interface{}{
				"type":        "string",
				"description": "duration parameter",
			},
		},
		"required": []string{"duration"},
		}
	data, _ := json.Marshal(schema)
	return data
}

func (t *HitlNotifierTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	duration := ""
	if p, ok := params["duration"].(string); ok {
		duration = p
	}

	// SEVERE COOLDOWN: 120 seconds (2 minutes) minimum between HITL notifications
	// Previous: 1 second = 86,400 notifications per day = UNACCEPTABLE
	// New: 2 minutes = 720 notifications per day = SEVERE THROTTLE
	command := `
COOLDOWN_FILE="/tmp/hitl_notifier_cooldown"
COOLDOWN_SECONDS=120

while true; do
  if [ $(echo '$(query_system_state)' | grep -c 'HITL Guard: ENABLED.*pending') -gt 0 ]; then
    NOW=$(date +%s)
    LAST_SENT=0
    if [ -f "$COOLDOWN_FILE" ]; then
      LAST_SENT=$(cat "$COOLDOWN_FILE" 2>/dev/null || echo 0)
    fi
    ELAPSED=$((NOW - LAST_SENT))

    # Only send notification if COOLDOWN_SECONDS have passed
    if [ $ELAPSED -ge $COOLDOWN_SECONDS ]; then
      echo 'HITL detected' | notification_send --title 'HITL Approval Needed' --content 'Tool requires approval: Check TUI prompt now.'
      echo "$NOW" > "$COOLDOWN_FILE"
      # Wait 120 seconds before next check
      sleep 120
    else
      # Still in cooldown, wait before checking again
      WAIT=$((COOLDOWN_SECONDS - ELAPSED + 5))
      sleep "$WAIT"
    fi
  else
    # No HITL pending, check less frequently (every 10 seconds)
    sleep 10
  fi
done
`

	bashTool := NewBashTool()
	return bashTool.Execute(ctx, map[string]interface{}{
		"command": command,
	})
}
