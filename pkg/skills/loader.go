package skills

import (
	"fmt"
	"io/ioutil"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var semverPattern = regexp.MustCompile(`^\d+\.\d+\.\d+(?:[-+][A-Za-z0-9.\-]+)?$`)
var schemaVersionPattern = regexp.MustCompile(`^\d+\.\d+$`)

// Loader handles loading and parsing skill manifests.
type Loader struct {
	registry Registry
	logger   *slog.Logger
}

// NewLoader creates a new skill loader.
func NewLoader(registry Registry, logger *slog.Logger) *Loader {
	if logger == nil {
		logger = slog.Default()
	}

	return &Loader{
		registry: registry,
		logger:   logger,
	}
}

// LoadManifest parses a .gorkskill.yaml file.
func (l *Loader) LoadManifest(filePath string) (*SkillManifest, error) {
	// Read file
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	// Parse YAML
	var manifest SkillManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Validate
	if err := l.validate(&manifest); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Set timestamps if not provided
	if manifest.Created.IsZero() {
		manifest.Created = time.Now()
	}
	manifest.Modified = time.Now()
	if manifest.SchemaVersion == "" {
		manifest.SchemaVersion = "1.0"
	}

	// Default enabled to true
	if !manifest.Enabled {
		manifest.Enabled = true
	}

	return &manifest, nil
}

// LoadDirectory scans a directory for .gorkskill.yaml files and loads them.
func (l *Loader) LoadDirectory(dirPath string) (int, error) {
	entries, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read directory: %w", err)
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			// Recursively check subdirectories
			skillFile := filepath.Join(entry.Name(), ".gorkskill.yaml")
			fullPath := filepath.Join(dirPath, skillFile)
			if _, err := os.Stat(fullPath); err == nil {
				manifest, err := l.LoadManifest(fullPath)
				if err != nil {
					l.logger.Warn("failed to load skill", "file", fullPath, "error", err)
					continue
				}

				if err := l.registry.Register(manifest, fullPath); err != nil {
					l.logger.Warn("failed to register skill", "name", manifest.Name, "error", err)
					continue
				}

				count++
			}
		} else if entry.Name() == ".gorkskill.yaml" {
			fullPath := filepath.Join(dirPath, entry.Name())
			manifest, err := l.LoadManifest(fullPath)
			if err != nil {
				l.logger.Warn("failed to load skill", "file", fullPath, "error", err)
				continue
			}

			if err := l.registry.Register(manifest, fullPath); err != nil {
				l.logger.Warn("failed to register skill", "name", manifest.Name, "error", err)
				continue
			}

			count++
		}
	}

	l.logger.Info("loaded skills from directory", "dir", dirPath, "count", count)
	return count, nil
}

// validate checks required fields.
func (l *Loader) validate(manifest *SkillManifest) error {
	if manifest.Name == "" {
		return fmt.Errorf("name is required")
	}
	if manifest.Version == "" {
		return fmt.Errorf("version is required")
	}
	if !semverPattern.MatchString(manifest.Version) {
		return fmt.Errorf("version must be semantic version (e.g. 1.2.3)")
	}
	if manifest.SchemaVersion != "" && !schemaVersionPattern.MatchString(manifest.SchemaVersion) {
		return fmt.Errorf("schema_version must be in major.minor format (e.g. 1.0)")
	}
	if manifest.Description == "" {
		return fmt.Errorf("description is required")
	}

	// Validate workflows
	for _, wf := range manifest.Workflows {
		if wf.Name == "" {
			return fmt.Errorf("workflow name is required")
		}
		if len(wf.Steps) == 0 {
			return fmt.Errorf("workflow %s has no steps", wf.Name)
		}

		// Validate steps
		for _, step := range wf.Steps {
			if step.ID == "" {
				return fmt.Errorf("step ID is required in workflow %s", wf.Name)
			}
			if step.Type == "" {
				return fmt.Errorf("step type is required in workflow %s", wf.Name)
			}
		}
	}

	return nil
}

// List returns enabled skills currently registered by the loader.
func (l *Loader) List() []*SkillMetadata {
	if l == nil || l.registry == nil {
		return nil
	}
	return l.registry.List()
}

