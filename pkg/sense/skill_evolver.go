package sense

// skill_evolver.go — SENSE Evolutionary Pipeline
//
// Implements /self evolve: converts trace analysis failure reports into
// SKILL.md files containing explicit invariants for the LLM.
//
// A SKILL.md file encodes a learned invariant as a structured Markdown
// document.  The LLM's system prompt is enriched with active SKILL files
// via the skills loader, teaching it to avoid previously observed failure
// patterns.
//
// Safety constraints (Poehnelt 2026, §4.3):
//
//   - Every write action MUST be preceded by a dry-run validation pass.
//   - The --dry-run flag causes the evolver to return what it WOULD write
//     without touching the filesystem.
//   - All paths are constructed with path/filepath (platform-agnostic).
//   - The skills directory is created with mode 0700.
//
// Minimum evidence threshold: a pattern must have at least MinEvidence
// occurrences before a SKILL file is generated (default 2).  This prevents
// noise from one-off transient errors inflating the skill library.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ── Success-candidate detection ────────────────────────────────────────────

// SkillCandidate represents a potential skill extracted from a successful
// multi-tool session that might benefit from being saved as a reusable skill.
type SkillCandidate struct {
	// SuggestedName is the kebab-case slug derived from the task signature.
	SuggestedName string
	// Description is a short auto-generated description of the candidate skill.
	Description string
	// DraftContent is a full markdown skill file (with frontmatter) generated
	// from the tool sequence observed in the session.
	DraftContent string
	// Confidence is a fixed 0.75 value (no embedding comparison available).
	Confidence float64
	// SessionID is the session that produced this candidate.
	SessionID string
}

// EvolverConfig controls the behaviour of the SkillEvolver.
type EvolverConfig struct {
	// SkillsDir is the directory where SKILL.md files are written.
	// Created (mode 0700) if it does not exist.
	SkillsDir string

	// DryRun when true causes the evolver to return a plan without writing
	// any files.  MANDATORY for /self fix and /self evolve by default.
	DryRun bool

	// MinEvidence is the minimum number of failure occurrences required
	// before a SKILL file is generated.  Default: 2.
	MinEvidence int

	// MaxSkillsPerRun caps the number of SKILL files written in one run.
	// Default: 20.
	MaxSkillsPerRun int

	// Author is embedded in generated SKILL files (e.g. "SENSE v1.0").
	Author string
}

// DefaultEvolverConfig returns an EvolverConfig with safe defaults.
// DryRun is true by default to prevent accidental writes.
func DefaultEvolverConfig(skillsDir string) EvolverConfig {
	return EvolverConfig{
		SkillsDir:       skillsDir,
		DryRun:          true, // MANDATORY default — must be explicitly disabled
		MinEvidence:     2,
		MaxSkillsPerRun: 20,
		Author:          "SENSE Autonomous Evolver v1.0",
	}
}

// SkillFile represents a SKILL.md file to be created.
type SkillFile struct {
	// Filename is the base name (e.g. "SKILL-bash-path-escape.md").
	Filename string
	// FullPath is the absolute path (only set after Write succeeds).
	FullPath string
	// Content is the full Markdown text.
	Content string
	// Pattern is the ToolFailurePattern this file addresses.
	Pattern ToolFailurePattern
	// DryRun indicates this file was not actually written.
	DryRun bool
}

// EvolveResult is the output of a single SkillEvolver.Evolve() run.
type EvolveResult struct {
	// DryRun is true when no files were written.
	DryRun bool
	// Skills is the list of SKILL files generated (or planned, if dry-run).
	Skills []SkillFile
	// Skipped is the count of patterns that did not meet MinEvidence.
	Skipped int
	// Summary is a human-readable Markdown summary of what was done.
	Summary string
}

// SkillEvolver converts AnalysisReport failure patterns into SKILL.md files.
type SkillEvolver struct {
	cfg EvolverConfig
}

// NewSkillEvolver creates an evolver with the given configuration.
func NewSkillEvolver(cfg EvolverConfig) *SkillEvolver {
	return &SkillEvolver{cfg: cfg}
}

