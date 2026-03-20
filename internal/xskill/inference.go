package xskill

// inference.go — InferenceEngine: Phase 2 (Inference Loop / Solving).
//
// The InferenceEngine is the front-facing execution enricher (Gorkbot-Exec).
// Its PrepareContext method runs synchronously on the request path, BEFORE
// the primary LLM call is made, and returns an enriched system-prompt header
// that injects task-relevant skills and experiences into the agent's context.
//
// Pipeline (Phase 2):
//  1. Decompose the task into 2–3 sub-tasks (LLM)
//  2. Embed each sub-task query and retrieve top-3 experiences from the bank
//  3. Deduplicate the retrieved experiences
//  4. Rewrite experiences to fit this specific task (LLM)
//  5. Load the domain skill document (disk read)
//  6. Adapt the skill using the rewritten experiences (LLM)
//  7. Build and return the injection header string
//
// Graceful degradation: if the bank is empty or the skill file does not exist,
// PrepareContext returns an empty string (no injection) rather than an error.
// Non-fatal LLM errors at any step also cause a graceful degradation.

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ──────────────────────────────────────────────────────────────────────────────
// InferenceEngine struct
// ──────────────────────────────────────────────────────────────────────────────

// InferenceEngine enriches the agent's prompt with task-relevant experiences
// and an adapted skill document before each primary LLM call.
//
// It is safe for concurrent use provided the underlying KnowledgeBase and
// LLMProvider are also safe for concurrent use.
type InferenceEngine struct {
	// kb is the shared KnowledgeBase that provides the Experience Bank and
	// the path to skill documents.
	kb *KnowledgeBase

	// provider is the LLM backend for decomposition, rewriting, and adaptation.
	// May be the same instance used by KnowledgeBase or a separate one.
	provider LLMProvider
}