// Get returns a single skill by name.
func (l *Loader) Get(name string) *SkillMetadata {
	if l == nil || l.registry == nil || name == "" {
		return nil
	}
	return l.registry.Get(name)
}

// FormatList renders a concise Markdown list for /skills and API surfaces.
func (l *Loader) FormatList() string {
	skills := l.List()
	if len(skills) == 0 {
		return "# Skills\n\nNo skills loaded."
	}

	var sb strings.Builder
	sb.WriteString("# Skills\n\n")
	for _, sm := range skills {
		sb.WriteString(fmt.Sprintf("- **%s** (%s): %s\n",
			sm.Manifest.Name,
			sm.Manifest.Version,
			sm.Manifest.Description,
		))
	}
	return sb.String()
}

// FormatIndexForPrompt renders a compact, model-friendly index used in system prompt injection.
func (l *Loader) FormatIndexForPrompt() string {
	skills := l.List()
	if len(skills) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n<available_skills>\n")
	for _, sm := range skills {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", sm.Manifest.Name, sm.Manifest.Description))
	}
	sb.WriteString("</available_skills>\n\n")
	return sb.String()
}

// RenderInvocation returns a best-effort prompt expansion for slash-command skill invocation.
func (l *Loader) RenderInvocation(name, args string) (string, bool) {
	skill := l.Get(name)
	if skill == nil || skill.Manifest == nil || !skill.Manifest.Enabled {
		return "", false
	}

	manifest := skill.Manifest
	if len(manifest.Prompts) > 0 {
		tpl := manifest.Prompts[0].Template
		if args != "" {
			tpl = strings.ReplaceAll(tpl, "{{args}}", args)
		}
		return tpl, true
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Use skill '%s' (%s).\n", manifest.Name, manifest.Description))
	if args != "" {
		sb.WriteString(fmt.Sprintf("User input: %s\n", args))
	}
	if len(manifest.Workflows) > 0 {
		sb.WriteString("Follow workflow steps:\n")
		for _, wf := range manifest.Workflows {
			sb.WriteString(fmt.Sprintf("- %s\n", wf.Name))
			for _, step := range wf.Steps {
				sb.WriteString(fmt.Sprintf("  - [%s] %s\n", step.Type, step.ID))
			}
			break
		}
	}
	return strings.TrimSpace(sb.String()), true
}

// Format returns a human-readable list of installed skills.
func (l *Loader) Format() string {
	return l.FormatList()
}

// Create writes a new skill manifest and registers it.
func (l *Loader) Create(name, description, content string, tags, platforms []string) error {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" || description == "" {
		return fmt.Errorf("name and description are required")
	}
	if l.Get(name) != nil {
		return fmt.Errorf("skill already exists: %s", name)
	}

	skillDir := filepath.Join(l.skillRootDir(), name)
	manifestPath := filepath.Join(skillDir, ".gorkskill.yaml")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return fmt.Errorf("failed to create skill directory: %w", err)
	}

	var out []byte
	if strings.TrimSpace(content) != "" {
		out = []byte(content)
	} else {
		now := time.Now()
		manifest := SkillManifest{
			Name:          name,
			SchemaVersion: "1.0",
			Version:       "1.0.0",
			Description:   description,
			Category:      "custom",
			Tags:          tags,
			Keywords:      append([]string{}, tags...),
			Created:       now,
			Modified:      now,
			Enabled:       true,
		}
		for _, p := range platforms {
			p = strings.TrimSpace(p)
			if p != "" {
				manifest.Keywords = append(manifest.Keywords, "platform:"+p)
			}
		}
		data, err := yaml.Marshal(&manifest)
		if err != nil {
			return fmt.Errorf("failed to marshal manifest: %w", err)
		}
		out = data
	}

	if err := os.WriteFile(manifestPath, out, 0o644); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	manifest, err := l.LoadManifest(manifestPath)
	if err != nil {
		return fmt.Errorf("created file is invalid manifest: %w", err)
	}
	if err := l.registry.Register(manifest, manifestPath); err != nil {
		return err
	}
	return nil
}

