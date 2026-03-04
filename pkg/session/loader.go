package session

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/ai"
)

// sensitivePatterns matches common credential/PII patterns in message content.
// Each pattern replaces the captured value with [REDACTED].
var sensitivePatterns = []*regexp.Regexp{
	// Bearer / API tokens
	regexp.MustCompile(`(?i)(bearer[_\s:]+)[^\s"']+`),
	// Generic key=value credentials
	regexp.MustCompile(`(?i)(api[_-]?key[_\s:=]+)[^\s"'&,\n]+`),
	regexp.MustCompile(`(?i)(api[_-]?secret[_\s:=]+)[^\s"'&,\n]+`),
	regexp.MustCompile(`(?i)(access[_-]?token[_\s:=]+)[^\s"'&,\n]+`),
	regexp.MustCompile(`(?i)(secret[_\s:=]+)[^\s"'&,\n]+`),
	regexp.MustCompile(`(?i)(password[_\s:=]+)[^\s"'&,\n]+`),
	regexp.MustCompile(`(?i)(consumer[_-]?key[_\s:=]+)[^\s"'&,\n]+`),
	regexp.MustCompile(`(?i)(consumer[_-]?secret[_\s:=]+)[^\s"'&,\n]+`),
	// xAI / OpenAI style keys (long alphanumeric strings with prefix)
	regexp.MustCompile(`\bxai-[A-Za-z0-9_-]{20,}\b`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{20,}\b`),
	// Google API keys (AIza…)
	regexp.MustCompile(`\bAIza[A-Za-z0-9_-]{30,}\b`),
}

// redactContent replaces credential patterns in text with [REDACTED].
func redactContent(s string) string {
	for _, re := range sensitivePatterns {
		s = re.ReplaceAllStringFunc(s, func(match string) string {
			// For patterns with a prefix group, keep the prefix, redact the value.
			// For standalone token patterns, redact the whole match.
			sub := re.FindStringSubmatch(match)
			if len(sub) == 2 {
				return sub[1] + "[REDACTED]"
			}
			return "[REDACTED]"
		})
	}
	return s
}

// redactMessages returns a copy of msgs with sensitive content removed.
func redactMessages(msgs []ai.ConversationMessage) []ai.ConversationMessage {
	out := make([]ai.ConversationMessage, len(msgs))
	for i, m := range msgs {
		m.Content = redactContent(m.Content)
		out[i] = m
	}
	return out
}

// SessionFile is the on-disk format for named sessions saved by /save and /chat save.
type SessionFile struct {
	SavedAt  string                   `json:"saved_at"`
	Name     string                   `json:"name"`
	Messages []ai.ConversationMessage `json:"messages"`
}

// SessionMeta is a lightweight summary of a saved session for listing.
type SessionMeta struct {
	Name    string // human-readable name stored in the file
	Key     string // opaque filename key (without .json)
	SavedAt string
}

// sessionKey derives the on-disk filename (without extension) for a session name.
// A truncated SHA-256 makes filenames opaque to casual filesystem browsing.
// The human-readable name is always stored in the file's "name" field for lookup.
func sessionKey(name string) string {
	sum := sha256.Sum256([]byte("gorkbot\x00" + strings.ToLower(strings.TrimSpace(name))))
	return fmt.Sprintf("%x", sum[:8]) // 16 hex chars — 64-bit key space
}

// SaveSessionFile writes conversation messages to a JSON file at path.
// Sensitive credential patterns are automatically redacted before writing.
func SaveSessionFile(path, name string, msgs []ai.ConversationMessage) error {
	sf := SessionFile{
		SavedAt:  time.Now().Format(time.RFC3339),
		Name:     name,
		Messages: redactMessages(msgs),
	}
	data, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	return os.WriteFile(path, data, 0600)
}

// SaveSessionByName writes a session file using a key-derived filename.
// Returns the opaque key used, or an error.
func SaveSessionByName(dir, name string, msgs []ai.ConversationMessage) (string, error) {
	key := sessionKey(name)
	path := filepath.Join(dir, key+".json")
	return key, SaveSessionFile(path, name, msgs)
}

// LoadSessionFile reads a JSON session file and returns the messages.
func LoadSessionFile(path string) ([]ai.ConversationMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read session file: %w", err)
	}
	var sf SessionFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("parse session file: %w", err)
	}
	return sf.Messages, nil
}

// FindSessionFile scans dir for a session file whose internal name field matches name
// (case-insensitive). Returns the full path, or "" if not found.
func FindSessionFile(dir, name string) string {
	target := strings.ToLower(strings.TrimSpace(name))

	// Fast path: try the deterministic key first (avoids scanning all files).
	fastPath := filepath.Join(dir, sessionKey(name)+".json")
	if data, err := os.ReadFile(fastPath); err == nil {
		var sf SessionFile
		if json.Unmarshal(data, &sf) == nil && strings.ToLower(sf.Name) == target {
			return fastPath
		}
	}

	// Fallback: scan all .json files (handles legacy files without a key-named path).
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var sf SessionFile
		if err := json.Unmarshal(data, &sf); err != nil {
			continue
		}
		if strings.ToLower(sf.Name) == target {
			return path
		}
	}
	return ""
}

// ListSessionMetas returns metadata for all saved sessions in dir,
// sorted newest-first by saved_at timestamp.
func ListSessionMetas(dir string) []SessionMeta {
	entries, _ := os.ReadDir(dir)
	var metas []SessionMeta
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var sf SessionFile
		if err := json.Unmarshal(data, &sf); err != nil {
			continue
		}
		key := strings.TrimSuffix(e.Name(), ".json")
		name := sf.Name
		if name == "" {
			name = key // legacy files stored name as filename
		}
		metas = append(metas, SessionMeta{Name: name, Key: key, SavedAt: sf.SavedAt})
	}
	return metas
}

// ListSessionFiles returns the human-readable names of all sessions in dir.
// Kept for backward compatibility; prefer ListSessionMetas for richer data.
func ListSessionFiles(dir string) []string {
	metas := ListSessionMetas(dir)
	names := make([]string, len(metas))
	for i, m := range metas {
		names[i] = m.Name
	}
	return names
}