// Evolve processes the AnalysisReport and generates (or plans) SKILL.md files.
// When cfg.DryRun is true, no files are written and SkillFile.DryRun is set.
func (e *SkillEvolver) Evolve(report *AnalysisReport) (*EvolveResult, error) {
	result := &EvolveResult{DryRun: e.cfg.DryRun}

	if len(report.Patterns) == 0 {
		result.Summary = "No failure patterns found. Nothing to evolve."
		return result, nil
	}

	// Ensure skills directory exists (unless dry-run).
	if !e.cfg.DryRun {
		if err := os.MkdirAll(e.cfg.SkillsDir, 0700); err != nil {
			return nil, fmt.Errorf("SENSE evolver: cannot create skills dir %q: %w",
				e.cfg.SkillsDir, err)
		}
	}

	// Sort patterns by count descending; cap at MaxSkillsPerRun.
	patterns := make([]ToolFailurePattern, len(report.Patterns))
	copy(patterns, report.Patterns)
	sort.Slice(patterns, func(i, j int) bool {
		return patterns[i].Count > patterns[j].Count
	})

	limit := e.cfg.MaxSkillsPerRun
	if len(patterns) < limit {
		limit = len(patterns)
	}
	patterns = patterns[:limit]

	for _, p := range patterns {
		if p.Count < e.cfg.MinEvidence {
			result.Skipped++
			continue
		}

		sf, err := e.generateSkill(p)
		if err != nil {
			// Non-fatal: continue with remaining patterns.
			continue
		}

		if !e.cfg.DryRun {
			// Validate + write.
			destPath := filepath.Join(e.cfg.SkillsDir, sf.Filename)
			// Use filepath.Clean to normalise the path (platform-agnostic).
			destPath = filepath.Clean(destPath)

			// Safety check: destination must stay inside the skills directory.
			skillsAnchor := e.cfg.SkillsDir
			if !strings.HasSuffix(skillsAnchor, string(filepath.Separator)) {
				skillsAnchor += string(filepath.Separator)
			}
			destCheck := destPath
			if !strings.HasSuffix(destCheck, string(filepath.Separator)) {
				destCheck += string(filepath.Separator)
			}
			if destPath != e.cfg.SkillsDir && !strings.HasPrefix(destCheck, skillsAnchor) {
				continue // path escaped sandbox — skip silently
			}

			if err := os.WriteFile(destPath, []byte(sf.Content), 0600); err != nil {
				continue // non-fatal write error
			}
			sf.FullPath = destPath
			sf.DryRun = false
		} else {
			sf.DryRun = true
			sf.FullPath = filepath.Join(e.cfg.SkillsDir, sf.Filename)
		}

		result.Skills = append(result.Skills, sf)
	}

	result.Summary = renderEvolveResult(result, e.cfg.SkillsDir)
	return result, nil
}

// generateSkill builds a SkillFile from a single ToolFailurePattern.
func (e *SkillEvolver) generateSkill(p ToolFailurePattern) (SkillFile, error) {
	slug := patternSlug(p)
	filename := "SKILL-" + slug + ".md"
	content := e.renderSkillContent(p, slug)
	return SkillFile{
		Filename: filename,
		Content:  content,
		Pattern:  p,
	}, nil
}

// renderSkillContent produces the full SKILL.md Markdown for a pattern.
func (e *SkillEvolver) renderSkillContent(p ToolFailurePattern, slug string) string {
	var sb strings.Builder
	now := time.Now().UTC().Format("2006-01-02")
	toolDisplay := p.Tool
	if toolDisplay == "__provider__" {
		toolDisplay = "(AI provider)"
	}

	// Header
	sb.WriteString(fmt.Sprintf("# SKILL: %s\n\n", titleFromSlug(slug)))
	sb.WriteString(fmt.Sprintf(
		"*Invariant generated by %s on %s*  \n",
		e.cfg.Author, now,
	))
	sb.WriteString(fmt.Sprintf(
		"*Pattern: %s (%s — %d occurrence(s))*\n\n",
		p.Category, toolDisplay, p.Count,
	))

	// Problem description
	sb.WriteString("## Problem Pattern\n\n")
	sb.WriteString(categoryDescription(p.Category, p.Tool))
	sb.WriteString("\n\n")

	// Evidence
	sb.WriteString("## Evidence\n\n")
	sb.WriteString(fmt.Sprintf(
		"- **%d** failure(s) observed between `%s` and `%s`\n",
		p.Count,
		p.FirstSeen.Format("2006-01-02"),
		p.LastSeen.Format("2006-01-02"),
	))
	if len(p.CommonErrors) > 0 {
		sb.WriteString("- Sample error messages:\n")
		for _, e := range p.CommonErrors {
			sb.WriteString(fmt.Sprintf("  - `%s`\n", e))
		}
	}
	sb.WriteString("\n")

	// Invariants (category-specific)
	sb.WriteString("## Invariants\n\n")
	sb.WriteString(categoryInvariants(p.Category, p.Tool))
	sb.WriteString("\n\n")

	// Correct usage examples
	sb.WriteString("## Correct Usage\n\n")
	sb.WriteString(categoryExample(p.Category, p.Tool))
	sb.WriteString("\n\n")

	// Checklist
	sb.WriteString("## Pre-call Checklist\n\n")
	sb.WriteString(categoryChecklist(p.Category, p.Tool))
	sb.WriteString("\n\n")

	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("*Auto-generated by SENSE — do not edit manually. Re-run `/self evolve` to refresh.*\n"))

	return sb.String()
}

