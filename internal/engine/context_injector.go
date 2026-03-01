// Package engine — context_injector.go
//
// Three-Tier Context Injection: automatically loads project-specific context
// into the conversation at session start without requiring the user to manually
// reference it.
//
// Inspired by oh-my-opencode's directory-agents-injector, directory-readme-injector,
// and rules-injector hooks, which inject relevant context files (AGENTS.md,
// README.md, .sisyphus/rules/) automatically so agents always have project context.
//
// Gorkbot's three tiers:
//
//  1. GORKBOT.md hierarchy — walk from cwd up to root, collect all GORKBOT.md files.
//     Inner directories take precedence (appended last, so they override earlier content).
//
//  2. README.md injection — if the cwd has a README.md, inject a summary context.
//
//  3. Rules files — inject any *.md files from .gorkbot/rules/ in cwd or above.
//
// All injected content is combined into a single system message added at the
// beginning of the conversation (before the user's first message).
package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ContextInjector walks the filesystem to collect project context files and
// returns a composite system prompt prefix.
type ContextInjector struct {
	// MaxFileSize limits how many bytes are read from any single context file.
	// Default: 32 KB per file.
	MaxFileSize int64

	// MaxTotalSize limits the total bytes of injected context.
	// Default: 128 KB.
	MaxTotalSize int64
}

// NewContextInjector creates a ContextInjector with sensible defaults.
func NewContextInjector() *ContextInjector {
	return &ContextInjector{
		MaxFileSize:  32 * 1024,
		MaxTotalSize: 128 * 1024,
	}
}

// InjectedContext holds the gathered context and metadata.
type InjectedContext struct {
	// SystemPromptPrefix is ready to prepend to the AI system prompt.
	SystemPromptPrefix string

	// Sources lists the file paths that were injected (for logging/debug).
	Sources []string

	// TotalBytes is the total bytes of injected text.
	TotalBytes int
}

// Collect gathers all context files starting from the given working directory.
// Returns an InjectedContext (which may be empty if no files are found).
func (ci *ContextInjector) Collect(workdir string) InjectedContext {
	if workdir == "" {
		workdir, _ = os.Getwd()
	}
	workdir, _ = filepath.Abs(workdir)

	var parts []string
	var sources []string
	totalBytes := 0

	addPart := func(header, content, path string) bool {
		blob := fmt.Sprintf("\n\n---\n### %s\n\n%s", header, content)
		if int64(totalBytes+len(blob)) > ci.MaxTotalSize {
			return false
		}
		parts = append(parts, blob)
		sources = append(sources, path)
		totalBytes += len(blob)
		return true
	}

	// ── Tier 1: GORKBOT.md hierarchy ─────────────────────────────────────────
	// Walk from workdir up to root, collect GORKBOT.md at each level.
	// We collect them root-first, then append — so innermost overrides.
	gorkbotFiles := ci.findFilesUpward(workdir, "GORKBOT.md")
	// Reverse so outermost (root) is first, innermost last (higher precedence).
	for i, j := 0, len(gorkbotFiles)-1; i < j; i, j = i+1, j-1 {
		gorkbotFiles[i], gorkbotFiles[j] = gorkbotFiles[j], gorkbotFiles[i]
	}
	for _, path := range gorkbotFiles {
		content := ci.readLimited(path)
		if content == "" {
			continue
		}
		rel := ci.relPath(workdir, path)
		addPart(fmt.Sprintf("Project Instructions (%s)", rel), content, path)
	}

	// ── Tier 2: README.md injection ──────────────────────────────────────────
	// Look for README.md in the working directory only (not hierarchy).
	readmePath := filepath.Join(workdir, "README.md")
	if readme := ci.readLimited(readmePath); readme != "" {
		// Trim README to first 4 KB to avoid overwhelming the context.
		trimmed := ci.truncate(readme, 4*1024)
		suffix := ""
		if len(readme) > 4*1024 {
			suffix = "\n\n[... README truncated for context brevity ...]"
		}
		addPart("Project README (summary context)", trimmed+suffix, readmePath)
	}

	// ── Tier 3: Rules files ───────────────────────────────────────────────────
	// Collect *.md from .gorkbot/rules/ walking upward from workdir.
	rulesDirs := ci.findDirsUpward(workdir, filepath.Join(".gorkbot", "rules"))
	// Deduplicate (same path might appear if symlinked).
	seen := make(map[string]bool)
	for _, dir := range rulesDirs {
		if seen[dir] {
			continue
		}
		seen[dir] = true
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			content := ci.readLimited(path)
			if content == "" {
				continue
			}
			rel := ci.relPath(workdir, path)
			addPart(fmt.Sprintf("Project Rule: %s", rel), content, path)
		}
	}

	if len(parts) == 0 {
		return InjectedContext{}
	}

	var header strings.Builder
	header.WriteString("═══════════════════════════════════════════════════════\n")
	header.WriteString("  PROJECT CONTEXT (auto-injected by Gorkbot)\n")
	header.WriteString(fmt.Sprintf("  %d file(s) | %d bytes\n", len(sources), totalBytes))
	header.WriteString("═══════════════════════════════════════════════════════")
	header.WriteString(strings.Join(parts, ""))
	header.WriteString("\n\n═══════════════════════════════════════════════════════\n")
	header.WriteString("  END OF PROJECT CONTEXT — respond to user below\n")
	header.WriteString("═══════════════════════════════════════════════════════\n")

	return InjectedContext{
		SystemPromptPrefix: header.String(),
		Sources:            sources,
		TotalBytes:         totalBytes,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// helpers
// ─────────────────────────────────────────────────────────────────────────────

// findFilesUpward walks from dir up to root and returns paths to files named
// filename found at each level. Returns paths from innermost first.
func (ci *ContextInjector) findFilesUpward(dir, filename string) []string {
	var found []string
	current := dir
	for {
		candidate := filepath.Join(current, filename)
		if _, err := os.Stat(candidate); err == nil {
			found = append(found, candidate)
		}
		parent := filepath.Dir(current)
		if parent == current {
			break // reached root
		}
		current = parent
	}
	return found
}

// findDirsUpward finds directories named dirName walking upward from dir.
func (ci *ContextInjector) findDirsUpward(dir, dirName string) []string {
	var found []string
	current := dir
	for {
		candidate := filepath.Join(current, dirName)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			found = append(found, candidate)
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return found
}

// readLimited reads a file up to MaxFileSize bytes. Returns "" on error.
func (ci *ContextInjector) readLimited(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	buf := make([]byte, ci.MaxFileSize)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return ""
	}
	return strings.TrimSpace(string(buf[:n]))
}

// truncate shortens text to maxBytes, respecting UTF-8 boundaries.
func (ci *ContextInjector) truncate(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	// Trim at rune boundary.
	return s[:maxBytes]
}

// relPath returns path relative to base, or path itself if it fails.
func (ci *ContextInjector) relPath(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return rel
}
