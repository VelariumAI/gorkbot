package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"
)

// schedulerCtxKey is an unexported type for the context key.
type schedulerCtxKey struct{}

// SchedulerKey is the exported context key used to inject a *Scheduler into tool calls.
var SchedulerKey = schedulerCtxKey{}

// TaskRunner is a callback invoked when a scheduled task is due.
type TaskRunner func(task ScheduledTask)

// Scheduler polls the store every 30 seconds and fires due tasks.
type Scheduler struct {
	store  *Store
	runner TaskRunner
	stop   chan struct{}
	logger *slog.Logger
}

// NewScheduler creates a Scheduler backed by store, invoking runner for each due task.
func NewScheduler(store *Store, runner TaskRunner, logger *slog.Logger) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{
		store:  store,
		runner: runner,
		stop:   make(chan struct{}),
		logger: logger,
	}
}

// Start launches the background polling goroutine. It respects ctx cancellation.
func (s *Scheduler) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		// Run immediately on start to pick up any tasks already due.
		s.runDueTasks()
		for {
			select {
			case <-ctx.Done():
				return
			case <-s.stop:
				return
			case <-ticker.C:
				s.runDueTasks()
			}
		}
	}()
}

// Stop signals the background goroutine to exit.
func (s *Scheduler) Stop() {
	select {
	case s.stop <- struct{}{}:
	default:
	}
}

// Store returns the underlying store (used by tools to compute NextRun on creation).
func (s *Scheduler) Store() *Store {
	return s.store
}

// ComputeNextRun is exported so tools can call it when creating a task.
func (s *Scheduler) ComputeNextRun(task ScheduledTask) (*time.Time, error) {
	return s.computeNextRun(task)
}

// computeNextRun returns when the task should next execute.
func (s *Scheduler) computeNextRun(task ScheduledTask) (*time.Time, error) {
	now := time.Now()

	switch task.ScheduleType {
	case ScheduleCron:
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		sched, err := parser.Parse(task.ScheduleValue)
		if err != nil {
			return nil, fmt.Errorf("invalid cron expression %q: %w", task.ScheduleValue, err)
		}
		next := sched.Next(now)
		return &next, nil

	case ScheduleInterval:
		d, err := time.ParseDuration(task.ScheduleValue)
		if err != nil {
			return nil, fmt.Errorf("invalid interval %q: %w", task.ScheduleValue, err)
		}
		base := task.CreatedAt
		if task.LastRun != nil {
			base = *task.LastRun
		}
		next := base.Add(d)
		// If the computed next is already in the past, use now + duration.
		if next.Before(now) {
			next = now.Add(d)
		}
		return &next, nil

	case ScheduleOnce:
		t, err := time.Parse(time.RFC3339, task.ScheduleValue)
		if err != nil {
			return nil, fmt.Errorf("invalid RFC3339 time %q: %w", task.ScheduleValue, err)
		}
		return &t, nil

	default:
		return nil, fmt.Errorf("unknown schedule type: %q", task.ScheduleType)
	}
}

// isDue returns true if the task should fire right now.
func (s *Scheduler) isDue(task ScheduledTask) bool {
	if task.Status != TaskActive {
		return false
	}
	if task.NextRun == nil {
		return false
	}
	return time.Now().After(*task.NextRun)
}

// runDueTasks iterates all tasks and fires any that are due.
func (s *Scheduler) runDueTasks() {
	tasks := s.store.List()
	for _, task := range tasks {
		if !s.isDue(task) {
			continue
		}
		s.logger.Info("scheduler: firing task", "id", task.ID, "name", task.Name)

		// Invoke the runner (non-blocking — caller can goroutine if needed).
		if s.runner != nil {
			s.runner(task)
		}

		// Update task state.
		now := time.Now()
		task.LastRun = &now
		task.RunCount++

		if task.ScheduleType == ScheduleOnce {
			task.Status = TaskCompleted
			task.NextRun = nil
		} else {
			next, err := s.computeNextRun(task)
			if err != nil {
				s.logger.Warn("scheduler: failed to compute next run", "id", task.ID, "err", err)
			} else {
				task.NextRun = next
			}
		}

		if err := s.store.Update(task); err != nil {
			s.logger.Warn("scheduler: failed to update task after run", "id", task.ID, "err", err)
		}
	}
}

// WithScheduler returns a context with the scheduler embedded.
func WithScheduler(ctx context.Context, s *Scheduler) context.Context {
	return context.WithValue(ctx, SchedulerKey, s)
}
