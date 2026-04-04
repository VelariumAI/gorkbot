// Package xskill implements the XSKILL continual learning framework for Gorkbot.
//
// XSKILL provides a dual-stream memory system that allows the agent to
// continuously improve its tool-use efficiency and problem-solving flexibility:
//
//   - Skill Library   : task-level Standard Operating Procedures stored as
//     domain-partitioned Markdown documents (e.g. visual-logic.md,
//     search-tactics.md).  One file per task class — never a
//     monolithic global blob.
//   - Experience Bank : action-level tactical insights stored as a JSON file with
//     cosine-similarity-indexed float64 vectors for retrieval.
//
// Architecture:
//
//	KnowledgeBase   — Phase 1 (Accumulation): background learning from trajectories
//	InferenceEngine — Phase 2 (Solving):      foreground context enrichment before LLM calls
//
// Thread safety: KnowledgeBase uses sync.RWMutex for all shared state, and an
// atomic.Int64 for race-free sequential experience ID generation.
//
// Platform: pure Go, zero CGO.  Works on Windows / Linux / macOS / ARM (Android/Termux).
package xskill

import "time"

// ──────────────────────────────────────────────────────────────────────────────
// Tuneable constants
// ──────────────────────────────────────────────────────────────────────────────

// ExperienceBankVersion is the schema version written to every experiences.json.
const ExperienceBankVersion = "1.0.0"

// MaxExperienceWords is the maximum word count allowed for a single experience.
// Experiences exceeding this limit are truncated during insertion.
const MaxExperienceWords = 64

// MaxExperienceLibSize is the total number of experiences that triggers a global
// pruning pass.  When len(bank.Experiences) > MaxExperienceLibSize, KnowledgeBase
// calls pruneLibrary() in the same Accumulate goroutine before continuing.
const MaxExperienceLibSize = 120

// PruneTargetSize is the desired library size after a global pruning pass.
// Sits in the middle of the spec's "80–100 high-quality experiences" band.
const PruneTargetSize = 90

// SimilarityMergeThreshold is the cosine-similarity threshold above which two
// experiences are considered near-duplicates and should be merged.
const SimilarityMergeThreshold = 0.70

// MaxSkillWords is the word count above which a skill document is submitted to
// the LLM for a refinement pass.  Keeps individual skill files focused.
const MaxSkillWords = 1000

// TopKRetrieval is the number of experiences retrieved per sub-task during Phase 2.
const TopKRetrieval = 3

// ──────────────────────────────────────────────────────────────────────────────
// Experience Bank types
// ──────────────────────────────────────────────────────────────────────────────

// Experience is a single action-level tactical insight stored in the Experience
// Bank.  It encodes a generalised condition → action pair extracted from
// completed task trajectories.
//
// IDs follow the pattern "E1", "E2", … "E120" and are stable across sessions.
// An experience referenced by a Trajectory.ExperiencesUsed field retains its ID
// even after a merge/modify operation (the lower-numbered ID is always kept).
type Experience struct {
	// ID is the stable unique identifier, e.g. "E7".
	ID string `json:"id"`

	// Condition describes the trigger situation — when this guidance applies.
	// Always starts with a situation clause ("When X …", "If the task requires …").
	Condition string `json:"condition"`

	// Action describes what to do once the condition is met.
	Action string `json:"action"`

	// Vector is the float64 embedding of Condition+" "+Action used for
	// cosine-similarity retrieval.  May be nil for manually inserted entries
	// (those are skipped during retrieval).
	Vector []float64 `json:"vector,omitempty"`

	// CreatedAt is the UTC timestamp when this experience was first added.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is the UTC timestamp of the most recent modification.
	UpdatedAt time.Time `json:"updated_at"`
}

// ExperienceBank is the serialised form of the entire experience library written
// to ~/.gorkbot/xskill_kb/experiences.json.
type ExperienceBank struct {
	// Version is the schema version (ExperienceBankVersion).
	Version string `json:"version"`

	// Experiences is the ordered, stable list of experience entries.
	Experiences []Experience `json:"experiences"`

	// UpdatedAt records the last time any experience was added, modified,
	// or deleted.
	UpdatedAt time.Time `json:"updated_at"`
}

// ──────────────────────────────────────────────────────────────────────────────
// Trajectory types (Phase 1 input)
// ──────────────────────────────────────────────────────────────────────────────

