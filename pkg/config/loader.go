// Package config loads hierarchical project-level configuration from GORKBOT.md files.
//
// Discovery order (lowest → highest priority):
//  1. ~/.config/gorkbot/GLOBAL.md        — user-global preferences (all projects)
//  2. ~/.config/gorkbot/GLOBAL.local.md  — personal overrides (gitignored)
//  3. <project_root>/GORKBOT.md          — project-level (commit to repo)
//  4. <project_root>/GORKBOT.local.md    — personal per-project (gitignored)
//  5. <project_root>/.gorkbot/rules/*.md — modular topic rules
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Loader discovers and loads GORKBOT.md instruction files.
type Loader struct {
	globalConfigDir string // e.g. ~/.config/gorkbot
	cwd             string // working directory (project root detection)
}

// NewLoader creates a Loader.
// configDir is the global config directory (platform.GetEnvConfig().ConfigDir).
// cwd is the working directory to search for project-level files.
func NewLoader(configDir, cwd string) *Loader {
	return &Loader{
		globalConfigDir: configDir,
		cwd:             cwd,
	}
}

// LoadInstructions returns the concatenated content of all discovered
// GORKBOT.md / GORKBOT.local.md / rules/*.md files, in priority order.
func (l *Loader) LoadInstructions() string {
	var parts []string

	// 1. Global user preferences
	if s := readFile(filepath.Join(l.globalConfigDir, "GLOBAL.md")); s != "" {
		parts = append(parts, header("Global Instructions", l.globalConfigDir+"/GLOBAL.md")+s)
	}
	if s := readFile(filepath.Join(l.globalConfigDir, "GLOBAL.local.md")); s != "" {
		parts = append(parts, header("Global Local Overrides", l.globalConfigDir+"/GLOBAL.local.md")+s)
	}

	// 2. Walk up from cwd to find project-level GORKBOT.md
	projectRoot := l.findProjectRoot()
	if projectRoot != "" {
		if s := readFile(filepath.Join(projectRoot, "GORKBOT.md")); s != "" {
			parts = append(parts, header("Project Instructions", projectRoot+"/GORKBOT.md")+s)
		}
		if s := readFile(filepath.Join(projectRoot, "GORKBOT.local.md")); s != "" {
			parts = append(parts, header("Project Local Overrides", projectRoot+"/GORKBOT.local.md")+s)
		}

		// 3. Modular rules from .gorkbot/rules/*.md
		rulesDir := filepath.Join(projectRoot, ".gorkbot", "rules")
		if entries, err := os.ReadDir(rulesDir); err == nil {
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
					path := filepath.Join(rulesDir, e.Name())
					if s := readFile(path); s != "" {
						parts = append(parts, header("Rule: "+e.Name(), path)+s)
					}
				}
			}
		}
	}

	if len(parts) == 0 {
		return ""
	}

	return "### PROJECT INSTRUCTIONS (from GORKBOT.md):\n\n" +
		strings.Join(parts, "\n\n---\n\n") +
		"\n\n"
}

// FindProjectRoot returns the path where GORKBOT.md was found by walking up
// from cwd. Returns "" if no GORKBOT.md found.
func (l *Loader) findProjectRoot() string {
	dir := l.cwd
	for {
		if fileExists(filepath.Join(dir, "GORKBOT.md")) ||
			fileExists(filepath.Join(dir, "GORKBOT.local.md")) ||
			fileExists(filepath.Join(dir, ".gorkbot")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break // filesystem root
		}
		dir = parent
	}
	return ""
}

// CreateExample writes a template GORKBOT.md in the current directory.
func (l *Loader) CreateExample() error {
	path := filepath.Join(l.cwd, "GORKBOT.md")
	if fileExists(path) {
		return fmt.Errorf("GORKBOT.md already exists at %s", path)
	}
	template := `# Project Instructions for Gorkbot

## Project Overview
<!-- Describe what this project is and its tech stack -->

## Build & Run
<!-- How to build, test, and run the project -->
<!-- Example:
Build: go build ./cmd/myapp/
Test:  go test ./...
Run:   ./myapp
-->

## Coding Standards
<!-- Your project's coding conventions -->

## Tool Behavior
<!-- Customize how Gorkbot uses tools in this project -->
<!-- Example: Never auto-commit. Always show git diff before committing. -->

## Context
<!-- Any important context the AI should always know -->
`
	return os.WriteFile(path, []byte(template), 0644)
}

// LoadSubdirInstructions returns the GORKBOT.md content for the directory
// containing filePath. Returns "" if no GORKBOT.md is found in that directory.
// This enables lazy injection of subdirectory-specific instructions when a file
// in that directory is first accessed.
func (l *Loader) LoadSubdirInstructions(filePath string) string {
	dir := filepath.Dir(filePath)
	// Resolve to absolute path.
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(l.cwd, dir)
	}
	path := filepath.Join(dir, "GORKBOT.md")
	return readFile(path)
}

// ActiveFiles returns the list of config files that are currently loaded.
func (l *Loader) ActiveFiles() []string {
	var files []string
	candidates := []string{
		filepath.Join(l.globalConfigDir, "GLOBAL.md"),
		filepath.Join(l.globalConfigDir, "GLOBAL.local.md"),
	}

	projectRoot := l.findProjectRoot()
	if projectRoot != "" {
		candidates = append(candidates,
			filepath.Join(projectRoot, "GORKBOT.md"),
			filepath.Join(projectRoot, "GORKBOT.local.md"),
		)
		rulesDir := filepath.Join(projectRoot, ".gorkbot", "rules")
		if entries, err := os.ReadDir(rulesDir); err == nil {
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
					candidates = append(candidates, filepath.Join(rulesDir, e.Name()))
				}
			}
		}
	}

	for _, f := range candidates {
		if fileExists(f) {
			files = append(files, f)
		}
	}
	return files
}

func readFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func header(label, path string) string {
	return fmt.Sprintf("<!-- %s: %s -->\n", label, path)
}
