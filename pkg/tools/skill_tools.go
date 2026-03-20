package tools

// skill_tools.go — AI-callable Skill Management Tools
//
// Exposes skill CRUD operations as Gorkbot tools so the AI agent can
// create, modify, delete, and view skill definitions without requiring
// a human to use the filesystem directly.
//
// All tools require a reference to the skills.Loader, injected via
// NewSkillTools(loader).  Register them with:
//
//	st := tools.NewSkillTools(loader)
//	st.Register(toolRegistry)
//
// Permission model:
//   - skill_view   → always (read-only)
//   - skill_create → always (additive, non-destructive)
//   - skill_patch  → always (modifies a skill file — reversible)
//   - skill_delete → always (destructive — only allowed for user-defined skills)

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/velariumai/gorkbot/pkg/skills"
)

// SkillTools groups the four skill management tools around a shared Loader.
type SkillTools struct {
	loader *skills.Loader
}

// NewSkillTools creates a SkillTools backed by the given skills.Loader.
func NewSkillTools(loader *skills.Loader) *SkillTools {
	return &SkillTools{loader: loader}
}

// Register registers all five skill tools into reg.
func (st *SkillTools) Register(reg *Registry) {
	_ = reg.Register(st.newSkillListTool())
	_ = reg.Register(st.newSkillCreateTool())
	_ = reg.Register(st.newSkillPatchTool())
	_ = reg.Register(st.newSkillDeleteTool())
	_ = reg.Register(st.newSkillViewTool())
}

// ── skills_list ───────────────────────────────────────────────────────────────

type skillListTool struct {
	BaseTool
	loader *skills.Loader
}

func (st *SkillTools) newSkillListTool() *skillListTool {
	return &skillListTool{
		BaseTool: NewBaseTool(
			"skills_list",
			"List all installed skill definitions (built-in and user-defined). "+
				"Shows name, aliases, description, model override, and source file. "+
				"Call this before starting any complex task to find applicable skills.",
			CategoryMeta,
			false,
			PermissionAlways,
		),
		loader: st.loader,
	}
}

func (t *skillListTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}

func (t *skillListTool) Execute(_ context.Context, _ map[string]interface{}) (*ToolResult, error) {
	return &ToolResult{
		Success:      true,
		Output:       t.loader.Format(),
		OutputFormat: FormatText,
	}, nil
}

func (t *skillListTool) OutputFormat() OutputFormat { return FormatText }

// ── skill_create ──────────────────────────────────────────────────────────────

type skillCreateTool struct {
	BaseTool
	loader *skills.Loader
}

func (st *SkillTools) newSkillCreateTool() *skillCreateTool {
	return &skillCreateTool{
		BaseTool: NewBaseTool(
			"skill_create",
			"Create a new skill definition file. Provide name (slug) and description; "+
				"optionally supply full markdown content with frontmatter. "+
				"If content is omitted a minimal template is generated automatically.",
			CategoryMeta,
			false,
			PermissionAlways,
		),
		loader: st.loader,
	}
}

func (t *skillCreateTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {
				"type": "string",
				"description": "Skill name slug (e.g. code-review). Must match ^[a-z0-9][a-z0-9._-]{0,63}$"
			},
			"description": {
				"type": "string",
				"description": "Short one-line description of what the skill does"
			},
			"content": {
				"type": "string",
				"description": "Full skill markdown content including frontmatter (optional — generated if omitted)"
			},
			"tags": {
				"type": "string",
				"description": "Comma-separated tags for the skill (optional)"
			},
			"platforms": {
				"type": "string",
				"description": "Comma-separated platform names this skill targets (optional)"
			}
		},
		"required": ["name", "description"]
	}`)
}

func (t *skillCreateTool) Execute(_ context.Context, params map[string]interface{}) (*ToolResult, error) {
	name, ok := params["name"].(string)
	if !ok || strings.TrimSpace(name) == "" {
		return &ToolResult{Success: false, Error: "name parameter is required", OutputFormat: FormatError}, nil
	}
	description, _ := params["description"].(string)
	content, _ := params["content"].(string)

	var tags []string
	if tagsStr, ok := params["tags"].(string); ok && tagsStr != "" {
		for _, t := range strings.Split(tagsStr, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
	}

	var platforms []string
	if platStr, ok := params["platforms"].(string); ok && platStr != "" {
		for _, p := range strings.Split(platStr, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				platforms = append(platforms, p)
			}
		}
	}

	if err := t.loader.Create(name, description, content, tags, platforms); err != nil {
		return &ToolResult{
			Success:      false,
			Error:        fmt.Sprintf("skill_create failed: %v", err),
			OutputFormat: FormatError,
		}, nil
	}

	return &ToolResult{
		Success:      true,
		Output:       fmt.Sprintf("Skill %q created successfully.", name),
		OutputFormat: FormatText,
	}, nil
}

func (t *skillCreateTool) OutputFormat() OutputFormat { return FormatText }

// ── skill_patch ───────────────────────────────────────────────────────────────

type skillPatchTool struct {
	BaseTool
	loader *skills.Loader
}

func (st *SkillTools) newSkillPatchTool() *skillPatchTool {
	return &skillPatchTool{
		BaseTool: NewBaseTool(
			"skill_patch",
			"Find-and-replace text within an existing skill file. "+
				"Replaces the first occurrence of old_text with new_text in the skill source file.",
			CategoryMeta,
			false,
			PermissionAlways,
		),
		loader: st.loader,
	}
}

func (t *skillPatchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {
				"type": "string",
				"description": "Canonical skill name to patch"
			},
			"old_text": {
				"type": "string",
				"description": "Text to find (first occurrence will be replaced)"
			},
			"new_text": {
				"type": "string",
				"description": "Replacement text"
			}
		},
		"required": ["name", "old_text", "new_text"]
	}`)
}

