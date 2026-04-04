package skills

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegistry_Register tests skill registration.
func TestRegistry_Register(t *testing.T) {
	registry := NewInMemoryRegistry(slog.Default())
	manifest := &SkillManifest{
		Name:        "test_skill",
		Version:     "1.0.0",
		Description: "Test skill",
		Enabled:     true,
	}

	err := registry.Register(manifest, "/path/to/skill.yaml")
	assert.NoError(t, err)

	retrieved := registry.Get("test_skill")
	assert.NotNil(t, retrieved)
	assert.Equal(t, "test_skill", retrieved.Manifest.Name)
}

// TestRegistry_RegisterDuplicate tests duplicate registration.
func TestRegistry_RegisterDuplicate(t *testing.T) {
	registry := NewInMemoryRegistry(slog.Default())
	manifest := &SkillManifest{
		Name:        "test_skill",
		Version:     "1.0.0",
		Description: "Test skill",
	}

	err := registry.Register(manifest, "/path/to/skill.yaml")
	assert.NoError(t, err)

	err = registry.Register(manifest, "/path/to/skill2.yaml")
	assert.Error(t, err)
}

// TestRegistry_Unregister tests skill removal.
func TestRegistry_Unregister(t *testing.T) {
	registry := NewInMemoryRegistry(slog.Default())
	manifest := &SkillManifest{
		Name:        "test_skill",
		Version:     "1.0.0",
		Description: "Test skill",
	}

	registry.Register(manifest, "/path/to/skill.yaml")
	err := registry.Unregister("test_skill")
	assert.NoError(t, err)

	retrieved := registry.Get("test_skill")
	assert.Nil(t, retrieved)
}

// TestRegistry_List tests listing skills.
func TestRegistry_List(t *testing.T) {
	registry := NewInMemoryRegistry(slog.Default())

	manifest1 := &SkillManifest{
		Name:        "skill1",
		Version:     "1.0.0",
		Description: "Skill 1",
		Enabled:     true,
	}
	manifest2 := &SkillManifest{
		Name:        "skill2",
		Version:     "1.0.0",
		Description: "Skill 2",
		Enabled:     false,
	}

	registry.Register(manifest1, "/path/to/skill1.yaml")
	registry.Register(manifest2, "/path/to/skill2.yaml")

	skills := registry.List()
	assert.Equal(t, 1, len(skills)) // Only enabled skills
	assert.Equal(t, "skill1", skills[0].Manifest.Name)
}

// TestRegistry_Search tests skill search.
func TestRegistry_Search(t *testing.T) {
	registry := NewInMemoryRegistry(slog.Default())

	manifest := &SkillManifest{
		Name:        "data_processor",
		Version:     "1.0.0",
		Description: "Process data files",
		Keywords:    []string{"data", "processing", "files"},
		Category:    "data",
		Enabled:     true,
	}

	registry.Register(manifest, "/path/to/skill.yaml")

	// Search by name
	results := registry.Search("data_processor", 10)
	assert.Greater(t, len(results), 0)

	// Search by keyword
	results = registry.Search("processing", 10)
	assert.Greater(t, len(results), 0)

	// Search by category
	results = registry.Search("data", 10)
	assert.Greater(t, len(results), 0)
}

// TestRegistry_EnableDisable tests enable/disable functionality.
func TestRegistry_EnableDisable(t *testing.T) {
	registry := NewInMemoryRegistry(slog.Default())
	manifest := &SkillManifest{
		Name:        "test_skill",
		Version:     "1.0.0",
		Description: "Test skill",
		Enabled:     true,
	}

	registry.Register(manifest, "/path/to/skill.yaml")

	err := registry.Disable("test_skill")
	assert.NoError(t, err)

	retrieved := registry.Get("test_skill")
	assert.False(t, retrieved.Manifest.Enabled)

	err = registry.Enable("test_skill")
	assert.NoError(t, err)

	retrieved = registry.Get("test_skill")
	assert.True(t, retrieved.Manifest.Enabled)
}

// TestRegistry_GetWorkflow tests workflow retrieval.
func TestRegistry_GetWorkflow(t *testing.T) {
	registry := NewInMemoryRegistry(slog.Default())

	manifest := &SkillManifest{
		Name:        "test_skill",
		Version:     "1.0.0",
		Description: "Test skill",
		Workflows: []Workflow{
			{
				Name: "process_data",
				Steps: []WorkflowStep{
					{ID: "step1", Type: "tool", Tool: "read_file"},
				},
			},
		},
		Enabled: true,
	}

	registry.Register(manifest, "/path/to/skill.yaml")

	workflow := registry.GetWorkflow("test_skill", "process_data")
	require.NotNil(t, workflow)
	assert.Equal(t, "process_data", workflow.Name)
	assert.Equal(t, 1, len(workflow.Steps))
}

// TestRegistry_ResolveTools tests tool resolution with dependencies.
func TestRegistry_ResolveTools(t *testing.T) {
	registry := NewInMemoryRegistry(slog.Default())

	// Base skill
	skillA := &SkillManifest{
		Name:        "skill_a",
		Version:     "1.0.0",
		Description: "Skill A",
		Tools: []SkillTool{
			{Name: "tool1", Required: true},
		},
		Enabled: true,
	}

	// Dependent skill
	skillB := &SkillManifest{
		Name:        "skill_b",
		Version:     "1.0.0",
		Description: "Skill B",
		Tools: []SkillTool{
			{Name: "tool2", Required: true},
		},
		Dependencies: []string{"skill_a"},
		Enabled:      true,
	}

	registry.Register(skillA, "/path/to/skill_a.yaml")
	registry.Register(skillB, "/path/to/skill_b.yaml")

	tools, err := registry.ResolveTools("skill_b")
	assert.NoError(t, err)
	assert.Equal(t, 2, len(tools)) // tool1 from A + tool2 from B
}

// TestRegistry_RecordUsage tests usage tracking.
func TestRegistry_RecordUsage(t *testing.T) {
	registry := NewInMemoryRegistry(slog.Default())

	manifest := &SkillManifest{
		Name:        "test_skill",
		Version:     "1.0.0",
		Description: "Test skill",
		Enabled:     true,
	}

	registry.Register(manifest, "/path/to/skill.yaml")

	registry.RecordUsage("test_skill", 1000)

	retrieved := registry.Get("test_skill")
	assert.Equal(t, 1, retrieved.UsageCount)
	assert.Equal(t, int64(1000), retrieved.AvgExecutionTime)

	// Record another usage
	registry.RecordUsage("test_skill", 1200)
	retrieved = registry.Get("test_skill")
	assert.Equal(t, 2, retrieved.UsageCount)
	assert.Equal(t, int64(1100), retrieved.AvgExecutionTime) // Moving average
}