// Patch replaces the first occurrence of oldText with newText in the skill file.
func (l *Loader) Patch(name, oldText, newText string) error {
	content, err := l.View(name)
	if err != nil {
		return err
	}
	if !strings.Contains(content, oldText) {
		return fmt.Errorf("old_text not found in skill %s", name)
	}
	updated := strings.Replace(content, oldText, newText, 1)
	md := l.Get(name)
	if md == nil {
		return fmt.Errorf("skill not found: %s", name)
	}
	if err := os.WriteFile(md.FilePath, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("failed to write patched skill: %w", err)
	}
	manifest, err := l.LoadManifest(md.FilePath)
	if err != nil {
		return fmt.Errorf("patched file is invalid manifest: %w", err)
	}
	_ = l.registry.Unregister(name)
	return l.registry.Register(manifest, md.FilePath)
}

// Delete removes a user-defined skill file and unregisters it.
func (l *Loader) Delete(name string) error {
	md := l.Get(name)
	if md == nil {
		return fmt.Errorf("skill not found: %s", name)
	}
	if strings.Contains(md.FilePath, string(filepath.Separator)+".system"+string(filepath.Separator)) {
		return fmt.Errorf("refusing to delete built-in system skill: %s", name)
	}
	if err := os.Remove(md.FilePath); err != nil {
		return fmt.Errorf("failed to remove skill file: %w", err)
	}
	_ = l.registry.Unregister(name)
	return nil
}

// View returns the raw manifest content for a skill.
func (l *Loader) View(name string) (string, error) {
	md := l.Get(name)
	if md == nil {
		return "", fmt.Errorf("skill not found: %s", name)
	}
	data, err := os.ReadFile(md.FilePath)
	if err != nil {
		return "", fmt.Errorf("failed to read skill file: %w", err)
	}
	return string(data), nil
}

func (l *Loader) skillRootDir() string {
	if reg, ok := l.registry.(*InMemoryRegistry); ok && reg.dir != "" {
		return reg.dir
	}
	if v := strings.TrimSpace(os.Getenv("GORKBOT_SKILLS_DIR")); v != "" {
		return v
	}
	return "skills"
}

// LintIssue represents a skill validation issue.
type LintIssue struct {
	File    string `json:"file"`
	Skill   string `json:"skill,omitempty"`
	Message string `json:"message"`
}

// LintFile validates a manifest file and returns any issues.
func (l *Loader) LintFile(filePath string) []LintIssue {
	manifest, err := l.LoadManifest(filePath)
	if err != nil {
		return []LintIssue{{File: filePath, Message: err.Error()}}
	}
	return l.lintManifest(manifest, filePath)
}

// LintDirectory validates all .gorkskill manifests in a directory tree.
func (l *Loader) LintDirectory(dirPath string) []LintIssue {
	var issues []LintIssue
	_ = filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			issues = append(issues, LintIssue{File: path, Message: err.Error()})
			return nil
		}
		if info == nil || info.IsDir() {
			return nil
		}
		if info.Name() != ".gorkskill.yaml" {
			return nil
		}
		issues = append(issues, l.LintFile(path)...)
		return nil
	})
	return issues
}

func (l *Loader) lintManifest(manifest *SkillManifest, filePath string) []LintIssue {
	var issues []LintIssue
	if manifest == nil {
		return []LintIssue{{File: filePath, Message: "manifest is nil"}}
	}
	if len(manifest.Tools) == 0 && len(manifest.Workflows) == 0 && len(manifest.Prompts) == 0 {
		issues = append(issues, LintIssue{
			File:    filePath,
			Skill:   manifest.Name,
			Message: "skill has no tools, prompts, or workflows",
		})
	}
	for _, perm := range manifest.Permissions {
		level := strings.ToLower(strings.TrimSpace(perm.Level))
		switch level {
		case "", "always", "session", "once", "never":
		default:
			issues = append(issues, LintIssue{
				File:    filePath,
				Skill:   manifest.Name,
				Message: fmt.Sprintf("invalid permission level: %s", perm.Level),
			})
		}
	}
	return issues
}
