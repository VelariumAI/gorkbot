package xskill

// kb.go — KnowledgeBase: Phase 1 (Accumulation Loop).
//
// The KnowledgeBase is the background knowledge manager (Gorkbot-KB).
// Its Accumulate method is designed to run inside a goroutine while the main
// agent handles the next user request.
//
// Data layout on disk (~/.gorkbot/xskill_kb/ by default):
//
//	experiences.json          — the Experience Bank (all tactical experiences)
//	skills/visual-logic.md    — domain skill document for visual reasoning
//	skills/search-tactics.md  — domain skill document for search tasks
//	skills/<skillName>.md     — one file per task class, never a monolithic blob
//
// Thread safety:
//   - All reads/writes to bank ([]Experience) are protected by mu (sync.RWMutex).
//   - ID generation uses nextID (atomic.Int64) — no mutex needed for incrementing.
//   - LLM calls (Generate, Embed) are performed WITHOUT holding any lock to
//     avoid blocking concurrent readers.
//   - saveBank() requires the WRITE lock to be held by the caller.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ──────────────────────────────────────────────────────────────────────────────
// KnowledgeBase struct
// ──────────────────────────────────────────────────────────────────────────────

// KnowledgeBase manages the Experience Bank and Skill Library on behalf of
// the XSKILL framework.  All public methods are safe for concurrent use.
type KnowledgeBase struct {
	// mu guards bank and all in-memory state that derives from it.
	// LLM calls must NEVER be made while this lock is held.
	mu sync.RWMutex

	// bank is the in-memory representation of experiences.json.
	bank ExperienceBank

	// bankPath is the absolute path to experiences.json.
	bankPath string

	// skillsDir is the absolute path to the skills/ subdirectory.
	// Each domain skill is stored as skillsDir/<skillName>.md.
	skillsDir string

	// provider is the injected LLM backend (Generate + Embed).
	provider LLMProvider

	// nextID is the thread-safe sequential ID counter for experience IDs
	// (E1, E2, …).  It is initialised from the maximum existing ID when
	// the bank is loaded.  Uses atomic.Int64 so ID generation never
	// requires the mu write lock, preventing contention between concurrent
	// Accumulate calls.
	nextID atomic.Int64
}

// NewKnowledgeBase creates and initialises a KnowledgeBase.
//
// If baseDir is empty the XSkill data directory defaults to
// ~/.gorkbot/xskill_kb (resolved via os.UserHomeDir).  Pass a non-empty
// baseDir to override (useful for testing with a temp directory).
//
// The required subdirectories are bootstrapped with os.MkdirAll — safe to call
// even if they already exist.  The call will fail only if directory creation
// itself fails (e.g. permission denied).
func NewKnowledgeBase(baseDir string, provider LLMProvider) (*KnowledgeBase, error) {
	if provider == nil {
		return nil, fmt.Errorf("xskill: KnowledgeBase requires a non-nil LLMProvider")
	}

	// Resolve the base directory.
	if baseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("xskill: cannot resolve home directory: %w", err)
		}
		baseDir = filepath.Join(home, ".gorkbot", "xskill_kb")
	}

	// Bootstrap directories.  Mode 0700 — only the current user should read
	// the experience bank (it may contain sensitive operational patterns).
	skillsDir := filepath.Join(baseDir, "skills")
	for _, dir := range []string{baseDir, skillsDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("xskill: cannot create directory %q: %w", dir, err)
		}
	}

	bankPath := filepath.Join(baseDir, "experiences.json")

	kb := &KnowledgeBase{
		bankPath:  bankPath,
		skillsDir: skillsDir,
		provider:  provider,
	}

	// Load existing bank (or initialise a fresh one).
	if err := kb.loadBank(); err != nil {
		return nil, err
	}

	return kb, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Public API
// ──────────────────────────────────────────────────────────────────────────────

