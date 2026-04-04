package skills

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// RuleRegistrar is the interface for registering permission rules from skills (Task 5.4).
type RuleRegistrar interface {
	// AddRule adds a permission rule (decision, pattern, comment).
	AddRule(decision string, pattern, comment string) error
}

// InMemoryRegistry implements the Registry interface with in-memory storage.
type InMemoryRegistry struct {
	skills        map[string]*SkillMetadata
	dir           string           // Persistence directory (Task 5.4)
	ruleRegistrar RuleRegistrar    // For registering skill permission rules (Task 5.4)
	mu            sync.RWMutex
	logger        *slog.Logger
}

// NewInMemoryRegistry creates a new in-memory skill registry.
func NewInMemoryRegistry(logger *slog.Logger) *InMemoryRegistry {
	if logger == nil {
		logger = slog.Default()
	}

	return &InMemoryRegistry{
		skills: make(map[string]*SkillMetadata),
		logger: logger,
	}
}

// NewPersistentRegistry creates a registry with persistence (Task 5.4).
// Loads index.json if it exists to restore enabled/disabled state.
func NewPersistentRegistry(dir string, logger *slog.Logger) *InMemoryRegistry {
	if logger == nil {
		logger = slog.Default()
	}

	reg := &InMemoryRegistry{
		skills: make(map[string]*SkillMetadata),
		dir:    dir,
		logger: logger,
	}

	// Attempt to load persisted index (enabled/disabled state)
	_ = reg.loadIndex()

	return reg
}

// SetRuleRegistrar injects a RuleRegistrar for skill permission registration (Task 5.4).
func (r *InMemoryRegistry) SetRuleRegistrar(rr RuleRegistrar) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ruleRegistrar = rr
}

// Register adds a new skill to the registry.
func (r *InMemoryRegistry) Register(manifest *SkillManifest, filePath string) error {
	if manifest == nil {
		return fmt.Errorf("manifest is required")
	}
	if manifest.Name == "" {
		return fmt.Errorf("skill name is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.skills[manifest.Name]; exists {
		return fmt.Errorf("skill already registered: %s", manifest.Name)
	}

	metadata := &SkillMetadata{
		Manifest:    manifest,
		FilePath:    filePath,
		SearchScore: 0,
		UsageCount:  0,
		LastUsed:    time.Time{},
	}

	r.skills[manifest.Name] = metadata
	r.logger.Info("skill registered", "name", manifest.Name, "version", manifest.Version)

	return nil
}

// Unregister removes a skill from the registry.
func (r *InMemoryRegistry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.skills[name]; !exists {
		return fmt.Errorf("skill not found: %s", name)
	}

	delete(r.skills, name)
	r.logger.Info("skill unregistered", "name", name)

	return nil
}

// Get retrieves a skill by name.
func (r *InMemoryRegistry) Get(name string) *SkillMetadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.skills[name]
}

// List returns all registered skills.
func (r *InMemoryRegistry) List() []*SkillMetadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	skills := make([]*SkillMetadata, 0, len(r.skills))
	for _, skill := range r.skills {
		if skill.Manifest.Enabled {
			skills = append(skills, skill)
		}
	}

	return skills
}

// Search finds skills using keyword and semantic matching.
func (r *InMemoryRegistry) Search(query string, limit int) []*SearchResult {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*SearchResult
	queryLower := strings.ToLower(query)

	for _, skill := range r.skills {
		if !skill.Manifest.Enabled {
			continue
		}

		// Keyword matching in name and description
		nameLower := strings.ToLower(skill.Manifest.Name)
		descLower := strings.ToLower(skill.Manifest.Description)

		matchScore := 0.0
		matchField := ""

		if strings.Contains(nameLower, queryLower) {
			matchScore = 1.0
			matchField = "name"
		} else if strings.Contains(descLower, queryLower) {
			matchScore = 0.8
			matchField = "description"
		}

		// Keyword matching in tags and keywords
		for _, kw := range skill.Manifest.Keywords {
			if strings.Contains(strings.ToLower(kw), queryLower) {
				matchScore = 0.9
				matchField = "keyword"
				break
			}
		}

		// Category matching
		if strings.ToLower(skill.Manifest.Category) == queryLower {
			matchScore = 0.95
			matchField = "category"
		}

		if matchScore > 0 {
			results = append(results, &SearchResult{
				Skill:     skill,
				Score:     matchScore,
				MatchType: "keyword",
				MatchField: matchField,
			})
		}
	}

	// Sort by score (highest first)
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	// Limit results
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results
}

