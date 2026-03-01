// Package session provides session persistence, checkpointing, and export.
package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/velariumai/gorkbot/pkg/ai"
)

// Checkpoint captures conversation state before a tool executes.
type Checkpoint struct {
	ID            string                   `json:"id"`
	Timestamp     time.Time                `json:"timestamp"`
	Description   string                   `json:"description"` // e.g. "Before: bash(rm -rf build/)"
	WorkspaceHash string                   `json:"workspace_hash,omitempty"`
	Messages      []ai.ConversationMessage `json:"messages"`
	Meta          map[string]interface{}   `json:"meta,omitempty"`
}

// CheckpointManager maintains a bounded stack of session checkpoints.
type CheckpointManager struct {
	mu          sync.Mutex
	checkpoints []Checkpoint
	maxHistory  int
	storePath   string // Optional: persist checkpoints to disk
}

// NewCheckpointManager creates a manager keeping at most maxHistory checkpoints.
// If storePath != "" checkpoints are persisted to that directory.
func NewCheckpointManager(maxHistory int, storePath string) *CheckpointManager {
	if maxHistory <= 0 {
		maxHistory = 20
	}
	cm := &CheckpointManager{
		maxHistory: maxHistory,
		storePath:  storePath,
	}
	if storePath != "" {
		_ = os.MkdirAll(storePath, 0755)
	}
	return cm
}

// Save creates a checkpoint from the current conversation history.
// Returns the new checkpoint ID.
func (cm *CheckpointManager) Save(description string, history *ai.ConversationHistory, workspaceHash string) string {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	id := fmt.Sprintf("cp_%03d_%d", len(cm.checkpoints)+1, time.Now().UnixMilli()%100000)

	msgs := make([]ai.ConversationMessage, 0)
	if history != nil {
		msgs = append(msgs, history.GetMessages()...)
	}

	cp := Checkpoint{
		ID:            id,
		Timestamp:     time.Now(),
		Description:   description,
		WorkspaceHash: workspaceHash,
		Messages:      msgs,
	}

	cm.checkpoints = append(cm.checkpoints, cp)

	// Trim to maxHistory
	if len(cm.checkpoints) > cm.maxHistory {
		cm.checkpoints = cm.checkpoints[len(cm.checkpoints)-cm.maxHistory:]
	}

	// Optionally persist
	if cm.storePath != "" {
		_ = cm.persistCheckpoint(cp)
	}

	return id
}

// Rewind restores the conversation history to a checkpoint.
// Pass "last" to restore the most recent checkpoint.
func (cm *CheckpointManager) Rewind(id string, history *ai.ConversationHistory) (*Checkpoint, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if len(cm.checkpoints) == 0 {
		return nil, fmt.Errorf("no checkpoints available")
	}

	var cp *Checkpoint
	if id == "last" || id == "" {
		c := cm.checkpoints[len(cm.checkpoints)-1]
		cp = &c
	} else {
		for i := range cm.checkpoints {
			if cm.checkpoints[i].ID == id {
				cp = &cm.checkpoints[i]
				break
			}
		}
	}

	if cp == nil {
		return nil, fmt.Errorf("checkpoint not found: %s", id)
	}

	if history != nil {
		history.Clear()
		for _, m := range cp.Messages {
			switch m.Role {
			case "user":
				history.AddUserMessage(m.Content)
			case "assistant":
				history.AddAssistantMessage(m.Content)
			case "system":
				history.AddSystemMessage(m.Content)
			}
		}
	}

	return cp, nil
}

// List returns summary information for all stored checkpoints, newest first.
func (cm *CheckpointManager) List() []CheckpointSummary {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	summaries := make([]CheckpointSummary, 0, len(cm.checkpoints))
	for i := len(cm.checkpoints) - 1; i >= 0; i-- {
		cp := cm.checkpoints[i]
		summaries = append(summaries, CheckpointSummary{
			ID:           cp.ID,
			Timestamp:    cp.Timestamp,
			Description:  cp.Description,
			MessageCount: len(cp.Messages),
		})
	}
	return summaries
}

// Count returns the number of stored checkpoints.
func (cm *CheckpointManager) Count() int {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return len(cm.checkpoints)
}

// Format returns a human-readable checkpoint list for display.
func (cm *CheckpointManager) Format() string {
	summaries := cm.List()
	if len(summaries) == 0 {
		return "No checkpoints available. Checkpoints are saved automatically before each tool execution.\n"
	}

	out := fmt.Sprintf("# Session Checkpoints (%d saved)\n\n", len(summaries))
	for _, s := range summaries {
		age := time.Since(s.Timestamp).Round(time.Second)
		out += fmt.Sprintf("**%s** — %s\n", s.ID, s.Description)
		out += fmt.Sprintf("   %d messages | %s ago\n\n", s.MessageCount, age)
	}
	out += "---\n**Usage:** `/rewind last` or `/rewind cp_003_12345`\n"
	return out
}

// CheckpointSummary is a lightweight view of a Checkpoint.
type CheckpointSummary struct {
	ID           string
	Timestamp    time.Time
	Description  string
	MessageCount int
}

func (cm *CheckpointManager) persistCheckpoint(cp Checkpoint) error {
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(cm.storePath, cp.ID+".json")
	return os.WriteFile(path, data, 0600)
}