// Accumulate processes a completed task trajectory and updates the Experience
// Bank and Skill Library.  This is the Phase 1 (Accumulation Loop) entry point.
//
// skillName identifies the task class / domain for the skill document (e.g.
// "visual-logic", "search-tactics", "code-review").  It must be a short,
// lowercase, hyphen-separated identifier.  Each distinct skillName produces
// its own Markdown document in skillsDir/, preventing the 1000-word limit from
// constantly triggering destructive refinement of an unrelated skill.
//
// Accumulate is safe to call from a goroutine.  A typical invocation:
//
//	go func() {
//	    if err := kb.Accumulate(completedTraj, "visual-logic"); err != nil {
//	        slog.Error("xskill accumulate failed", "err", err)
//	    }
//	}()
//
// The method returns an error if any mandatory LLM call fails.  Non-fatal
// operations (pruning, skill refinement) do not surface errors to the caller
// but are silently best-efforted.
func (kb *KnowledgeBase) Accumulate(traj Trajectory, skillName string) error {
	// ── Step 1: Generate rollout summary ─────────────────────────────────────
	summary := traj.Summary
	if summary == "" {
		var err error
		summary, err = kb.generateRolloutSummary(traj)
		if err != nil {
			return fmt.Errorf("xskill: rollout summary failed: %w", err)
		}
	}

	// ── Step 2: Cross-rollout critique → extract experience candidate ─────────
	critique, err := kb.generateCritique(traj, summary)
	if err != nil {
		return fmt.Errorf("xskill: cross-rollout critique failed: %w", err)
	}

	// ── Step 3: Process the critique (add new or modify existing experience) ──
	if err := kb.processCritique(critique); err != nil {
		// Non-fatal: failing to update the bank should not block skill learning.
		_ = err
	}

	// ── Step 4: Global prune if library exceeded ──────────────────────────────
	kb.mu.RLock()
	size := len(kb.bank.Experiences)
	kb.mu.RUnlock()
	if size > MaxExperienceLibSize {
		// Best-effort prune: do not fail the accumulation if pruning fails.
		_ = kb.pruneLibrary()
	}

	// ── Step 5: Generate raw skill SOP from trajectory ────────────────────────
	rawSkill, err := kb.generateRawSkill(traj)
	if err != nil {
		return fmt.Errorf("xskill: raw skill generation failed: %w", err)
	}

	// ── Step 6: Merge raw skill into domain-specific global skill ─────────────
	if err := kb.mergeIntoGlobalSkill(rawSkill, skillName); err != nil {
		return fmt.Errorf("xskill: skill merge failed: %w", err)
	}

	return nil
}

// Snapshot returns a deep copy of the current ExperienceBank.  The returned
// value is safe to read without holding any locks.
func (kb *KnowledgeBase) Snapshot() ExperienceBank {
	kb.mu.RLock()
	defer kb.mu.RUnlock()

	// Deep copy to prevent the caller from observing mutations.
	snap := ExperienceBank{
		Version:     kb.bank.Version,
		UpdatedAt:   kb.bank.UpdatedAt,
		Experiences: make([]Experience, len(kb.bank.Experiences)),
	}
	copy(snap.Experiences, kb.bank.Experiences)
	return snap
}

// SkillsDir returns the directory path where domain skill documents are stored.
func (kb *KnowledgeBase) SkillsDir() string { return kb.skillsDir }

// ──────────────────────────────────────────────────────────────────────────────
// Bank I/O
// ──────────────────────────────────────────────────────────────────────────────

// loadBank reads experiences.json from disk into kb.bank.
// If the file does not exist a fresh empty bank is initialised.
// Must be called during construction (before any goroutines are started).
func (kb *KnowledgeBase) loadBank() error {
	kb.mu.Lock()
	defer kb.mu.Unlock()

	data, err := os.ReadFile(kb.bankPath)
	if os.IsNotExist(err) {
		// Fresh installation — start with an empty bank.
		kb.bank = ExperienceBank{
			Version:     ExperienceBankVersion,
			Experiences: []Experience{},
			UpdatedAt:   time.Now().UTC(),
		}
		kb.nextID.Store(0)
		return nil
	}
	if err != nil {
		return fmt.Errorf("xskill: cannot read experience bank %q: %w", kb.bankPath, err)
	}

	if err := json.Unmarshal(data, &kb.bank); err != nil {
		return fmt.Errorf("xskill: cannot parse experience bank %q: %w", kb.bankPath, err)
	}

	// Ensure the slice is non-nil (guards against "null" in JSON).
	if kb.bank.Experiences == nil {
		kb.bank.Experiences = []Experience{}
	}

	// Initialise atomic counter from the highest existing numeric ID.
	kb.nextID.Store(int64(maxExperienceID(kb.bank.Experiences)))
	return nil
}

