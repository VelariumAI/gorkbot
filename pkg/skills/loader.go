// Package skills loads user-defined skill definitions from markdown files
// with YAML frontmatter and makes them available as slash commands.
//
// Skill files live in:
//
//	~/.config/gorkbot/skills/          (user-global)
//	<project>/.gorkbot/skills/         (project-level)
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
	"regexp"
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

// validSkillName is the compiled regexp for skill name validation.
var validSkillName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// FirstUserDir returns the first configured user directory (or "" if none).
// This is the preferred target for writing new skill files.
func (l *Loader) FirstUserDir() string {
	for _, d := range l.dirs {
		return d
	}
	return ""
}

// Create writes a new skill file to the first writable user directory.
// name must match ^[a-z0-9][a-z0-9._-]{0,63}$ and must not already exist.
// If content is empty a minimal template is generated from name and description.
func (l *Loader) Create(name, description, content string, tags, platforms []string) error {
	if !validSkillName.MatchString(name) {
		return fmt.Errorf("invalid skill name %q: must match ^[a-z0-9][a-z0-9._-]{0,63}$", name)
	}
	if strings.Contains(name, "..") || strings.Contains(name, "/") {
		return fmt.Errorf("invalid skill name %q: must not contain '..' or '/'", name)
	}

	// Determine target directory — first existing dir or create it.
	dir := l.FirstUserDir()
	if dir == "" {
		return fmt.Errorf("no user skill directories configured")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cannot create skill directory %q: %w", dir, err)
	}

	destPath := filepath.Join(dir, name+".md")
	if _, err := os.Stat(destPath); err == nil {
		return fmt.Errorf("skill %q already exists at %s", name, destPath)
	}

	if content == "" {
		// Generate minimal frontmatter template.
		tagsYAML := "[]"
		if len(tags) > 0 {
			tagsYAML = "[" + strings.Join(tags, ", ") + "]"
		}
		platformsYAML := "[]"
		if len(platforms) > 0 {
			platformsYAML = "[" + strings.Join(platforms, ", ") + "]"
		}
		if description == "" {
			description = name + " skill"
		}
		content = fmt.Sprintf("---\nname: %s\ndescription: %s\ntags: %s\nplatforms: %s\n---\n\n%s skill template.\n",
			name, description, tagsYAML, platformsYAML, name)
	}

	return os.WriteFile(destPath, []byte(content), 0644)
}

// Patch finds skill 'name' and does a find-and-replace of oldText→newText.
// Returns an error if the skill is not found or oldText is not present.
func (l *Loader) Patch(name, oldText, newText string) error {
	def, ok := l.Get(name)
	if !ok {
		return fmt.Errorf("skill %q not found", name)
	}
	if def.SourceFile == "" {
		return fmt.Errorf("skill %q has no source file (built-in skills cannot be patched)", name)
	}

	data, err := os.ReadFile(def.SourceFile)
	if err != nil {
		return fmt.Errorf("cannot read skill file %q: %w", def.SourceFile, err)
	}

	original := string(data)
	if !strings.Contains(original, oldText) {
		return fmt.Errorf("old_text %q not found in skill %q", oldText, name)
	}

	// Replace only the first occurrence.
	updated := strings.Replace(original, oldText, newText, 1)
	return os.WriteFile(def.SourceFile, []byte(updated), 0644)
}

// Delete removes skill 'name' from the filesystem.
// Only deletes skills whose SourceFile is under one of the configured dirs.
// Built-in (embedded) skills cannot be deleted.
func (l *Loader) Delete(name string) error {
	def, ok := l.Get(name)
	if !ok {
		return fmt.Errorf("skill %q not found", name)
	}
	if def.SourceFile == "" {
		return fmt.Errorf("skill %q is a built-in skill and cannot be deleted", name)
	}

	// Verify the source file is under one of our configured dirs.
	cleanSource := filepath.Clean(def.SourceFile)
	allowed := false
	for _, d := range l.dirs {
		cleanDir := filepath.Clean(d)
		if !strings.HasSuffix(cleanDir, string(filepath.Separator)) {
			cleanDir += string(filepath.Separator)
		}
		if strings.HasPrefix(cleanSource+string(filepath.Separator), cleanDir) {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("skill %q source file %q is outside configured skill directories", name, def.SourceFile)
	}

	return os.Remove(cleanSource)
}

// View returns the full raw content of skill 'name'.
// For user-defined skills it reads the source file; for built-in skills it
// reads from the embedded filesystem.
func (l *Loader) View(name string) (string, error) {
	def, ok := l.Get(name)
	if !ok {
		return "", fmt.Errorf("skill %q not found", name)
	}

	// Built-in skills have SourceFile set to "builtin/<name>.md" (embed path).
	if def.SourceFile != "" && !filepath.IsAbs(def.SourceFile) {
		// Likely an embedded path — try the embedded FS.
		data, err := builtinSkills.ReadFile(def.SourceFile)
		if err == nil {
			return string(data), nil
		}
	}

	if def.SourceFile == "" {
		// Reconstruct from parsed fields if no source file.
		return fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n%s\n",
			def.Name, def.Description, def.Template), nil
	}

	data, err := os.ReadFile(def.SourceFile)
	if err != nil {
		return "", fmt.Errorf("cannot read skill file %q: %w", def.SourceFile, err)
	}
	return string(data), nil
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
