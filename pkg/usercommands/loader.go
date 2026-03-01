package usercommands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// UserCommand represents a user-defined slash command
type UserCommand struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Prompt      string `json:"prompt"` // may contain {{args}} placeholder
}

// Loader manages user-defined commands persisted to JSON
type Loader struct {
	mu       sync.RWMutex
	path     string
	commands map[string]UserCommand
}

// NewLoader creates a Loader backed by configDir/user_commands.json
func NewLoader(configDir string) (*Loader, error) {
	l := &Loader{
		path:     filepath.Join(configDir, "user_commands.json"),
		commands: make(map[string]UserCommand),
	}
	if err := l.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return l, nil
}

// Get returns the rendered prompt for a command, substituting {{args}}
// Returns (prompt, true) if found, ("", false) otherwise
func (l *Loader) Get(name, args string) (string, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	cmd, ok := l.commands[strings.ToLower(name)]
	if !ok {
		return "", false
	}
	prompt := strings.ReplaceAll(cmd.Prompt, "{{args}}", args)
	return prompt, true
}

// Define adds or updates a user command
func (l *Loader) Define(cmd UserCommand) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.commands[strings.ToLower(cmd.Name)] = cmd
	return l.save()
}

// Delete removes a user command by name
func (l *Loader) Delete(name string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.commands, strings.ToLower(name))
	return l.save()
}

// List returns all user commands sorted by name
func (l *Loader) List() []UserCommand {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]UserCommand, 0, len(l.commands))
	for _, c := range l.commands {
		out = append(out, c)
	}
	return out
}

func (l *Loader) load() error {
	data, err := os.ReadFile(l.path)
	if err != nil {
		return err
	}
	var list []UserCommand
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("usercommands: parse %s: %w", l.path, err)
	}
	for _, c := range list {
		l.commands[strings.ToLower(c.Name)] = c
	}
	return nil
}

func (l *Loader) save() error {
	list := make([]UserCommand, 0, len(l.commands))
	for _, c := range l.commands {
		list = append(list, c)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0700); err != nil {
		return err
	}
	return os.WriteFile(l.path, data, 0600)
}
