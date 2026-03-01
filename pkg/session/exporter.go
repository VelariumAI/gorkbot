package session

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/ai"
)

// ExportFormat defines the output format for conversation export.
type ExportFormat string

const (
	ExportMarkdown ExportFormat = "markdown"
	ExportJSON     ExportFormat = "json"
	ExportPlain    ExportFormat = "plain"
)

// Exporter converts conversation history to various formats.
type Exporter struct{}

// NewExporter creates an Exporter.
func NewExporter() *Exporter { return &Exporter{} }

// Export converts history to the specified format and optionally writes to path.
// If path is empty, returns the content as a string.
func (e *Exporter) Export(history *ai.ConversationHistory, format ExportFormat, path string) (string, error) {
	msgs := history.GetMessages()

	var content string
	var err error

	switch format {
	case ExportMarkdown:
		content, err = e.toMarkdown(msgs)
	case ExportPlain:
		content, err = e.toPlain(msgs)
	case ExportJSON:
		content, err = e.toJSON(msgs)
	default:
		return "", fmt.Errorf("unknown format: %s", format)
	}

	if err != nil {
		return "", err
	}

	if path != "" {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return "", fmt.Errorf("write export: %w", err)
		}
		return fmt.Sprintf("Exported %d messages to %s", len(msgs), path), nil
	}

	return content, nil
}

func (e *Exporter) toMarkdown(msgs []ai.ConversationMessage) (string, error) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Gorkbot Session Export\n\n*Exported: %s*\n\n---\n\n",
		time.Now().Format("2006-01-02 15:04:05")))

	for _, m := range msgs {
		switch m.Role {
		case "user":
			sb.WriteString(fmt.Sprintf("## User\n\n%s\n\n", m.Content))
			sb.WriteString("---\n\n")
		case "assistant":
			sb.WriteString(fmt.Sprintf("## Assistant\n\n%s\n\n", m.Content))
			sb.WriteString("---\n\n")
		case "system":
			// Skip system messages in markdown export
		}
	}
	return sb.String(), nil
}

func (e *Exporter) toPlain(msgs []ai.ConversationMessage) (string, error) {
	var sb strings.Builder
	for _, m := range msgs {
		if m.Role == "system" {
			continue
		}
		prefix := "User"
		if m.Role == "assistant" {
			prefix = "Assistant"
		}
		sb.WriteString(fmt.Sprintf("[%s]\n%s\n\n", prefix, m.Content))
	}
	return sb.String(), nil
}

func (e *Exporter) toJSON(msgs []ai.ConversationMessage) (string, error) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`{"exported_at":%q,"messages":[`, time.Now().Format(time.RFC3339)))
	for i, m := range msgs {
		if i > 0 {
			sb.WriteString(",")
		}
		content := strings.ReplaceAll(m.Content, `"`, `\"`)
		content = strings.ReplaceAll(content, "\n", `\n`)
		sb.WriteString(fmt.Sprintf(`{"role":%q,"content":%q}`, m.Role, content))
	}
	sb.WriteString("]}")
	return sb.String(), nil
}
