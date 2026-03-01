package router

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FeedbackRecord is a single persisted routing outcome (JSONL format).
type FeedbackRecord struct {
	Timestamp  time.Time `json:"ts"`
	TaskID     string    `json:"task_id"`
	ModelID    string    `json:"model_id"`
	Category   string    `json:"category"` // QueryCategory string
	Score      float64   `json:"score"`    // 0.0–1.0
	LatencyMs  int64     `json:"latency_ms"`
	Success    bool      `json:"success"`
	UserRating int       `json:"user_rating"` // 0 = unrated
}

// FeedbackManager persists routing outcomes to a JSONL file and feeds them
// into an AdaptiveRouter so future routing decisions improve over time.
type FeedbackManager struct {
	mu          sync.Mutex
	logger      *slog.Logger
	path        string // ~/.config/gorkbot/router_feedback.jsonl
	file        *os.File
	adaptive    *AdaptiveRouter
}

// NewFeedbackManager creates a FeedbackManager that persists to dir/router_feedback.jsonl.
// Pass configDir = "" to disable persistence (log-only mode).
func NewFeedbackManager(configDir string, logger *slog.Logger) *FeedbackManager {
	fm := &FeedbackManager{
		logger:   logger,
		adaptive: NewAdaptiveRouter(500),
	}
	if configDir == "" {
		return fm
	}

	fm.path = filepath.Join(configDir, "router_feedback.jsonl")
	fm.loadHistory()

	f, err := os.OpenFile(fm.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		logger.Warn("Cannot open feedback file", "path", fm.path, "error", err)
	} else {
		fm.file = f
	}
	return fm
}

// loadHistory pre-seeds the AdaptiveRouter from persisted records.
func (fm *FeedbackManager) loadHistory() {
	data, err := os.ReadFile(fm.path)
	if err != nil {
		return
	}
	lines := splitLines(data)
	for _, line := range lines {
		var rec FeedbackRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		fm.adaptive.RecordOutcome(QueryCategory(rec.Category), rec.ModelID, rec.Score)
	}
	fm.logger.Info("Loaded routing feedback history", "records", len(lines), "path", fm.path)
}

// RecordFeedback saves the result of a task for future learning.
func (fm *FeedbackManager) RecordFeedback(f Feedback) {
	status := "SUCCESS"
	if !f.Success {
		status = "FAILURE"
	}
	fm.logger.Info("Routing feedback",
		"task_id", f.TaskID,
		"model", f.Route.Primary.ID,
		"tier", f.Route.RoutingTier,
		"status", status,
		"rating", f.UserRating,
		"latency_ms", f.Latency,
	)
}

// RecordOutcome records a simple outcome (query category, model used, quality score).
// score should be 0.0–1.0 where 1.0 = perfect response.
func (fm *FeedbackManager) RecordOutcome(category QueryCategory, modelID string, score float64, success bool) {
	rec := FeedbackRecord{
		Timestamp: time.Now(),
		ModelID:   modelID,
		Category:  string(category),
		Score:     score,
		Success:   success,
	}

	fm.mu.Lock()
	defer fm.mu.Unlock()

	// Feed into adaptive router.
	fm.adaptive.RecordOutcome(category, modelID, score)

	// Persist to JSONL.
	if fm.file != nil {
		data, _ := json.Marshal(rec)
		fmt.Fprintf(fm.file, "%s\n", data)
	}
}

// SuggestModel asks the adaptive router for the best model for a query category.
// Returns "" when there is insufficient history.
func (fm *FeedbackManager) SuggestModel(category QueryCategory) string {
	return fm.adaptive.SuggestModel(category)
}

// Close flushes and closes the feedback file.
func (fm *FeedbackManager) Close() {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	if fm.file != nil {
		_ = fm.file.Close()
		fm.file = nil
	}
}

func splitLines(data []byte) []string {
	var lines []string
	start := 0
	for i, b := range data {
		if b == '\n' {
			line := string(data[start:i])
			if line != "" {
				lines = append(lines, line)
			}
			start = i + 1
		}
	}
	if start < len(data) {
		if line := string(data[start:]); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
