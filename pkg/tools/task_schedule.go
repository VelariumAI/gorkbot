package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/scheduler"
)

// ---- schedule_task --------------------------------------------------------

// ScheduleTaskTool schedules an AI prompt to run on a cron, interval, or once schedule.
type ScheduleTaskTool struct {
	BaseTool
}

func NewScheduleTaskTool() *ScheduleTaskTool {
	return &ScheduleTaskTool{
		BaseTool: BaseTool{
			name:               "schedule_task",
			description:        "Schedule an AI prompt to run automatically on a cron schedule, interval, or once at a specific time.",
			category:           CategoryMeta,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *ScheduleTaskTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "A human-readable name for the task.",
			},
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "The AI prompt to run on the schedule.",
			},
			"schedule_type": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"cron", "interval", "once"},
				"description": "Type of schedule: cron expression, repeating interval, or single run.",
			},
			"schedule_value": map[string]interface{}{
				"type":        "string",
				"description": "The schedule value: a cron expression (e.g. \"0 8 * * 1\"), a Go duration (e.g. \"30m\", \"2h\"), or an RFC3339 timestamp (e.g. \"2026-03-01T08:00:00Z\").",
			},
			"context_mode": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"session", "isolated"},
				"description": "Whether to run in the current session context or an isolated context. Defaults to \"isolated\".",
			},
		},
		"required": []string{"name", "prompt", "schedule_type", "schedule_value"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *ScheduleTaskTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	name, _ := params["name"].(string)
	prompt, _ := params["prompt"].(string)
	schedTypeStr, _ := params["schedule_type"].(string)
	schedValue, _ := params["schedule_value"].(string)
	contextModeStr, _ := params["context_mode"].(string)

	if name == "" || prompt == "" || schedTypeStr == "" || schedValue == "" {
		return &ToolResult{Success: false, Error: "name, prompt, schedule_type, and schedule_value are required"}, nil
	}

	schedType := scheduler.ScheduleType(schedTypeStr)
	switch schedType {
	case scheduler.ScheduleCron, scheduler.ScheduleInterval, scheduler.ScheduleOnce:
	default:
		return &ToolResult{Success: false, Error: fmt.Sprintf("invalid schedule_type %q; must be cron, interval, or once", schedTypeStr)}, nil
	}

	contextMode := scheduler.ContextIsolated
	if contextModeStr == string(scheduler.ContextSession) {
		contextMode = scheduler.ContextSession
	}

	sched, _ := ctx.Value(scheduler.SchedulerKey).(*scheduler.Scheduler)
	if sched == nil {
		return &ToolResult{Success: false, Error: "scheduler not available"}, nil
	}

	now := time.Now()
	id := fmt.Sprintf("%d", now.UnixNano())

	task := scheduler.ScheduledTask{
		ID:            id,
		Name:          name,
		Prompt:        prompt,
		ScheduleType:  schedType,
		ScheduleValue: schedValue,
		ContextMode:   contextMode,
		Status:        scheduler.TaskActive,
		CreatedAt:     now,
		RunCount:      0,
	}

	next, err := sched.ComputeNextRun(task)
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("invalid schedule: %v", err)}, nil
	}
	task.NextRun = next

	if err := sched.Store().Add(task); err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("failed to save task: %v", err)}, nil
	}

	nextStr := next.Format(time.RFC3339)
	result := fmt.Sprintf("Task scheduled.\nID: %s\nName: %s\nSchedule: %s (%s)\nContext: %s\nNext run: %s",
		id, name, schedValue, schedType, contextMode, nextStr)

	return &ToolResult{
		Success: true,
		Output:  result,
	}, nil
}

// ---- list_scheduled_tasks -------------------------------------------------

// ListScheduledTasksTool lists all scheduled tasks.
type ListScheduledTasksTool struct {
	BaseTool
}

func NewListScheduledTasksTool() *ListScheduledTasksTool {
	return &ListScheduledTasksTool{
		BaseTool: BaseTool{
			name:               "list_scheduled_tasks",
			description:        "List all scheduled AI tasks, their status, schedule, and run history.",
			category:           CategoryMeta,
			requiresPermission: false,
			defaultPermission:  PermissionAlways,
		},
	}
}