// saveBank serialises kb.bank to disk using an atomic rename.
//
// IMPORTANT: The caller MUST hold kb.mu.Lock() (write lock) before calling
// saveBank.  Holding the lock guarantees that no other goroutine mutates
// kb.bank between the json.Marshal call and the final Rename.
func (kb *KnowledgeBase) saveBank() error {
	data, err := json.MarshalIndent(kb.bank, "", "  ")
	if err != nil {
		return fmt.Errorf("xskill: cannot marshal experience bank: %w", err)
	}

	// Write to a temp file first, then atomically rename.  This prevents a
	// partial write from corrupting the bank if the process is interrupted.
	tmpPath := kb.bankPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("xskill: cannot write experience bank temp file %q: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, kb.bankPath); err != nil {
		_ = os.Remove(tmpPath) // best-effort cleanup
		return fmt.Errorf("xskill: cannot commit experience bank %q: %w", kb.bankPath, err)
	}
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Phase 1 — Step 1: Rollout Summary
// ──────────────────────────────────────────────────────────────────────────────

// generateRolloutSummary calls the LLM to produce a structured step-by-step
// narrative of the trajectory.
func (kb *KnowledgeBase) generateRolloutSummary(traj Trajectory) (string, error) {
	trajText := formatTrajectory(traj)
	sys, user := promptRolloutSummary(trajText)
	return kb.provider.Generate(sys, user)
}

// ──────────────────────────────────────────────────────────────────────────────
// Phase 1 — Step 2: Cross-Rollout Critique
// ──────────────────────────────────────────────────────────────────────────────

// generateCritique calls the LLM to extract a new or modified experience from
// the trajectory summary.
func (kb *KnowledgeBase) generateCritique(traj Trajectory, summary string) (RawCritique, error) {
	usedIDs := strings.Join(traj.ExperiencesUsed, ", ")
	if usedIDs == "" {
		usedIDs = "(none)"
	}

	sys, user := promptCrossRolloutCritique(traj.Question, summary, usedIDs, traj.GroundTruth)
	response, err := kb.provider.Generate(sys, user)
	if err != nil {
		return RawCritique{}, fmt.Errorf("xskill: critique LLM call failed: %w", err)
	}

	raw := extractLastJSON(response)
	if raw == "" {
		return RawCritique{}, fmt.Errorf("xskill: no JSON found in critique response")
	}

	var critique RawCritique
	if err := json.Unmarshal([]byte(raw), &critique); err != nil {
		return RawCritique{}, fmt.Errorf("xskill: cannot parse critique JSON %q: %w", raw, err)
	}
	if critique.Option != "add" && critique.Option != "modify" {
		return RawCritique{}, fmt.Errorf("xskill: unexpected critique option %q (want add|modify)", critique.Option)
	}
	if strings.TrimSpace(critique.Experience) == "" {
		return RawCritique{}, fmt.Errorf("xskill: critique has empty experience field")
	}
	return critique, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Phase 1 — Step 3: Process Critique
// ──────────────────────────────────────────────────────────────────────────────

// processCritique dispatches an "add" or "modify" critique to the appropriate
// experience management method.
func (kb *KnowledgeBase) processCritique(critique RawCritique) error {
	text := truncateToWords(strings.TrimSpace(critique.Experience), MaxExperienceWords)
	switch critique.Option {
	case "add":
		return kb.addExperience(text)
	case "modify":
		if critique.ModifiedFrom == "" {
			return fmt.Errorf("xskill: modify critique missing modified_from field")
		}
		return kb.modifyExperience(critique.ModifiedFrom, text)
	default:
		return fmt.Errorf("xskill: unknown critique option %q", critique.Option)
	}
}

// addExperience embeds text, checks for near-duplicate experiences, and either
// merges with the nearest similar entry or appends a new entry.
//
// Call sequence (illustrating lock discipline):
//  1. Embed text — no lock held (LLM call)
//  2. RLock → similarity search → RUnlock
//  3. If similar: LLM merge call — no lock; re-embed — no lock
//  4. Lock → mutate bank → saveBank → Unlock
func (kb *KnowledgeBase) addExperience(text string) error {
	// Step 1: Embed new experience text (no lock held during LLM call).
	vec, err := kb.provider.Embed(text)
	if err != nil {
		// On embed failure, add the experience without a vector (it will be
		// skipped during similarity search but still present for reference).
		return kb.appendExperience(text, nil)
	}

	// Step 2: Find the most similar existing experience (read lock only).
	kb.mu.RLock()
	topK := TopKExperiences(vec, kb.bank.Experiences, 1)
	var (
		simID    string
		simText  string
		simScore float64
	)
	if len(topK) > 0 {
		idx := topK[0]
		simScore = CosineSimilarity64(vec, kb.bank.Experiences[idx].Vector)
		if simScore >= SimilarityMergeThreshold {
			simID = kb.bank.Experiences[idx].ID
			simText = kb.bank.Experiences[idx].Condition + "\n" + kb.bank.Experiences[idx].Action
		}
	}
	kb.mu.RUnlock()

	// Step 3a: Merge — release all locks during LLM call.
	if simID != "" {
		sys, user := promptMergeExperiences(simText, text)
		merged, mergeErr := kb.provider.Generate(sys, user)
		if mergeErr != nil {
			// Merge LLM call failed — fall back to appending as a new entry.
			return kb.appendExperience(text, vec)
		}
		merged = truncateToWords(strings.TrimSpace(merged), MaxExperienceWords)

		// Re-embed the merged text (still no lock held).
		mergedVec, embErr := kb.provider.Embed(merged)
		if embErr != nil {
			mergedVec = vec // fall back to new-text vector
		}

		// Update the existing entry under write lock.
		cond, action := splitCondAction(merged)
		now := time.Now().UTC()
		kb.mu.Lock()
		defer kb.mu.Unlock()
		for i, e := range kb.bank.Experiences {
			if e.ID == simID {
				kb.bank.Experiences[i].Condition = cond
				kb.bank.Experiences[i].Action = action
				kb.bank.Experiences[i].Vector = mergedVec
				kb.bank.Experiences[i].UpdatedAt = now
				break
			}
		}
		kb.bank.UpdatedAt = now
		return kb.saveBank()
	}

	// Step 3b: No near-duplicate found — append as a new entry.
	return kb.appendExperience(text, vec)
}

// appendExperience adds a brand-new experience to the bank.
// ID generation uses the atomic nextID counter — never blocks on the RW mutex.
func (kb *KnowledgeBase) appendExperience(text string, vec []float64) error {
	cond, action := splitCondAction(text)
	now := time.Now().UTC()

	// Atomically generate the next ID — safe for concurrent Accumulate calls.
	n := kb.nextID.Add(1)
	id := fmt.Sprintf("E%d", n)

	kb.mu.Lock()
	defer kb.mu.Unlock()

	kb.bank.Experiences = append(kb.bank.Experiences, Experience{
		ID:        id,
		Condition: cond,
		Action:    action,
		Vector:    vec,
		CreatedAt: now,
		UpdatedAt: now,
	})
	kb.bank.UpdatedAt = now
	return kb.saveBank()
}

// modifyExperience replaces the content of the experience with the given id.
// Re-embeds the new text so future similarity searches remain accurate.
func (kb *KnowledgeBase) modifyExperience(id, newText string) error {
	// Embed outside lock (LLM call).
	vec, err := kb.provider.Embed(newText)
	if err != nil {
		vec = nil // update text fields even if embed fails
	}

	cond, action := splitCondAction(newText)
	now := time.Now().UTC()

	kb.mu.Lock()
	defer kb.mu.Unlock()

	for i, e := range kb.bank.Experiences {
		if e.ID == id {
			kb.bank.Experiences[i].Condition = cond
			kb.bank.Experiences[i].Action = action
			if vec != nil {
				kb.bank.Experiences[i].Vector = vec
			}
			kb.bank.Experiences[i].UpdatedAt = now
			kb.bank.UpdatedAt = now
			return kb.saveBank()
		}
	}
	return fmt.Errorf("xskill: experience %q not found for modify", id)
}

// ──────────────────────────────────────────────────────────────────────────────
// Phase 1 — Step 4: Global Library Prune
// ──────────────────────────────────────────────────────────────────────────────

// pruneLibrary asks the LLM to perform a global refinement pass when the
// experience library exceeds MaxExperienceLibSize entries.
//
// The LLM emits a sequence of JSON operation lines (merge or delete); all
// embedding for merged results is done in parallel with the write lock NOT held.
func (kb *KnowledgeBase) pruneLibrary() error {
	// Snapshot the current library (read lock only).
	kb.mu.RLock()
	expCount := len(kb.bank.Experiences)
	expJSON, err := json.Marshal(kb.bank.Experiences)
	kb.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("xskill: cannot marshal library for pruning: %w", err)
	}

	// LLM call — no lock held.
	sys, user := promptRefineLibrary(expCount, string(expJSON))
	response, err := kb.provider.Generate(sys, user)
	if err != nil {
		return fmt.Errorf("xskill: prune LLM call failed: %w", err)
	}

	ops := parseRefinementOps(response)
	if len(ops) == 0 {
		return nil // LLM returned no operations — nothing to do
	}

	// Re-embed merged results outside the write lock.
	type mergeResult struct {
		targetID  string    // experience ID to keep (first of ids[])
		deleteIDs []string  // additional IDs to remove after merge
		text      string    // merged experience text
		vec       []float64 // embedding of merged text
	}

	var merges []mergeResult
	var toDelete []string // IDs collected from "delete" ops

	for _, op := range ops {
		switch op.Op {
		case "merge":
			if len(op.IDs) < 2 || strings.TrimSpace(op.Result) == "" {
				continue
			}
			mergedText := truncateToWords(strings.TrimSpace(op.Result), MaxExperienceWords)
			vec, _ := kb.provider.Embed(mergedText) // best-effort; nil on failure
			merges = append(merges, mergeResult{
				targetID:  op.IDs[0],
				deleteIDs: op.IDs[1:],
				text:      mergedText,
				vec:       vec,
			})
		case "delete":
			if op.ID != "" {
				toDelete = append(toDelete, op.ID)
			}
		}
	}

	// Apply all operations atomically under write lock.
	kb.mu.Lock()
	defer kb.mu.Unlock()

	now := time.Now().UTC()

	// Build the complete delete set (merge losers + explicit deletes).
	deleteSet := make(map[string]bool, len(toDelete)+len(merges)*2)
	for _, id := range toDelete {
		deleteSet[id] = true
	}
	for _, m := range merges {
		for _, id := range m.deleteIDs {
			deleteSet[id] = true
		}
	}

	// Update merge winners with the new merged content.
	for _, m := range merges {
		cond, action := splitCondAction(m.text)
		for i, e := range kb.bank.Experiences {
			if e.ID == m.targetID {
				kb.bank.Experiences[i].Condition = cond
				kb.bank.Experiences[i].Action = action
				if m.vec != nil {
					kb.bank.Experiences[i].Vector = m.vec
				}
				kb.bank.Experiences[i].UpdatedAt = now
				break
			}
		}
	}

	// Filter out deleted entries in-place (avoids extra allocation).
	kept := kb.bank.Experiences[:0]
	for _, e := range kb.bank.Experiences {
		if !deleteSet[e.ID] {
			kept = append(kept, e)
		}
	}
	kb.bank.Experiences = kept
	kb.bank.UpdatedAt = now

	return kb.saveBank()
}

// ──────────────────────────────────────────────────────────────────────────────
// Phase 1 — Steps 5 & 6: Skill Generation and Merging
// ──────────────────────────────────────────────────────────────────────────────

// generateRawSkill calls the LLM to extract a raw Standard Operating Procedure
// from the trajectory.
func (kb *KnowledgeBase) generateRawSkill(traj Trajectory) (string, error) {
	sys, user := promptGenerateSkill(formatTrajectory(traj))
	raw, err := kb.provider.Generate(sys, user)
	if err != nil {
		return "", fmt.Errorf("xskill: skill generation LLM call failed: %w", err)
	}
	return strings.TrimSpace(raw), nil
}

// mergeIntoGlobalSkill integrates a raw skill SOP into the domain-specific
// global skill document identified by skillName.
//
// If no global skill exists for this domain yet, the raw skill is written
// directly.  Otherwise the LLM merges both documents.  If the result exceeds
// MaxSkillWords a refinement pass is triggered.
//
// skillName is sanitized before use as a filename (see sanitizeSkillName).
func (kb *KnowledgeBase) mergeIntoGlobalSkill(rawSkill, skillName string) error {
	path := kb.globalSkillPath(skillName)

	// Load current global skill (if any).
	existingContent, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("xskill: cannot read global skill %q: %w", path, err)
	}

	var finalContent string
	if len(existingContent) == 0 {
		// No existing skill for this domain — use raw skill as the first version.
		finalContent = rawSkill
	} else {
		// Merge the new raw skill into the existing global skill (LLM call — no lock).
		sys, user := promptMergeSkill(string(existingContent), rawSkill)
		merged, mergeErr := kb.provider.Generate(sys, user)
		if mergeErr != nil {
			return fmt.Errorf("xskill: skill merge LLM call failed: %w", mergeErr)
		}
		finalContent = strings.TrimSpace(merged)
	}

	// If the merged document is too long, run a refinement pass.
	if wordCount(finalContent) > MaxSkillWords {
		sys, user := promptRefineSkill(finalContent)
		refined, refErr := kb.provider.Generate(sys, user)
		if refErr == nil { // non-fatal: if refine fails, keep the longer version
			finalContent = strings.TrimSpace(refined)
		}
	}

	// Write the final skill document.  Mode 0600 — private to current user.
	if err := os.WriteFile(path, []byte(finalContent), 0600); err != nil {
		return fmt.Errorf("xskill: cannot write global skill %q: %w", path, err)
	}
	return nil
}