// ── Category-specific text generators ────────────────────────────────────

func categoryDescription(cat FailureCategory, tool string) string {
	switch cat {
	case CatHallucination:
		return fmt.Sprintf(
			"The agent referenced tool `%s` or a resource that does not exist, "+
				"or produced output contradicting verifiable facts. "+
				"This is classified as a **Neural Hallucination** by SENSE §2.1.",
			ifEmpty(tool, "(unknown)"),
		)
	case CatToolFailure:
		return fmt.Sprintf(
			"Tool `%s` repeatedly returned errors that are not caused by context overflow. "+
				"The failures indicate either incorrect parameter construction, "+
				"missing prerequisites, or a systematic misuse of the tool.",
			ifEmpty(tool, "(unknown)"),
		)
	case CatContextOverflow:
		return "The AI provider rejected the request because the conversation context " +
			"exceeded the model's maximum token window. This typically occurs when " +
			"large file contents, long tool outputs, or many conversation turns are " +
			"accumulated without compression."
	case CatSanitizerReject:
		return fmt.Sprintf(
			"The SENSE stabilization middleware rejected an input to tool `%s`. "+
				"The parameter violated one of the three hardening invariants: "+
				"control-character rejection, path sandboxing, or resource-name validation.",
			ifEmpty(tool, "(unknown)"),
		)
	case CatProviderError:
		return "The AI provider returned a transient error (rate limit, server error, " +
			"or network timeout). This is not caused by agent behaviour but may indicate " +
			"excessive request frequency or quota exhaustion."
	default:
		return "Unknown failure category."
	}
}

func categoryInvariants(cat FailureCategory, tool string) string {
	bt := "`" // backtick helper — Go raw strings cannot contain backticks
	switch cat {
	case CatHallucination:
		return fmt.Sprintf(
			"**NEVER** call a tool whose name you cannot confirm via %slist_tools%s.\n"+
				"**NEVER** assert facts about filesystem paths without first calling %sfile_info%s or %slist_directory%s.\n"+
				"**ALWAYS** verify tool existence before constructing tool-call JSON for %s%s%s.\n"+
				"**ALWAYS** use %sgorkbot_status%s to confirm the current set of available tools before a complex task.",
			bt, bt, bt, bt, bt, bt, bt, ifEmpty(tool, "<tool_name>"), bt, bt, bt,
		)
	case CatToolFailure:
		tn := ifEmpty(tool, "<tool_name>")
		return fmt.Sprintf(
			"**ALWAYS** call %stool_info %s%s before using the tool if you are unsure of the parameter schema.\n"+
				"**NEVER** pass relative paths containing %s..%s components to %s%s%s.\n"+
				"**ALWAYS** check the tool's parameter schema and supply all required fields.\n"+
				"**NEVER** assume default values — explicitly set every required parameter.",
			bt, tn, bt, bt, bt, bt, tn, bt,
		)
	case CatContextOverflow:
		return fmt.Sprintf(
			"**ALWAYS** call %s/compress%s or the %scontext_stats%s tool when a session approaches 60,000 tokens.\n"+
				"**NEVER** paste raw file contents > 10,000 characters into the conversation; use %sread_file%s instead.\n"+
				"**ALWAYS** summarise tool outputs > 2,000 characters before continuing reasoning.\n"+
				"**NEVER** accumulate more than 30 uncompressed tool results in one session.",
			bt, bt, bt, bt, bt, bt,
		)
	case CatSanitizerReject:
		return "**NEVER** include control characters (ASCII < 0x20) in tool parameters.\n" +
			"**NEVER** use `..` or absolute paths outside the current working directory.\n" +
			"**NEVER** include `?`, `#`, or `%` in resource name parameters.\n" +
			"**ALWAYS** construct file paths relative to the current working directory."
	case CatProviderError:
		return "**ALWAYS** implement exponential back-off when a provider returns 429 or 503.\n" +
			"**NEVER** retry more than 3 times without notifying the user.\n" +
			"**ALWAYS** fall back to the secondary provider when the primary is unavailable for > 30 s."
	default:
		return "Apply general defensive coding practices."
	}
}