func (t *skillPatchTool) Execute(_ context.Context, params map[string]interface{}) (*ToolResult, error) {
	name, ok := params["name"].(string)
	if !ok || strings.TrimSpace(name) == "" {
		return &ToolResult{Success: false, Error: "name parameter is required", OutputFormat: FormatError}, nil
	}
	oldText, ok := params["old_text"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "old_text parameter is required", OutputFormat: FormatError}, nil
	}
	newText, _ := params["new_text"].(string)

	if err := t.loader.Patch(name, oldText, newText); err != nil {
		return &ToolResult{
			Success:      false,
			Error:        fmt.Sprintf("skill_patch failed: %v", err),
			OutputFormat: FormatError,
		}, nil
	}

	return &ToolResult{
		Success:      true,
		Output:       fmt.Sprintf("Skill %q patched successfully.", name),
		OutputFormat: FormatText,
	}, nil
}

func (t *skillPatchTool) OutputFormat() OutputFormat { return FormatText }

// ── skill_delete ──────────────────────────────────────────────────────────────

type skillDeleteTool struct {
	BaseTool
	loader *skills.Loader
}

func (st *SkillTools) newSkillDeleteTool() *skillDeleteTool {
	return &skillDeleteTool{
		BaseTool: NewBaseTool(
			"skill_delete",
			"Delete a user-defined skill file by name. "+
				"Built-in skills cannot be deleted. This action is irreversible.",
			CategoryMeta,
			false,
			PermissionAlways,
		),
		loader: st.loader,
	}
}

func (t *skillDeleteTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {
				"type": "string",
				"description": "Canonical skill name to delete"
			}
		},
		"required": ["name"]
	}`)
}

func (t *skillDeleteTool) Execute(_ context.Context, params map[string]interface{}) (*ToolResult, error) {
	name, ok := params["name"].(string)
	if !ok || strings.TrimSpace(name) == "" {
		return &ToolResult{Success: false, Error: "name parameter is required", OutputFormat: FormatError}, nil
	}

	if err := t.loader.Delete(name); err != nil {
		return &ToolResult{
			Success:      false,
			Error:        fmt.Sprintf("skill_delete failed: %v", err),
			OutputFormat: FormatError,
		}, nil
	}

	return &ToolResult{
		Success:      true,
		Output:       fmt.Sprintf("Skill %q deleted successfully.", name),
		OutputFormat: FormatText,
	}, nil
}

func (t *skillDeleteTool) OutputFormat() OutputFormat { return FormatText }

// ── skill_view ────────────────────────────────────────────────────────────────

type skillViewTool struct {
	BaseTool
	loader *skills.Loader
}

func (st *SkillTools) newSkillViewTool() *skillViewTool {
	return &skillViewTool{
		BaseTool: NewBaseTool(
			"skill_view",
			"Read the full raw content of a skill definition file, including its frontmatter. "+
				"Works for both user-defined and built-in skills.",
			CategoryMeta,
			false,
			PermissionAlways,
		),
		loader: st.loader,
	}
}

func (t *skillViewTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {
				"type": "string",
				"description": "Canonical skill name to view"
			}
		},
		"required": ["name"]
	}`)
}

func (t *skillViewTool) Execute(_ context.Context, params map[string]interface{}) (*ToolResult, error) {
	name, ok := params["name"].(string)
	if !ok || strings.TrimSpace(name) == "" {
		return &ToolResult{Success: false, Error: "name parameter is required", OutputFormat: FormatError}, nil
	}

	content, err := t.loader.View(name)
	if err != nil {
		return &ToolResult{
			Success:      false,
			Error:        fmt.Sprintf("skill_view failed: %v", err),
			OutputFormat: FormatError,
		}, nil
	}

	return &ToolResult{
		Success:      true,
		Output:       content,
		OutputFormat: FormatText,
	}, nil
}

func (t *skillViewTool) OutputFormat() OutputFormat { return FormatText }
