package session

import (
	"encoding/json"
	"fmt"
	"os"
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

// ListSessionFiles returns the base names (without .json) of all session files in dir.
func ListSessionFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, strings.TrimSuffix(e.Name(), ".json"))
		}
	}
	return names
}