func categoryExample(cat FailureCategory, tool string) string {
	switch cat {
	case CatHallucination:
		return "```\n// Before calling any tool:\ntools := call list_tools()\nassert tool_name in tools  // GOOD\n\n// NEVER:\ncall non_existent_tool()   // BAD — hallucinated tool name\n```"
	case CatToolFailure:
		return fmt.Sprintf(
			"```\n// GOOD: verify schema first\ninfo := call tool_info(%q)\nparams := build_from_schema(info)\ncall %s(params)\n\n// BAD: guessing parameter names\ncall %s({random_field: value})\n```",
			ifEmpty(tool, "tool_name"),
			ifEmpty(tool, "tool_name"),
			ifEmpty(tool, "tool_name"),
		)
	case CatContextOverflow:
		return "```\n// GOOD: check context before large operations\nstats := call context_stats()\nif stats.tokens > 50000 { call /compress }\n\n// BAD: dumping full file into conversation\nprint(read_entire_large_file())  // BAD\n```"
	case CatSanitizerReject:
		return "```\n// GOOD: relative paths within CWD\ncall read_file({path: \"src/main.go\"})         // GOOD\ncall read_file({path: \"./docs/README.md\"})     // GOOD\n\n// BAD: adversarial inputs\ncall read_file({path: \"../../etc/passwd\"})     // BAD — path escape\ncall bash({command: \"echo\\x01injected\"})       // BAD — control char\n```"
	case CatProviderError:
		return "```\n// GOOD: check status and retry with backoff\nresult = call_with_retry(provider, max=3, backoff=exponential)\n\n// BAD: tight loop with no backoff\nfor { call provider() }  // BAD — exhausts quota\n```"
	default:
		return "Consult the tool's documentation for correct usage examples."
	}
}

func categoryChecklist(cat FailureCategory, tool string) string {
	switch cat {
	case CatHallucination:
		return fmt.Sprintf(
			"- [ ] Confirm `%s` appears in the output of `list_tools`\n"+
				"- [ ] Verify any file paths exist via `file_info` before referencing them\n"+
				"- [ ] Do not invent function names — only use documented API methods\n"+
				"- [ ] Re-read the user's request to confirm what was actually asked",
			ifEmpty(tool, "<tool_name>"),
		)
	case CatToolFailure:
		return fmt.Sprintf(
			"- [ ] Call `tool_info %s` and read the parameter schema\n"+
				"- [ ] Confirm all `required` parameters are supplied\n"+
				"- [ ] Validate path parameters are within the current working directory\n"+
				"- [ ] Check prerequisites (e.g. `git_status` before `git_commit`)",
			ifEmpty(tool, "<tool_name>"),
		)
	case CatContextOverflow:
		return "- [ ] Call `context_stats` at the start of long sessions\n" +
			"- [ ] Compress history if token count > 50,000 (`/compress`)\n" +
			"- [ ] Summarise large tool outputs before continuing\n" +
			"- [ ] Split multi-file operations across multiple turns"
	case CatSanitizerReject:
		return "- [ ] Strip all control characters from string parameters\n" +
			"- [ ] Use only relative paths that stay within the project directory\n" +
			"- [ ] Remove `?`, `#`, `%` from resource names and identifiers\n" +
			"- [ ] Validate parameters against the tool schema before submitting"
	case CatProviderError:
		return "- [ ] Check provider status before initiating long agentic tasks\n" +
			"- [ ] Implement back-off: 2s → 4s → 8s before giving up\n" +
			"- [ ] Notify the user if all providers fail\n" +
			"- [ ] Log the error and continue with reduced functionality if possible"
	default:
		return "- [ ] Review the failure pattern and apply appropriate mitigations"
	}
}

