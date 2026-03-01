package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Manager discovers and fires hook scripts at lifecycle events.
//
// Hook scripts live in: <configDir>/hooks/<event_name>.sh
// They receive a JSON Payload on stdin and communicate via:
//   - Exit code 0: proceed
//   - Exit code 2: block (stderr = reason shown to user)
//   - Other exit: warning logged, execution continues
type Manager struct {
	hooksDir string
	timeout  time.Duration
	logger   *slog.Logger
	enabled  bool
}

// NewManager creates a Manager looking for hooks in <configDir>/hooks/.
func NewManager(configDir string, logger *slog.Logger) *Manager {
	hooksDir := filepath.Join(configDir, "hooks")
	_ = os.MkdirAll(hooksDir, 0755)
	return &Manager{
		hooksDir: hooksDir,
		timeout:  15 * time.Second,
		logger:   logger,
		enabled:  true,
	}
}

// SetEnabled enables or disables hook execution globally.
func (m *Manager) SetEnabled(v bool) { m.enabled = v }

// Fire executes the hook script for the given event (if it exists).
// Returns a HookResult; Blocked=true means the caller should abort the action.
func (m *Manager) Fire(ctx context.Context, event Event, payload Payload) HookResult {
	if !m.enabled {
		return HookResult{}
	}

	payload.Event = event
	payload.Timestamp = time.Now()

	scriptPath := filepath.Join(m.hooksDir, string(event)+".sh")
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return HookResult{} // No hook installed — proceed
	}

	// Verify script is not world-writable (basic security check)
	info, err := os.Stat(scriptPath)
	if err != nil {
		m.logger.Warn("hook stat failed", "event", event, "error", err)
		return HookResult{}
	}
	if info.Mode()&0002 != 0 {
		m.logger.Warn("hook script is world-writable — skipping for security", "path", scriptPath)
		return HookResult{}
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		m.logger.Warn("hook payload marshal failed", "event", event, "error", err)
		return HookResult{}
	}

	tctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	cmd := exec.CommandContext(tctx, "/bin/sh", scriptPath)
	cmd.Stdin = bytes.NewReader(payloadJSON)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	reason := strings.TrimSpace(stderr.String())

	if runErr == nil {
		m.logger.Debug("hook completed", "event", event)
		return HookResult{}
	}

	// Check exit code
	if exitErr, ok := runErr.(*exec.ExitError); ok {
		code := exitErr.ExitCode()
		if code == 2 {
			m.logger.Info("hook blocked action", "event", event, "reason", reason)
			return HookResult{Blocked: true, Reason: reason}
		}
		m.logger.Warn("hook exited with non-zero code", "event", event, "code", code, "stderr", reason)
		return HookResult{}
	}

	// Context timeout or other error
	m.logger.Warn("hook execution error", "event", event, "error", runErr)
	return HookResult{Err: runErr}
}

// FireAsync fires a hook in a goroutine (non-blocking). Useful for post-events
// where blocking is undesirable.
func (m *Manager) FireAsync(ctx context.Context, event Event, payload Payload) {
	go func() {
		m.Fire(ctx, event, payload)
	}()
}

// InstallExample writes an example hook script for the given event.
func (m *Manager) InstallExample(event Event) error {
	path := filepath.Join(m.hooksDir, string(event)+".sh")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("hook already exists: %s", path)
	}

	content := fmt.Sprintf(`#!/bin/sh
# Gorkbot hook: %s
# Receives JSON payload on stdin.
# Exit 0 to proceed, exit 2 to block (add reason to stderr).

# Read the JSON payload
payload=$(cat)

# Example: log event to a file
# echo "$payload" >> /tmp/gorkbot_hooks.log

# Example: block a dangerous bash command
# if echo "$payload" | grep -q '"rm -rf"'; then
#   echo "Blocked: dangerous rm command" >&2
#   exit 2
# fi

exit 0
`, event)

	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		return fmt.Errorf("write hook: %w", err)
	}
	return nil
}

// ListInstalled returns the names of all installed hook scripts.
func (m *Manager) ListInstalled() []string {
	entries, err := os.ReadDir(m.hooksDir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sh") {
			names = append(names, strings.TrimSuffix(e.Name(), ".sh"))
		}
	}
	return names
}

// HooksDir returns the hooks directory path.
func (m *Manager) HooksDir() string { return m.hooksDir }