// globalSkillPath returns the absolute filesystem path for the domain skill
// document identified by skillName.
//
// skillName is sanitized to a safe lowercase kebab-case identifier before
// being used as a filename, preventing any path traversal.
func (kb *KnowledgeBase) globalSkillPath(skillName string) string {
	safe := sanitizeSkillName(skillName)
	if safe == "" {
		safe = "general"
	}
	return filepath.Join(kb.skillsDir, safe+".md")
}

// ──────────────────────────────────────────────────────────────────────────────
// Internal helpers
// ──────────────────────────────────────────────────────────────────────────────

// sanitizeSkillNameRe is the compiled regexp used by sanitizeSkillName.
var sanitizeSkillNameRe = regexp.MustCompile(`[^a-z0-9_-]+`)

// sanitizeSkillName converts an arbitrary skill name string into a safe
// lowercase filename identifier (alphanumeric, hyphens, underscores only).
// Maximum length is capped at 50 characters.  Returns "general" if the
// sanitized result is empty.
func sanitizeSkillName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = sanitizeSkillNameRe.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	if len(name) > 50 {
		name = name[:50]
	}
	return name
}

// maxExperienceID scans a slice of experiences and returns the highest numeric
// suffix found in IDs matching "E<n>".  Returns 0 if no numeric IDs exist.
// Used to initialise the atomic nextID counter after loading the bank.
func maxExperienceID(exps []Experience) int {
	max := 0
	for _, e := range exps {
		var n int
		if _, err := fmt.Sscanf(e.ID, "E%d", &n); err == nil && n > max {
			max = n
		}
	}
	return max
}