// DetectSuccessCandidates analyses a slice of trace events for successful
// multi-tool sessions that might benefit from being saved as reusable skills.
//
// Criteria: a session must have at least 5 tool_success events.
// Each qualifying session produces one SkillCandidate with Confidence=0.75.
func (se *SkillEvolver) DetectSuccessCandidates(traces []SENSETrace) []SkillCandidate {
	type sessionBucket struct {
		successCount int
		toolSequence []string        // ordered, deduplicated tool names
		toolSeen     map[string]bool // for deduplication
	}

	sessions := make(map[string]*sessionBucket)
	for i := range traces {
		ev := &traces[i]
		sid := ev.SessionID
		if sid == "" {
			sid = "__nosid__"
		}
		if _, ok := sessions[sid]; !ok {
			sessions[sid] = &sessionBucket{toolSeen: make(map[string]bool)}
		}
		b := sessions[sid]
		if ev.Kind == KindToolSuccess {
			b.successCount++
			if ev.ToolName != "" && !b.toolSeen[ev.ToolName] {
				b.toolSeen[ev.ToolName] = true
				b.toolSequence = append(b.toolSequence, ev.ToolName)
			}
		}
	}

	var candidates []SkillCandidate
	for sid, b := range sessions {
		if b.successCount < 5 {
			continue
		}

		// Build a task signature from the tool sequence.
		toolList := strings.Join(b.toolSequence, ", ")
		taskSignature := "multi-tool-session"
		if len(b.toolSequence) > 0 {
			// Use the first tool name as the base for the task signature.
			taskSignature = b.toolSequence[0]
			if len(b.toolSequence) > 1 {
				taskSignature += "-workflow"
			}
		}

		suggestedName := successSlug(taskSignature, sid)
		description := fmt.Sprintf("Auto-detected workflow using: %s (%d successful calls)",
			toolList, b.successCount)

		now := time.Now().UTC().Format("2006-01-02")
		draftContent := fmt.Sprintf(`---
name: %s
description: %s
---

## Procedure

Tools used in order: %s
Session: %s
Auto-detected from successful task execution.

*Generated by SENSE DetectSuccessCandidates on %s*
`, suggestedName, description, toolList, sid, now)

		candidates = append(candidates, SkillCandidate{
			SuggestedName: suggestedName,
			Description:   description,
			DraftContent:  draftContent,
			Confidence:    0.75,
			SessionID:     sid,
		})
	}

	// Sort by session ID for deterministic output.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].SessionID < candidates[j].SessionID
	})

	return candidates
}

// successSlug converts a task signature + session ID to a safe kebab-case slug.
var successSlugNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func successSlug(taskSignature, sessionID string) string {
	base := strings.ToLower(taskSignature)
	base = successSlugNonAlnum.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")
	if len(base) > 40 {
		base = base[:40]
	}
	// Append a short session suffix for uniqueness.
	suffix := sessionID
	if len(suffix) > 8 {
		suffix = suffix[:8]
	}
	suffix = successSlugNonAlnum.ReplaceAllString(strings.ToLower(suffix), "")
	if suffix != "" {
		return base + "-" + suffix
	}
	return base
}

// ── Slug + title helpers ────────────────────────────────────────────────────

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// patternSlug converts a ToolFailurePattern to a safe filename slug.
func patternSlug(p ToolFailurePattern) string {
	tool := strings.ToLower(p.Tool)
	if tool == "" || tool == "__provider__" {
		tool = "provider"
	}
	cat := strings.ToLower(string(p.Category))
	// e.g. "bash-toolfailure", "provider-contextoverflow"
	slug := tool + "-" + cat
	slug = nonAlnum.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if len(slug) > 60 {
		slug = slug[:60]
	}
	return slug
}

// titleFromSlug converts "bash-toolfailure" → "Bash Toolfailure".
func titleFromSlug(slug string) string {
	parts := strings.Split(slug, "-")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

func ifEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// ── Summary renderer ────────────────────────────────────────────────────────

func renderEvolveResult(r *EvolveResult, skillsDir string) string {
	var sb strings.Builder
	action := "Written"
	if r.DryRun {
		action = "Would write (dry-run)"
	}
	sb.WriteString("# SENSE Evolutionary Pipeline Result\n\n")
	if r.DryRun {
		sb.WriteString("> **DRY-RUN MODE** — no files were written to disk.\n")
		sb.WriteString("> Re-run with `--dry-run=false` to apply changes.\n\n")
	}
	sb.WriteString(fmt.Sprintf("**%s:** %d SKILL file(s)  \n", action, len(r.Skills)))
	sb.WriteString(fmt.Sprintf("**Skipped:** %d pattern(s) below evidence threshold  \n", r.Skipped))
	sb.WriteString(fmt.Sprintf("**Skills Directory:** `%s`\n\n", skillsDir))

	if len(r.Skills) > 0 {
		sb.WriteString("## Generated SKILL Files\n\n")
		for _, sf := range r.Skills {
			mark := "✅"
			if sf.DryRun {
				mark = "📋"
			}
			sb.WriteString(fmt.Sprintf("%s `%s` — %s (%d occurrence(s))\n",
				mark, sf.Filename, sf.Pattern.Category, sf.Pattern.Count))
		}
		sb.WriteString("\n")
	}

	if !r.DryRun && len(r.Skills) > 0 {
		sb.WriteString(
			"SKILL files are now active. Restart Gorkbot or run `/skills reload` " +
				"to load them into the system prompt.\n",
		)
	}

	return sb.String()
}