// Enable enables a skill and registers its permission rules (Task 5.4).
func (r *InMemoryRegistry) Enable(name string) error {
	r.mu.Lock()
	skill, exists := r.skills[name]
	if !exists {
		r.mu.Unlock()
		return fmt.Errorf("skill not found: %s", name)
	}

	skill.Manifest.Enabled = true
	r.logger.Info("skill enabled", "name", name)

	// Copy permission list before releasing lock
	permissions := make([]SkillPermission, len(skill.Manifest.Permissions))
	copy(permissions, skill.Manifest.Permissions)
	r.mu.Unlock()

	// Register permission rules from the manifest (Task 5.4)
	// Do this outside the lock to avoid deadlock
	if r.ruleRegistrar != nil && len(permissions) > 0 {
		for _, perm := range permissions {
			// Build pattern: "tool" or "tool(pattern)" if pattern is specified
			pattern := perm.Tool
			if perm.Pattern != "" {
				pattern = fmt.Sprintf("%s(%s)", perm.Tool, perm.Pattern)
			}

			// Map level to decision string
			decision := perm.Level
			if decision == "" {
				decision = "session" // Default
			}

			// Register the rule
			comment := fmt.Sprintf("declared by skill %s", name)
			if err := r.ruleRegistrar.AddRule(decision, pattern, comment); err != nil {
				r.logger.Error("failed to register permission rule",
					"skill", name,
					"tool", perm.Tool,
					"error", err.Error(),
				)
			}
		}
	}

	// Persist the new state (also does its own locking)
	if r.dir != "" {
		_ = r.persistIndex()
	}

	return nil
}

// Disable disables a skill.
func (r *InMemoryRegistry) Disable(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	skill, exists := r.skills[name]
	if !exists {
		return fmt.Errorf("skill not found: %s", name)
	}

	skill.Manifest.Enabled = false
	r.logger.Info("skill disabled", "name", name)

	return nil
}

// GetWorkflow retrieves a workflow from a skill.
func (r *InMemoryRegistry) GetWorkflow(skillName, workflowName string) *Workflow {
	r.mu.RLock()
	defer r.mu.RUnlock()

	skill, exists := r.skills[skillName]
	if !exists {
		return nil
	}

	for _, wf := range skill.Manifest.Workflows {
		if wf.Name == workflowName {
			return &wf
		}
	}

	return nil
}

// ResolveTools returns all tools needed for a skill and its dependencies.
func (r *InMemoryRegistry) ResolveTools(skillName string) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.skills[skillName]
	if !exists {
		return nil, fmt.Errorf("skill not found: %s", skillName)
	}

	toolSet := make(map[string]bool)
	var queue []string = []string{skillName}
	visited := make(map[string]bool)

	// BFS to resolve all dependencies
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if visited[current] {
			continue
		}
		visited[current] = true

		currentSkill, ok := r.skills[current]
		if !ok {
			continue
		}

		// Add tools from current skill
		for _, tool := range currentSkill.Manifest.Tools {
			toolSet[tool.Name] = true
		}

		// Add dependencies to queue
		for _, dep := range currentSkill.Manifest.Dependencies {
			if !visited[dep] {
				queue = append(queue, dep)
			}
		}
	}

	// Convert set to slice
	tools := make([]string, 0, len(toolSet))
	for tool := range toolSet {
		tools = append(tools, tool)
	}

	return tools, nil
}

// RecordUsage records a skill usage.
func (r *InMemoryRegistry) RecordUsage(skillName string, executionTimeMs int64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	skill, exists := r.skills[skillName]
	if !exists {
		return
	}

	skill.UsageCount++
	skill.LastUsed = time.Now()
	if skill.AvgExecutionTime == 0 {
		skill.AvgExecutionTime = executionTimeMs
	} else {
		// Update moving average
		skill.AvgExecutionTime = (skill.AvgExecutionTime + executionTimeMs) / 2
	}
}

// persistIndex writes enabled/disabled state to index.json (Task 5.4).
func (r *InMemoryRegistry) persistIndex() error {
	if r.dir == "" {
		return nil // Persistence disabled
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	// Build index of enabled/disabled states
	index := make(map[string]bool)
	for name, skill := range r.skills {
		index[name] = skill.Manifest.Enabled
	}

	// Ensure directory exists
	if err := os.MkdirAll(r.dir, 0755); err != nil {
		return fmt.Errorf("failed to create skill directory: %w", err)
	}

	// Write to index.json
	indexPath := filepath.Join(r.dir, "index.json")
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal index: %w", err)
	}

	return os.WriteFile(indexPath, data, 0600)
}

// loadIndex restores enabled/disabled state from index.json (Task 5.4).
func (r *InMemoryRegistry) loadIndex() error {
	if r.dir == "" {
		return nil // Persistence disabled
	}

	indexPath := filepath.Join(r.dir, "index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet, that's okay
		}
		return fmt.Errorf("failed to read index: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Parse index
	var index map[string]bool
	if err := json.Unmarshal(data, &index); err != nil {
		return fmt.Errorf("failed to unmarshal index: %w", err)
	}

	// Apply enabled/disabled state to loaded skills
	for name, enabled := range index {
		if skill, exists := r.skills[name]; exists {
			skill.Manifest.Enabled = enabled
		}
	}

	return nil
}