// splitCondAction splits a free-form experience string into a (condition, action)
// pair.  The heuristic tries, in order:
//  1. Double newline — condition is the first paragraph.
//  2. Single newline — condition is the first line.
//  3. ". " — condition is the first sentence.
//  4. Fallback: the whole string is the condition, action is empty.
func splitCondAction(text string) (condition, action string) {
	text = strings.TrimSpace(text)

	if parts := strings.SplitN(text, "\n\n", 2); len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	if parts := strings.SplitN(text, "\n", 2); len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	if i := strings.Index(text, ". "); i != -1 {
		return strings.TrimSpace(text[:i+1]), strings.TrimSpace(text[i+2:])
	}
	return text, ""
}

// wordCount returns the number of whitespace-separated tokens in s.
func wordCount(s string) int {
	return len(strings.Fields(s))
}

// truncateToWords returns s with at most maxWords whitespace-separated tokens.
// If no truncation is needed, s is returned unchanged.
func truncateToWords(s string, maxWords int) string {
	fields := strings.Fields(s)
	if len(fields) <= maxWords {
		return s
	}
	return strings.Join(fields[:maxWords], " ")
}

// formatTrajectory renders a Trajectory into a human-readable string suitable
// for injection into LLM prompts.
func formatTrajectory(traj Trajectory) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Task: %s\n", traj.Question))
	if traj.GroundTruth != "" {
		sb.WriteString(fmt.Sprintf("Correct Answer: %s\n", traj.GroundTruth))
	}
	if len(traj.ExperiencesUsed) > 0 {
		sb.WriteString(fmt.Sprintf("Experiences Injected: %s\n", strings.Join(traj.ExperiencesUsed, ", ")))
	}
	sb.WriteString(fmt.Sprintf("\nSteps (%d total):\n", len(traj.Steps)))
	for _, step := range traj.Steps {
		sb.WriteString(fmt.Sprintf("\n[Step %d] Tool: %s\n", step.StepIndex+1, step.ToolName))
		if step.Parameters != "" {
			sb.WriteString(fmt.Sprintf("  Params: %s\n", step.Parameters))
		}
		if step.Reasoning != "" {
			sb.WriteString(fmt.Sprintf("  Reasoning: %s\n", step.Reasoning))
		}
		if step.Error != "" {
			sb.WriteString(fmt.Sprintf("  ERROR: %s\n", step.Error))
		} else if step.Output != "" {
			out := step.Output
			if len(out) > 300 {
				out = out[:300] + "…"
			}
			sb.WriteString(fmt.Sprintf("  Output: %s\n", out))
		}
		if step.ExperienceID != "" {
			sb.WriteString(fmt.Sprintf("  Applied Experience: %s\n", step.ExperienceID))
		}
	}
	return sb.String()
}

// extractLastJSON scans s from right to left and returns the last balanced
// JSON object found (everything between the last matching '{' and '}').
// Returns an empty string if no balanced JSON object is found.
func extractLastJSON(s string) string {
	end := strings.LastIndex(s, "}")
	if end == -1 {
		return ""
	}
	depth := 0
	for i := end; i >= 0; i-- {
		switch s[i] {
		case '}':
			depth++
		case '{':
			depth--
			if depth == 0 {
				return s[i : end+1]
			}
		}
	}
	return ""
}

// parseRefinementOps parses a multi-line LLM response looking for JSON
// refinement operation objects on individual lines.
// Lines that do not parse as valid refinementOp JSON are silently skipped.
func parseRefinementOps(text string) []refinementOp {
	var ops []refinementOp
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var op refinementOp
		if err := json.Unmarshal([]byte(line), &op); err == nil && op.Op != "" {
			ops = append(ops, op)
		}
	}
	return ops
}