// NewInferenceEngine creates an InferenceEngine that reads from kb and performs
// LLM calls via provider.
//
// It is valid (and common) to pass the same LLMProvider instance used by
// KnowledgeBase — all Gorkbot providers are safe for concurrent use.
func NewInferenceEngine(kb *KnowledgeBase, provider LLMProvider) (*InferenceEngine, error) {
	if kb == nil {
		return nil, fmt.Errorf("xskill: InferenceEngine requires a non-nil KnowledgeBase")
	}
	if provider == nil {
		return nil, fmt.Errorf("xskill: InferenceEngine requires a non-nil LLMProvider")
	}
	return &InferenceEngine{kb: kb, provider: provider}, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Public API
// ──────────────────────────────────────────────────────────────────────────────

// PrepareContext runs the full Phase 2 pipeline for a single task and returns
// a ready-to-inject system prompt header.
//
// skillName must match the domain label used in KnowledgeBase.Accumulate for
// this task class (e.g. "visual-logic", "search-tactics").  The same value
// selects the correct skill document from the skills directory.
//
// Return values:
//   - (header, nil) when enrichment succeeds.  header is the complete injection
//     string; prepend it to the agent's system prompt.
//   - ("", nil) when the bank is empty, the skill file is missing, or not enough
//     information exists to generate a useful header.  The caller should proceed
//     without injection.
//   - ("", err) only for unrecoverable internal errors (malformed JSON, etc.).
func (ie *InferenceEngine) PrepareContext(taskDescription, skillName string) (string, error) {
	taskDescription = strings.TrimSpace(taskDescription)
	if taskDescription == "" {
		return "", fmt.Errorf("xskill: PrepareContext requires a non-empty taskDescription")
	}

	// ── Step 1: Decompose task into 2–3 retrieval sub-tasks ──────────────────
	decomp, err := ie.decomposeTask(taskDescription)
	if err != nil {
		// Decomposition failure is non-fatal — skip injection.
		return "", nil
	}
	if len(decomp.SubTasks) == 0 {
		return "", nil
	}

	// ── Step 2: Vector retrieval from the Experience Bank ────────────────────
	retrieved, err := ie.retrieveExperiences(decomp)
	if err != nil || len(retrieved) == 0 {
		// No experiences to inject — skip injection.
		return "", nil
	}

	// ── Step 3: Rewrite experiences to fit this specific task ────────────────
	rewrite, err := ie.rewriteExperiences(taskDescription, retrieved)
	if err != nil || len(rewrite) == 0 {
		// Fallback: use raw experience text without LLM adaptation.
		rewrite = rawRewriteFallback(retrieved)
	}

	// ── Step 4: Load the domain skill document ───────────────────────────────
	skillContent, err := ie.loadSkillDocument(skillName)
	if err != nil {
		skillContent = "" // missing skill is not an error
	}

	// ── Step 5: Adapt the skill document using rewritten experiences ─────────
	var adaptedSkill string
	if skillContent != "" {
		expBullets := formatRewriteAsBullets(rewrite, retrieved)
		adaptedSkill, err = ie.adaptSkill(taskDescription, skillContent, expBullets)
		if err != nil {
			adaptedSkill = skillContent // fallback: use raw skill document
		}
	}

	// If we have neither skill nor experiences, there is nothing to inject.
	if adaptedSkill == "" && len(rewrite) == 0 {
		return "", nil
	}

	// ── Step 6: Build injection header ───────────────────────────────────────
	header := ie.buildInjectionHeader(adaptedSkill, rewrite, retrieved)
	return header, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Phase 2 — Step 1: Task Decomposition
// ──────────────────────────────────────────────────────────────────────────────

// decomposeTask calls the LLM to break the task description into 2–3 typed
// retrieval sub-tasks.
func (ie *InferenceEngine) decomposeTask(taskDescription string) (TaskDecomposition, error) {
	sys, user := promptDecomposeTask(taskDescription)
	response, err := ie.provider.Generate(sys, user)
	if err != nil {
		return TaskDecomposition{}, fmt.Errorf("xskill: task decomposition LLM call failed: %w", err)
	}

	// Extract the JSON object from the response (LLM may include preamble).
	raw := extractLastJSON(response)
	if raw == "" {
		return TaskDecomposition{}, fmt.Errorf("xskill: no JSON found in decomposition response")
	}

	var decomp TaskDecomposition
	if err := json.Unmarshal([]byte(raw), &decomp); err != nil {
		return TaskDecomposition{}, fmt.Errorf("xskill: cannot parse decomposition JSON: %w", err)
	}
	return decomp, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Phase 2 — Step 2: Vector Retrieval
// ──────────────────────────────────────────────────────────────────────────────

// retrieveExperiences embeds each sub-task query, retrieves top-k experiences
// per sub-task from the bank, and returns the deduplicated union.
//
// It acquires only a read lock on the bank, so Phase 1 accumulation can
// continue concurrently in the background.
func (ie *InferenceEngine) retrieveExperiences(decomp TaskDecomposition) ([]Experience, error) {
	// Snapshot the bank under a read lock for the duration of all retrieval.
	snap := ie.kb.Snapshot() // returns a deep copy; no lock needed after this
	if len(snap.Experiences) == 0 {
		return nil, nil
	}

	var allIndices []int

	for _, st := range decomp.SubTasks {
		if strings.TrimSpace(st.Query) == "" {
			continue
		}

		// Embed the sub-task query (LLM call — no lock held).
		qVec, err := ie.provider.Embed(st.Query)
		if err != nil {
			continue // skip this sub-task on embed failure
		}

		// Retrieve top-k indices from the snapshot.
		topK := TopKExperiences(qVec, snap.Experiences, TopKRetrieval)
		allIndices = append(allIndices, topK...)
	}

	if len(allIndices) == 0 {
		return nil, nil
	}

	// Deduplicate indices, preserving the order in which they were first found.
	uniqueIndices := deduplicateIndices(allIndices)

	// Collect the corresponding Experience structs.
	result := make([]Experience, 0, len(uniqueIndices))
	for _, idx := range uniqueIndices {
		if idx >= 0 && idx < len(snap.Experiences) {
			result = append(result, snap.Experiences[idx])
		}
	}
	return result, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Phase 2 — Step 3: Experience Rewriting
// ──────────────────────────────────────────────────────────────────────────────

// rewriteExperiences adapts the retrieved experiences to the specific task by
// asking the LLM to reframe them in the vocabulary of the task domain.
func (ie *InferenceEngine) rewriteExperiences(taskDescription string, exps []Experience) (ExperienceRewrite, error) {
	if len(exps) == 0 {
		return nil, nil
	}

	// Format experiences for the LLM.
	var sb strings.Builder
	for _, e := range exps {
		sb.WriteString(fmt.Sprintf("[%s] %s %s\n", e.ID, e.Condition, e.Action))
	}

	sys, user := promptRewriteExperiences(taskDescription, sb.String())
	response, err := ie.provider.Generate(sys, user)
	if err != nil {
		return nil, fmt.Errorf("xskill: experience rewrite LLM call failed: %w", err)
	}

	// Extract the JSON map from the response.
	raw := extractLastJSON(response)
	if raw == "" {
		return nil, fmt.Errorf("xskill: no JSON found in rewrite response")
	}

	var rewrite ExperienceRewrite
	if err := json.Unmarshal([]byte(raw), &rewrite); err != nil {
		return nil, fmt.Errorf("xskill: cannot parse rewrite JSON: %w", err)
	}
	return rewrite, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Phase 2 — Steps 4 & 5: Skill Loading and Adaptation
// ──────────────────────────────────────────────────────────────────────────────

// loadSkillDocument reads the domain skill Markdown file from disk.
// Returns ("", nil) when the file does not exist (not an error condition).
func (ie *InferenceEngine) loadSkillDocument(skillName string) (string, error) {
	path := ie.kb.globalSkillPath(skillName)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("xskill: cannot read skill document %q: %w", path, err)
	}
	return string(data), nil
}

// adaptSkill calls the LLM to tailor the global skill document to the specific
// task, integrating the rewritten experiences.
func (ie *InferenceEngine) adaptSkill(taskDescription, skillContent, expBullets string) (string, error) {
	sys, user := promptAdaptSkill(taskDescription, skillContent, expBullets)
	adapted, err := ie.provider.Generate(sys, user)
	if err != nil {
		return "", fmt.Errorf("xskill: skill adaptation LLM call failed: %w", err)
	}
	return strings.TrimSpace(adapted), nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Phase 2 — Step 6: Injection Header Assembly
// ──────────────────────────────────────────────────────────────────────────────

// buildInjectionHeader assembles the final prompt injection header using the
// exact structure mandated by the XSKILL specification.
//
// Format:
//
//	Here are practical experiences and skills for tool-based visual reasoning:
//	<skill>
//	{adapted_skill}
//	</skill>
//	• [E1] rewritten guidance…
//	• [E3] rewritten guidance…
//
//	You can use it as reference if it is relevant to help you solve the problem.
//	You can also have your own ideas or other approaches. Your instruction is following:
func (ie *InferenceEngine) buildInjectionHeader(
	adaptedSkill string,
	rewrite ExperienceRewrite,
	sourceExps []Experience,
) string {
	var sb strings.Builder

	sb.WriteString("Here are practical experiences and skills for tool-based visual reasoning:\n")

	// Skill block — only included when a skill was successfully adapted.
	if adaptedSkill != "" {
		sb.WriteString("<skill>\n")
		sb.WriteString(adaptedSkill)
		sb.WriteString("\n</skill>\n")
	}

	// Experience bullets — iterate over sourceExps to preserve retrieval order.
	if len(rewrite) > 0 {
		sb.WriteString("\n")
		for _, e := range sourceExps {
			text, ok := rewrite[e.ID]
			if !ok || strings.TrimSpace(text) == "" {
				continue
			}
			sb.WriteString(fmt.Sprintf("• [%s] %s\n", e.ID, text))
		}
	}

	sb.WriteString("\nYou can use it as reference if it is relevant to help you solve the problem. ")
	sb.WriteString("You can also have your own ideas or other approaches. ")
	sb.WriteString("Your instruction is following:")

	return sb.String()
}

// ──────────────────────────────────────────────────────────────────────────────
// Internal helpers
// ──────────────────────────────────────────────────────────────────────────────

// rawRewriteFallback builds a trivial ExperienceRewrite from the raw experiences
// when the LLM rewrite call fails.  Each experience maps its ID to the raw
// "Condition + Action" text.
func rawRewriteFallback(exps []Experience) ExperienceRewrite {
	out := make(ExperienceRewrite, len(exps))
	for _, e := range exps {
		text := strings.TrimSpace(e.Condition)
		if e.Action != "" {
			text += " " + strings.TrimSpace(e.Action)
		}
		out[e.ID] = text
	}
	return out
}

// formatRewriteAsBullets renders the ExperienceRewrite map as a bullet-point
// string suitable for injection into the skill adaptation prompt.
// sourceExps determines the order (retrieval rank order is preserved).
func formatRewriteAsBullets(rewrite ExperienceRewrite, sourceExps []Experience) string {
	var sb strings.Builder
	for _, e := range sourceExps {
		text, ok := rewrite[e.ID]
		if !ok || strings.TrimSpace(text) == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("• [%s] %s\n", e.ID, text))
	}
	return strings.TrimRight(sb.String(), "\n")
}
