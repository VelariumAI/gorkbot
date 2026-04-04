// Package api — Client-Server State Synchronization
// Phase 3: Unified state management and synchronization
package api

import (
	"context"
	"sync"
	"time"
)

// AppState represents the complete application state.
type AppState struct {
	// Current session and run
	SessionID      string
	CurrentRunID   string
	CurrentModel   string
	CurrentProvider string

	// Collections
	Runs        map[string]interface{}
	Workspaces  map[string]interface{}
	Tools       map[string]interface{}
	Memory      map[string]interface{}
	Agents      map[string]interface{}
	Metrics     map[string]interface{}

	// UI State
	ActiveWorkspace string
	ThemeMode      string
	LastUpdated    time.Time

	// Flags
	IsLoading   bool
	IsConnected bool
	HasError    bool
	ErrorMsg    string
}

// StateManager synchronizes state between client and server.
type StateManager struct {
	state            *AppState
	client           *Client
	wsClient         *WebSocketClient
	mu               sync.RWMutex
	stateChangedCh   chan *AppState
	ctx              context.Context
	cancel           context.CancelFunc
	syncInterval     time.Duration
	lastSyncTime     time.Time
}

// NewStateManager creates a new state manager.
func NewStateManager(client *Client, wsClient *WebSocketClient) *StateManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &StateManager{
		state: &AppState{
			SessionID:       generateSessionID(),
			Runs:            make(map[string]interface{}),
			Workspaces:      make(map[string]interface{}),
			Tools:           make(map[string]interface{}),
			Memory:          make(map[string]interface{}),
			Agents:          make(map[string]interface{}),
			Metrics:         make(map[string]interface{}),
			ActiveWorkspace: "chat",
			ThemeMode:       "dark",
			IsConnected:     true,
		},
		client:         client,
		wsClient:       wsClient,
		stateChangedCh: make(chan *AppState, 10),
		ctx:            ctx,
		cancel:         cancel,
		syncInterval:   30 * time.Second,
	}
}

// GetState returns a copy of the current state.
func (sm *StateManager) GetState() *AppState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// Return a shallow copy
	stateCopy := *sm.state
	return &stateCopy
}

// UpdateState updates a portion of the state.
func (sm *StateManager) UpdateState(updater func(*AppState)) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	updater(sm.state)
	sm.state.LastUpdated = time.Now()

	// Notify listeners
	select {
	case sm.stateChangedCh <- sm.state:
	default:
		// Channel full, skip notification
	}
}

// OnStateChange registers a listener for state changes.
func (sm *StateManager) OnStateChange() <-chan *AppState {
	return sm.stateChangedCh
}

// SyncRuns fetches recent runs from the server.
func (sm *StateManager) SyncRuns(ctx context.Context) error {
	runs, err := sm.client.GetRuns(ctx, 20)
	if err != nil {
		sm.UpdateState(func(s *AppState) {
			s.HasError = true
			s.ErrorMsg = "Failed to sync runs: " + err.Error()
		})
		return err
	}

	sm.UpdateState(func(s *AppState) {
		s.Runs = make(map[string]interface{})
		for _, run := range runs {
			if id, ok := run["id"].(string); ok {
				s.Runs[id] = run
			}
		}
		s.HasError = false
		s.ErrorMsg = ""
	})

	return nil
}

// SyncWorkspaces fetches available workspaces.
func (sm *StateManager) SyncWorkspaces(ctx context.Context) error {
	workspaces, err := sm.client.GetWorkspaces(ctx)
	if err != nil {
		return err
	}

	sm.UpdateState(func(s *AppState) {
		s.Workspaces = make(map[string]interface{})
		for _, ws := range workspaces {
			if id, ok := ws["id"].(string); ok {
				s.Workspaces[id] = ws
			}
		}
	})

	return nil
}

// SyncTools fetches available tools.
func (sm *StateManager) SyncTools(ctx context.Context) error {
	tools, err := sm.client.GetTools(ctx)
	if err != nil {
		return err
	}

	sm.UpdateState(func(s *AppState) {
		s.Tools = make(map[string]interface{})
		for _, tool := range tools {
			if id, ok := tool["id"].(string); ok {
				s.Tools[id] = tool
			}
		}
	})

	return nil
}

// SyncMemory fetches memory items.
func (sm *StateManager) SyncMemory(ctx context.Context) error {
	memory, err := sm.client.GetMemory(ctx)
	if err != nil {
		return err
	}

	sm.UpdateState(func(s *AppState) {
		s.Memory = make(map[string]interface{})
		for _, item := range memory {
			if id, ok := item["id"].(string); ok {
				s.Memory[id] = item
			}
		}
	})

	return nil
}

// SyncAgents fetches agent status.
func (sm *StateManager) SyncAgents(ctx context.Context) error {
	agents, err := sm.client.GetAgents(ctx)
	if err != nil {
		return err
	}

	sm.UpdateState(func(s *AppState) {
		s.Agents = make(map[string]interface{})
		for _, agent := range agents {
			if id, ok := agent["id"].(string); ok {
				s.Agents[id] = agent
			}
		}
	})

	return nil
}

// SyncMetrics fetches analytics metrics.
func (sm *StateManager) SyncMetrics(ctx context.Context) error {
	metrics, err := sm.client.GetAnalyticsMetrics(ctx)
	if err != nil {
		return err
	}

	sm.UpdateState(func(s *AppState) {
		s.Metrics = metrics
	})

	return nil
}

// SyncAll performs a full state synchronization.
func (sm *StateManager) SyncAll(ctx context.Context) error {
	// Check if we've synced recently
	if time.Since(sm.lastSyncTime) < 5*time.Second {
		return nil
	}

	sm.lastSyncTime = time.Now()

	// Sync critical data in parallel
	errCh := make(chan error, 6)

	go func() { errCh <- sm.SyncRuns(ctx) }()
	go func() { errCh <- sm.SyncWorkspaces(ctx) }()
	go func() { errCh <- sm.SyncTools(ctx) }()
	go func() { errCh <- sm.SyncMemory(ctx) }()
	go func() { errCh <- sm.SyncAgents(ctx) }()
	go func() { errCh <- sm.SyncMetrics(ctx) }()

	// Collect errors
	var lastErr error
	for i := 0; i < 6; i++ {
		if err := <-errCh; err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// StartPeriodicSync starts periodic state synchronization.
func (sm *StateManager) StartPeriodicSync() {
	go func() {
		ticker := time.NewTicker(sm.syncInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(sm.ctx, 10*time.Second)
				sm.SyncAll(ctx)
				cancel()

			case <-sm.ctx.Done():
				return
			}
		}
	}()
}

// Stop stops the state manager.
func (sm *StateManager) Stop() {
	sm.cancel()
}

// Helper function to generate session ID
func generateSessionID() string {
	return "sess_" + timeStampNano()
}

// Helper function for timestamp
func timeStampNano() string {
	return toBase62(time.Now().UnixNano())
}

// Simple base62 encoding
func toBase62(n int64) string {
	const base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	if n == 0 {
		return "0"
	}

	var result string
	for n > 0 {
		result = string(base62[n%62]) + result
		n /= 62
	}

	return result
}
