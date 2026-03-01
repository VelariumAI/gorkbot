// Package skills loads user-defined skill definitions from markdown files
// with YAML frontmatter and makes them available as slash commands.
//
// Skill files live in:
//   ~/.config/gorkbot/skills/          (user-global)
//   <project>/.gorkbot/skills/         (project-level)
//
// Format example (.gorkbot/skills/code-review.md):
//
//	---
//	name: code-review
//	description: Thorough code review with security focus
//	aliases: [cr, review]
//	tools: [read_file, grep_content, bash]
//	model: grok-3
//	---
//
//	Review {{target}} for security issues, logic errors, and code quality.
package skills

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed builtin/*.md
var builtinSkills embed.FS

// Definition holds a parsed skill definition.
type Definition struct {
	Name        string   // Canonical slash command name
	Description string   // Short description
	Aliases     []string // Alternative names
	Tools       []string // Allowed tools (empty = all)
	Model       string   // Override model (empty = default)
	Template    string   // Prompt template body
	SourceFile  string   // Path to the .md file
}

// Loader discovers and parses skill definitions from configured directories.
type Loader struct {
	dirs []string
}

// NewLoader creates a Loader that searches the given directories.
// Typical usage: NewLoader(globalConfigDir+"/skills", projectRoot+"/.gorkbot/skills")
func NewLoader(dirs ...string) *Loader {
	return &Loader{dirs: dirs}
}

// LoadAll returns all skill definitions found across all configured directories.
// Project-level skills override global skills with the same name.
func (l *Loader) LoadAll() []Definition {
	byName := map[string]Definition{}

	// 1. Load built-in skills
	if entries, err := builtinSkills.ReadDir("builtin"); err == nil {
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			path := "builtin/" + e.Name()
			data, err := builtinSkills.ReadFile(path)
			if err != nil {
				continue
			}
			def, err := parseSkillContent(data, path)
			if err == nil && def.Name != "" {
				byName[def.Name] = def
			}
		}
	}

	// 2. Load user/project skills (overrides built-ins)
	for _, dir := range l.dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			def, err := parseSkillFile(path)
			if err != nil || def.Name == "" {
				continue
			}
			byName[def.Name] = def
		}
	}

	result := make([]Definition, 0, len(byName))
	for _, def := range byName {
		result = append(result, def)
	}
	return result
}

// Get returns the named skill (checking canonical name and aliases).
func (l *Loader) Get(name string) (Definition, bool) {
	for _, def := range l.LoadAll() {
		if def.Name == name {
			return def, true
		}
		for _, alias := range def.Aliases {
			if alias == name {
				return def, true
			}
		}
	}
	return Definition{}, false
}

// Render expands template variables in a skill definition's template.
// Variables: {{target}}, {{args}}, {{date}}
func (def *Definition) Render(args string) string {
	t := def.Template
	t = strings.ReplaceAll(t, "{{target}}", args)
	t = strings.ReplaceAll(t, "{{args}}", args)
	return t
}

// Format returns a human-readable list of all skills.
func (l *Loader) Format() string {
	skills := l.LoadAll()
	if len(skills) == 0 {
		return fmt.Sprintf(
			"No skills installed.\n\nCreate skill files in:\n  ~/.config/gorkbot/skills/\n  .gorkbot/skills/\n\nSee `/skills help` for the file format.\n",
		)
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Skills (%d installed)\n\n", len(skills)))
	for _, s := range skills {
		sb.WriteString(fmt.Sprintf("**/%s**", s.Name))
		if len(s.Aliases) > 0 {
			aliases := make([]string, len(s.Aliases))
			for i, a := range s.Aliases {
				aliases[i] = "/" + a
			}
			sb.WriteString(fmt.Sprintf(" (also: %s)", strings.Join(aliases, ", ")))
		}
		sb.WriteString(fmt.Sprintf("\n  %s\n", s.Description))
		if s.Model != "" {
			sb.WriteString(fmt.Sprintf("  Model: `%s`\n", s.Model))
		}
		sb.WriteString(fmt.Sprintf("  Source: `%s`\n\n", s.SourceFile))
	}
	sb.WriteString("---\n**Usage:** `/<skill-name> [arguments]`\n")
	return sb.String()
}

// CreateExample writes a template skill file.
func CreateExample(dir, name string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, name+".md")
	content := fmt.Sprintf(`---
name: %s
description: Describe what this skill does
aliases: []
tools: []
model: ""
---

# %s Skill

Analyze {{target}} and provide:
1. A structured summary
2. Key findings
3. Actionable recommendations
`, name, strings.Title(name))
	return os.WriteFile(path, []byte(content), 0644)
}

// parseSkillFile reads a skill markdown file and parses its frontmatter.
func parseSkillFile(path string) (Definition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Definition{}, err
	}
	return parseSkillContent(data, path)
}

func parseSkillContent(data []byte, path string) (Definition, error) {
	content := string(data)
	def := Definition{SourceFile: path}

	// Extract YAML frontmatter between --- delimiters
	if !strings.HasPrefix(content, "---") {
		// No frontmatter — use filename as name, whole file as template
		def.Name = strings.TrimSuffix(filepath.Base(path), ".md")
		def.Template = strings.TrimSpace(content)
		return def, nil
	}

	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		return def, fmt.Errorf("invalid frontmatter")
	}

	yaml := strings.TrimSpace(parts[1])
	def.Template = strings.TrimSpace(parts[2])

	// Simple YAML parser (no external dependency)
	for _, line := range strings.Split(yaml, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kv := strings.SplitN(line, ":", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])

		switch key {
		case "name":
			def.Name = strings.Trim(val, `"'`)
		case "description":
			def.Description = strings.Trim(val, `"'`)
		case "model":
			def.Model = strings.Trim(val, `"'`)
		case "aliases":
			// Parse: [cr, review] or ["cr", "review"]
			val = strings.Trim(val, "[]")
			for _, a := range strings.Split(val, ",") {
				a = strings.Trim(strings.TrimSpace(a), `"'`)
				if a != "" {
					def.Aliases = append(def.Aliases, a)
				}
			}
		case "tools":
			val = strings.Trim(val, "[]")
			for _, t := range strings.Split(val, ",") {
				t = strings.Trim(strings.TrimSpace(t), `"'`)
				if t != "" {
					def.Tools = append(def.Tools, t)
				}
			}
		}
	}

	if def.Name == "" {
		def.Name = strings.TrimSuffix(filepath.Base(path), ".md")
	}
	return def, nil
}
