package scheduler

import "time"

type ScheduleType string

const (
	ScheduleCron     ScheduleType = "cron"
	ScheduleInterval ScheduleType = "interval"
	ScheduleOnce     ScheduleType = "once"
)

type ContextMode string

const (
	ContextSession  ContextMode = "session"  // uses existing conversation history
	ContextIsolated ContextMode = "isolated" // fresh context, no history
)

type TaskStatus string

const (
	TaskActive    TaskStatus = "active"
	TaskPaused    TaskStatus = "paused"
	TaskCompleted TaskStatus = "completed" // for "once" tasks after run
	TaskCancelled TaskStatus = "cancelled"
)

type ScheduledTask struct {
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	Prompt        string       `json:"prompt"`
	ScheduleType  ScheduleType `json:"schedule_type"`
	ScheduleValue string       `json:"schedule_value"` // cron expr, "30s"/"5m"/"2h", or RFC3339 time
	ContextMode   ContextMode  `json:"context_mode"`
	Status        TaskStatus   `json:"status"`
	CreatedAt     time.Time    `json:"created_at"`
	NextRun       *time.Time   `json:"next_run,omitempty"`
	LastRun       *time.Time   `json:"last_run,omitempty"`
	LastResult    string       `json:"last_result,omitempty"`
	RunCount      int          `json:"run_count"`
}
