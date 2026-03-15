package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// NotifyFunc sends a notification with a subject and body.
type NotifyFunc func(subject, body string)

// ResultDispatcher implements TaskRunner with retry logic and result notifications.
type ResultDispatcher struct {
	store  *Store
	runner func(ctx context.Context, prompt string) (string, error)
	notify NotifyFunc
	logger *slog.Logger
}

// NewResultDispatcher creates a ResultDispatcher.
func NewResultDispatcher(
	store *Store,
	runner func(ctx context.Context, prompt string) (string, error),
	notify NotifyFunc,
	logger *slog.Logger,
) *ResultDispatcher {
	return &ResultDispatcher{store: store, runner: runner, notify: notify, logger: logger}
}

// Dispatch implements TaskRunner. It runs the task, handles retries, and
// sends notifications on final success or failure.
func (rd *ResultDispatcher) Dispatch(task ScheduledTask) {
	timeout := time.Duration(task.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	rd.logger.Info("dispatcher: running task", "id", task.ID, "name", task.Name, "retry", task.RetryCount)

	result, err := rd.runner(ctx, task.Prompt)

	now := time.Now()
	task.LastRun = &now
	task.RunCount++

	if err != nil && task.RetryCount < task.MaxRetries {
		// Schedule a retry with exponential backoff.
		task.RetryCount++
		task.LastError = err.Error()
		backoff := time.Duration(1<<uint(task.RetryCount)) * time.Minute
		nextRun := time.Now().Add(backoff)
		task.NextRun = &nextRun
		_ = rd.store.Update(task)
		rd.logger.Warn("dispatcher: task failed, will retry",
			"id", task.ID, "attempt", task.RetryCount, "max", task.MaxRetries, "backoff", backoff)
		return
	}

	// Final result (success or exhausted retries).
	if err != nil {
		task.LastError = err.Error()
		result = fmt.Sprintf("Failed after %d retries: %v", task.RetryCount, err)
	} else {
		task.LastError = ""
	}
	task.LastResult = result
	task.RetryCount = 0
	_ = rd.store.Update(task)

	if rd.notify != nil {
		subject := fmt.Sprintf("Task: %s", task.Name)
		rd.notify(subject, rd.formatResult(task, result, err))
	}
}

func (rd *ResultDispatcher) formatResult(task ScheduledTask, result string, err error) string {
	status := "✅ Success"
	if err != nil {
		status = fmt.Sprintf("❌ Failed (%v)", err)
	}
	if len(result) > 2000 {
		result = result[:2000] + "…"
	}
	return fmt.Sprintf("%s\nTask: %s\nPrompt: %s\n\n%s", status, task.Name, task.Prompt, result)
}