// TrajectoryStep records a single tool invocation within a task execution.
type TrajectoryStep struct {
	// StepIndex is the 0-based position in the trajectory sequence.
	StepIndex int `json:"step_index"`

	// ToolName is the normalised name of the tool invoked (e.g. "bash", "read_file").
	ToolName string `json:"tool_name"`

	// Parameters is the JSON-encoded parameter map passed to the tool.
	// May be empty for tools called with no arguments.
	Parameters string `json:"parameters,omitempty"`

	// Output is the (possibly truncated) tool result string.
	Output string `json:"output,omitempty"`

	// Error holds the error message if the tool call failed.
	Error string `json:"error,omitempty"`

	// DurationMS is the wall-clock execution time in milliseconds.
	DurationMS int64 `json:"duration_ms,omitempty"`

	// Reasoning is an optional annotation describing why this tool was chosen
	// at this step.  Populated by the orchestrator when available.
	Reasoning string `json:"reasoning,omitempty"`

	// ExperienceID names the experience (e.g. "E3") that guided this step,
	// if the agent was operating with injected XSKILL context.
	ExperienceID string `json:"experience_id,omitempty"`
}

// Trajectory is the full execution record of a single completed task.
// It is the primary input to the Phase 1 (Accumulation Loop) via
// KnowledgeBase.Accumulate.
//
// The caller is responsible for populating Steps, Question, and the timing
// fields.  GroundTruth and ExperiencesUsed are optional but improve the
// quality of the cross-rollout critique.
type Trajectory struct {
	// TaskID is a unique identifier for this task execution (e.g. a UUID or
	// session-scoped counter string).
	TaskID string `json:"task_id"`

	// Question is the original user request or problem statement.
	Question string `json:"question"`

	// GroundTruth is the correct answer, if known.  Leave empty when unknown.
	GroundTruth string `json:"ground_truth,omitempty"`

	// ExperiencesUsed lists the IDs of experiences injected into the prompt
	// before this execution (Phase 2 output).  Used in the cross-rollout
	// critique to evaluate how well retrieved experiences helped.
	ExperiencesUsed []string `json:"experiences_used,omitempty"`

	// Steps is the ordered list of tool-call records.
	Steps []TrajectoryStep `json:"steps"`

	// Summary is optionally pre-filled with the rollout summary text.
	// If empty, KnowledgeBase.Accumulate will generate it via the LLM.
	Summary string `json:"summary,omitempty"`

	// StartedAt is the UTC timestamp when execution began.
	StartedAt time.Time `json:"started_at"`

	// CompletedAt is the UTC timestamp when execution finished.
	CompletedAt time.Time `json:"completed_at"`
}

// ──────────────────────────────────────────────────────────────────────────────
// Phase 2 inference types
// ──────────────────────────────────────────────────────────────────────────────

// SubTask is a decomposed retrieval query produced by InferenceEngine during
// Phase 2 task decomposition.  Each SubTask targets exactly one retrieval
// dimension in the Experience Bank.
type SubTask struct {
	// Type classifies the retrieval aspect.  One of:
	//   "ToolUtilization"   — how to select and configure the right tools
	//   "ReasoningStrategy" — how to sequence reasoning steps
	//   "ChallengeMitigation" — how to avoid or recover from known pitfalls
	Type string `json:"type"`

	// Query is an abstracted retrieval query that avoids task-specific literals
	// (e.g. "multi-step object detection" not "find the red car in img_001.jpg").
	Query string `json:"query"`
}

// TaskDecomposition is the structured output of the Phase 2 LLM decomposition
// step.  It contains 2–3 SubTasks for independent experience retrieval.
type TaskDecomposition struct {
	SubTasks []SubTask `json:"subtasks"`
}

// ExperienceRewrite maps experience IDs (e.g. "E5") to their rewritten,
// task-adapted guidance text produced during Phase 2 experience rewriting.
type ExperienceRewrite map[string]string

// RawCritique is the parsed output of the cross-rollout critique LLM call.
// The LLM either proposes adding a brand-new experience ("add") or refining
// an existing one ("modify").
type RawCritique struct {
	// Option is "add" or "modify".
	Option string `json:"option"`

	// Experience is the complete new or updated experience text.
	// Combines both the Condition and Action in a single string; the KB
	// splits them heuristically before storage.
	Experience string `json:"experience"`

	// ModifiedFrom is the ID of the experience to update (e.g. "E17").
	// Only present when Option == "modify".
	ModifiedFrom string `json:"modified_from,omitempty"`
}

// refinementOp is the machine-readable form of a single operation emitted by
// the global library pruning LLM call.  Unexported — only used inside kb.go.
type refinementOp struct {
	// Op is "merge" or "delete".
	Op string `json:"op"`

	// IDs is the list of experience IDs to merge.  Only present when Op == "merge".
	// The first element is the "winner" ID that will be kept.
	IDs []string `json:"ids,omitempty"`

	// Result is the merged experience text.  Only present when Op == "merge".
	Result string `json:"result,omitempty"`

	// ID is the experience to delete.  Only present when Op == "delete".
	ID string `json:"id,omitempty"`

	// Reason is a brief human-readable justification.  Only present when
	// Op == "delete".  Not used programmatically.
	Reason string `json:"reason,omitempty"`
}