func (t *ListScheduledTasksTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *ListScheduledTasksTool) Execute(ctx context.Context, _ map[string]interface{}) (*ToolResult, error) {
	sched, _ := ctx.Value(scheduler.SchedulerKey).(*scheduler.Scheduler)
	if sched == nil {
		return &ToolResult{Success: false, Error: "scheduler not available"}, nil
	}

	tasks := sched.Store().List()
	if len(tasks) == 0 {
		return &ToolResult{Success: true, Output: "No scheduled tasks."}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%-20s %-20s %-10s %-10s %-25s %-25s %s\n",
		"ID (short)", "Name", "Type", "Status", "Next Run", "Last Run", "Runs"))
	sb.WriteString(strings.Repeat("-", 120) + "\n")

	for _, task := range tasks {
		shortID := task.ID
		if len(shortID) > 18 {
			shortID = shortID[len(shortID)-18:]
		}

		nextStr := "—"
		if task.NextRun != nil {
			nextStr = task.NextRun.Format("2006-01-02 15:04:05")
		}
		lastStr := "—"
		if task.LastRun != nil {
			lastStr = task.LastRun.Format("2006-01-02 15:04:05")
		}

		sb.WriteString(fmt.Sprintf("%-20s %-20s %-10s %-10s %-25s %-25s %d\n",
			shortID, truncateStr(task.Name, 18), task.ScheduleType, task.Status, nextStr, lastStr, task.RunCount))
	}

	return &ToolResult{Success: true, Output: sb.String()}, nil
}

// truncateStr shortens a string to at most n runes.
func truncateStr(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}

// ---- cancel_scheduled_task ------------------------------------------------

// CancelScheduledTaskTool cancels a scheduled task by ID.
type CancelScheduledTaskTool struct {
	BaseTool
}

func NewCancelScheduledTaskTool() *CancelScheduledTaskTool {
	return &CancelScheduledTaskTool{
		BaseTool: BaseTool{
			name:               "cancel_scheduled_task",
			description:        "Cancel a scheduled AI task so it will no longer run.",
			category:           CategoryMeta,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *CancelScheduledTaskTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "string",
				"description": "The task ID to cancel.",
			},
		},
		"required": []string{"id"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *CancelScheduledTaskTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	id, _ := params["id"].(string)
	if id == "" {
		return &ToolResult{Success: false, Error: "id is required"}, nil
	}

	sched, _ := ctx.Value(scheduler.SchedulerKey).(*scheduler.Scheduler)
	if sched == nil {
		return &ToolResult{Success: false, Error: "scheduler not available"}, nil
	}

	if err := sched.Store().SetStatus(id, scheduler.TaskCancelled); err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("failed to cancel task: %v", err)}, nil
	}

	return &ToolResult{Success: true, Output: fmt.Sprintf("Task %s has been cancelled.", id)}, nil
}

// ---- pause_resume_scheduled_task ------------------------------------------

// PauseResumeScheduledTaskTool pauses or resumes a scheduled task.
type PauseResumeScheduledTaskTool struct {
	BaseTool
}

func NewPauseResumeScheduledTaskTool() *PauseResumeScheduledTaskTool {
	return &PauseResumeScheduledTaskTool{
		BaseTool: BaseTool{
			name:               "pause_resume_scheduled_task",
			description:        "Pause or resume a scheduled AI task.",
			category:           CategoryMeta,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *PauseResumeScheduledTaskTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "string",
				"description": "The task ID to pause or resume.",
			},
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"pause", "resume"},
				"description": "Whether to pause or resume the task.",
			},
		},
		"required": []string{"id", "action"},
	}
	data, _ := json.Marshal(schema)
	return data
}

func (t *PauseResumeScheduledTaskTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	id, _ := params["id"].(string)
	action, _ := params["action"].(string)

	if id == "" || action == "" {
		return &ToolResult{Success: false, Error: "id and action are required"}, nil
	}

	sched, _ := ctx.Value(scheduler.SchedulerKey).(*scheduler.Scheduler)
	if sched == nil {
		return &ToolResult{Success: false, Error: "scheduler not available"}, nil
	}

	var newStatus scheduler.TaskStatus
	switch action {
	case "pause":
		newStatus = scheduler.TaskPaused
	case "resume":
		newStatus = scheduler.TaskActive
	default:
		return &ToolResult{Success: false, Error: fmt.Sprintf("invalid action %q; must be pause or resume", action)}, nil
	}

	if err := sched.Store().SetStatus(id, newStatus); err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("failed to update task: %v", err)}, nil
	}

	verb := "paused"
	if action == "resume" {
		verb = "resumed"
	}
	return &ToolResult{Success: true, Output: fmt.Sprintf("Task %s has been %s.", id, verb)}, nil
}
