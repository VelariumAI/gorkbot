package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/velariumai/gorkbot/pkg/ai"
)

// Session represents a persistent conversation
type Session struct {
	ID        string                  `json:"id"`
	CreatedAt time.Time               `json:"created_at"`
	UpdatedAt time.Time               `json:"updated_at"`
	History   *ai.ConversationHistory `json:"history"`
	Metadata  map[string]interface{}  `json:"metadata"`
}

// MemoryManager handles session persistence and intelligence
type MemoryManager struct {
	sessionsDir string
	mu          sync.RWMutex
	current     *Session
	Logger      *slog.Logger
}

// NewMemoryManager creates a new memory manager
func NewMemoryManager(configDir string, logger *slog.Logger) (*MemoryManager, error) {
	sessionsDir := filepath.Join(configDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create sessions dir: %w", err)
	}

	return &MemoryManager{
		sessionsDir: sessionsDir,
		Logger:      logger,
	}, nil
}

// LoadDefaultSession loads the default session or creates a new one
func (mm *MemoryManager) LoadDefaultSession() (*Session, error) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	path := filepath.Join(mm.sessionsDir, "default.json")

	// Try to load existing
	if _, err := os.Stat(path); err == nil {
		data, err := os.ReadFile(path)
		if err == nil {
			var session Session
			if err := json.Unmarshal(data, &session); err == nil {
				mm.current = &session
				mm.Logger.Info("Loaded existing session", "id", session.ID, "messages", session.History.Count())
				return &session, nil
			}
		}
	}

	// Create new
	session := &Session{
		ID:        "default",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		History:   ai.NewConversationHistory(),
		Metadata:  make(map[string]interface{}),
	}
	mm.current = session
	if err := mm.SaveSession(); err != nil {
		mm.Logger.Warn("Failed to save new default session", "error", err)
	}

	mm.Logger.Info("Created new default session")
	return session, nil
}

// SaveSession saves the current session to disk
func (mm *MemoryManager) SaveSession() error {
	if mm.current == nil {
		return nil
	}

	mm.current.UpdatedAt = time.Now()

	path := filepath.Join(mm.sessionsDir, fmt.Sprintf("%s.json", mm.current.ID))
	data, err := json.MarshalIndent(mm.current, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

// ConsolidateMemory summarizes older messages to save tokens while keeping context
func (mm *MemoryManager) ConsolidateMemory(ctx context.Context, provider ai.AIProvider) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if mm.current == nil || mm.current.History.Count() < 20 {
		return nil // Not enough history to consolidate
	}

	mm.Logger.Info("Consolidating memory...", "original_count", mm.current.History.Count())

	// Strategy: Keep first (system), Keep last 10 (recent context), Summarize the middle
	msgs := mm.current.History.GetMessages()
	if len(msgs) < 15 {
		return nil
	}

	// Identify ranges
	systemPrompt := ""
	if msgs[0].Role == "system" {
		systemPrompt = msgs[0].Content
	}

	// Summarize middle
	middleMsgs := msgs[1 : len(msgs)-10]

	var sb strings.Builder
	for _, m := range middleMsgs {
		sb.WriteString(fmt.Sprintf("%s: %s\n", m.Role, m.Content))
	}

	prompt := fmt.Sprintf("Summarize the following conversation history into a concise list of key facts, user preferences, and project context. Keep it under 200 words:\n\n%s", sb.String())

	summary, err := provider.Generate(ctx, prompt)
	if err != nil {
		return fmt.Errorf("summarization failed: %w", err)
	}

	// Reconstruct history
	newHistory := ai.NewConversationHistory()
	if systemPrompt != "" {
		newHistory.AddSystemMessage(systemPrompt)
	}

	// Inject summary as a system context
	newHistory.AddSystemMessage(fmt.Sprintf("PREVIOUS CONTEXT SUMMARY:\n%s", summary))

	// Add recent messages
	for _, m := range msgs[len(msgs)-10:] {
		newHistory.AddMessage(m.Role, m.Content)
	}

	mm.current.History = newHistory
	mm.Logger.Info("Memory consolidated", "new_count", newHistory.Count())

	return mm.SaveSession()
}
