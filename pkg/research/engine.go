package research

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Engine provides three research primitives: Search, Open, Find.
// Document content stays in the buffer and never enters conversation history.
type Engine struct {
	buffer *DocBuffer
	logger *slog.Logger
}

// NewEngine creates a research engine with the given buffer capacity.
func NewEngine(maxDocs int, logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{
		buffer: NewDocBuffer(maxDocs),
		logger: logger,
	}
}

// Search performs a web search and returns structured results (no page content).
func (e *Engine) Search(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	if topK <= 0 {
		topK = 10
	}

	// DuckDuckGo lite via curl
	escaped := shellEscape(query)
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c",
		fmt.Sprintf(`curl -sL 'https://lite.duckduckgo.com/lite/?q=%s' | sed 's/<[^>]*>//g' | grep -v '^\s*$' | head -n 100`, escaped))

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &bytes.Buffer{}

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Parse the text output into results
	return e.parseSearchResults(out.String(), topK), nil
}

// Open fetches a URL, stores content in the buffer, and returns a summary.
// The full content stays in the buffer — only the summary enters conversation.
func (e *Engine) Open(ctx context.Context, url string) (*DocumentSummary, error) {
	content, title, err := e.fetchPage(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}

	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
	doc := &Document{
		URL:         url,
		Title:       title,
		Content:     content,
		Length:      len(content),
		FetchedAt:   time.Now(),
		ContentHash: hash,
	}

	e.buffer.Store(doc)
	e.buffer.SetActive(url)

	summary := &DocumentSummary{
		URL:      doc.URL,
		Title:    doc.Title,
		Length:   doc.Length,
		IsActive: true,
	}

	return summary, nil
}

// Find searches the active document for a pattern and returns matched excerpts.
func (e *Engine) Find(pattern string, contextLines int) ([]FindMatch, error) {
	doc := e.buffer.Active()
	if doc == nil {
		return nil, fmt.Errorf("no active document — use browser_open first")
	}

	if contextLines <= 0 {
		contextLines = 2
	}

	lines := strings.Split(doc.Content, "\n")

	// Try regex, fall back to literal match
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		// Fall back to case-insensitive literal
		re = regexp.MustCompile("(?i)" + regexp.QuoteMeta(pattern))
	}

	var matches []FindMatch
	for i, line := range lines {
		loc := re.FindStringIndex(line)
		if loc == nil {
			continue
		}

		// Build context window
		start := i - contextLines
		if start < 0 {
			start = 0
		}
		end := i + contextLines + 1
		if end > len(lines) {
			end = len(lines)
		}

		matches = append(matches, FindMatch{
			LineNumber:  i + 1,
			MatchText:   line[loc[0]:loc[1]],
			Context:     strings.Join(lines[start:end], "\n"),
			StartOffset: loc[0],
		})

		// Cap at 20 matches to prevent context bloat
		if len(matches) >= 20 {
			break
		}
	}

	return matches, nil
}

// ListBuffered returns summaries of all documents in the buffer.
func (e *Engine) ListBuffered() []DocumentSummary {
	return e.buffer.List()
}

// ── Internal helpers ─────────────────────────────────────────────────────────

// fetchPage tries scrapling → lynx → curl fallback chain.
func (e *Engine) fetchPage(ctx context.Context, url string) (content, title string, err error) {
	escaped := shellEscape(url)

	// Try lynx first (most common on Termux)
	content, err = e.runFetch(ctx,
		fmt.Sprintf("lynx -dump -nolist %s 2>/dev/null", escaped))
	if err == nil && len(content) > 100 {
		title = extractTitle(content)
		return content, title, nil
	}

	// Fallback: curl + html tag stripping
	content, err = e.runFetch(ctx,
		fmt.Sprintf("curl -sL -m 30 %s | sed 's/<[^>]*>//g' | grep -v '^\\s*$' | head -n 500", escaped))
	if err == nil && len(content) > 50 {
		title = extractTitle(content)
		return content, title, nil
	}

	return "", "", fmt.Errorf("all fetch methods failed for %s", url)
}

func (e *Engine) runFetch(ctx context.Context, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &bytes.Buffer{}
	if err := cmd.Run(); err != nil {
		return "", err
	}
	result := out.String()
	// Truncate to 50k chars
	if len(result) > 50000 {
		result = result[:50000]
	}
	return result, nil
}

func (e *Engine) parseSearchResults(text string, topK int) []SearchResult {
	var results []SearchResult
	lines := strings.Split(text, "\n")

	rank := 1
	for i := 0; i < len(lines) && len(results) < topK; i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		// Look for URL-like patterns followed by descriptive text
		if strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://") {
			sr := SearchResult{
				URL:  line,
				Rank: rank,
			}
			// Next non-empty line is likely the title/snippet
			for j := i + 1; j < len(lines) && j < i+3; j++ {
				next := strings.TrimSpace(lines[j])
				if next != "" {
					if sr.Title == "" {
						sr.Title = next
					} else if sr.Snippet == "" {
						sr.Snippet = next
					}
				}
			}
			if sr.Title == "" {
				sr.Title = sr.URL
			}
			results = append(results, sr)
			rank++
		}
	}

	// If structured parsing didn't find enough, grab text chunks as results
	if len(results) == 0 {
		for i := 0; i < len(lines) && len(results) < topK; i++ {
			line := strings.TrimSpace(lines[i])
			if len(line) > 20 {
				results = append(results, SearchResult{
					Title:   line,
					Snippet: line,
					Rank:    rank,
				})
				rank++
			}
		}
	}

	return results
}

func extractTitle(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) > 5 && len(trimmed) < 200 {
			return trimmed
		}
	}
	return "Untitled"
}

func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
