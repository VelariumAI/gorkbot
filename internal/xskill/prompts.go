package xskill

// prompts.go — All XSKILL LLM prompt templates.
//
// Every function in this file returns (systemPrompt, userPrompt) string pairs
// that map cleanly to the two-role Generate(system, user) signature on
// LLMProvider.  The system prompt carries the standing instructions and the
// user prompt carries the variable data — this separation must NEVER collapse
// into a single concatenated string.
//
// Prompt source: XSKILL specification (Poehnelt 2026, §3–§4).

import "fmt"

// ══════════════════════════════════════════════════════════════════════════════
// Phase 1 — Accumulation Loop prompts
// ══════════════════════════════════════════════════════════════════════════════

// promptRolloutSummary builds the prompt for generating a step-by-step
// narrative summary of a completed task trajectory (Phase 1, Step 1).
//
// trajectoryText is the serialised trajectory (JSON or human-readable).
func promptRolloutSummary(trajectoryText string) (system, user string) {
	system = `You are a World-Class reasoning analysis expert. A multimodal agent system uses tool-based visual reasoning to solve the given problem. The agent may have been provided with some experiences. Please summarize the following trajectory (also called rollout) step-by-step:

Summarization Guidelines:
* For each turn or step: Describe which tool was used and with what parameters, explain the reasoning for this specific action, and note which experience (if any) was applied and how. If this turn was part of meta-reasoning skills: identify the meta-reasoning type and explain how its outcome influenced subsequent steps or the final result.
* Given the trajectory and the correct answer, identify and explain any steps that represent detours, errors, backtracking, or any other failure patterns, highlighting why they might have occurred and what their impact was on the trajectory's progress. Discuss how the agent's tool-using knowledge and meta-reasoning skills handled or mitigated these issues.
* Maintain all the core outcomes of each turn or step, even if it was part of a flawed process.
* Thinking with images actions: Document any image preprocessing operations (cropping, resizing, annotation overlays). You need to analyze and note their impact.

Provide a clear, structured summary of the trajectory.`

	user = fmt.Sprintf("<trajectory>%s</trajectory>\n\nProvide a clear, structured summary of the trajectory.", trajectoryText)
	return
}

// promptCrossRolloutCritique builds the prompt for extracting generalised
// experiences from one or more rollout summaries (Phase 1, Step 2).
//
// question, summaries, experiencesUsed, and groundTruth are all variable
// data injected into the user role.
func promptCrossRolloutCritique(question, summaries, experiencesUsed, groundTruth string) (system, user string) {
	system = `You are a reasoning analysis expert. Review these problem-solving attempts and extract practical lessons that could help future attempts.

Analysis Framework:
* Trajectory Review: What worked? What didn't work? How were different tools combined? For visual operations: What preprocessing helped?
* Experience Extraction: Create experiences in two categories: Execution Tips and Decision Rules. You have two options: [add, modify]
* Experience Quality: Keep each experience under 64 words. Start with the situation or condition when the advice applies. Make it general enough to apply to similar problems.

Provide detailed reasoning following the above framework, then conclude with EXACTLY ONE of these JSON objects on its own line:

{"option": "add", "experience": "the new generalizable experience"}

or

{"option": "modify", "experience": "the modified experience", "modified_from": "E17"}

Output the JSON object as the very last line of your response.`

	user = fmt.Sprintf(
		"<question>%s</question>\n<summaries>%s</summaries>\n<experiences_used>%s</experiences_used>\n<ground_truth>%s</ground_truth>",
		question, summaries, experiencesUsed, groundTruth,
	)
	return
}

// promptMergeExperiences builds the prompt for merging two near-duplicate
// experiences into a single, comprehensive experience (Phase 1, Step 3a).
//
// Called when CosineSimilarity64(newVec, existingVec) >= SimilarityMergeThreshold.
func promptMergeExperiences(experienceA, experienceB string) (system, user string) {
	system = `You are an experience library management expert. Merge the following experiences into a single, comprehensive experience.

Requirements:
1. Contain all important information points from both experiences.
2. Be clear, generalizable, and no more than 64 words.
3. Maintain core lessons and decision points.
4. Avoid redundancy.

Output ONLY the merged experience text — no explanation, no JSON wrapper, no preamble.`

	user = fmt.Sprintf("Experience A:\n%s\n\nExperience B:\n%s", experienceA, experienceB)
	return
}

// promptRefineLibrary builds the global library pruning prompt.
//
// Called when len(bank.Experiences) > MaxExperienceLibSize (120 items).
// expCount is the current size; experiencesJSON is the full JSON-encoded
// experience list for the LLM to analyse.
func promptRefineLibrary(expCount int, experiencesJSON string) (system, user string) {
	system = fmt.Sprintf(`You are an experience library curator. The library has grown through multiple batches and may contain redundancy. Perform a global refinement pass.

Current Library Size: %d experiences.
Target: Reduce to 80–100 high-quality, diverse experiences.

Refinement Goals:
1. Merge Truly Redundant: combine near-duplicate experiences into one.
2. Generalize Over-Specific: broaden single-instance lessons into transferable principles.
3. Delete Low-Value: remove noise, trivially obvious entries, or unhelpful entries.

For each change, output one of the following JSON objects, one per line:

{"op":"merge","ids":["E1","E2"],"result":"merged text under 64 words starting with trigger condition"}
{"op":"delete","id":"E3","reason":"brief reason"}

Quality Standard: every retained experience must be under 64 words and start with the trigger condition.
Output ONLY the JSON operation lines — no explanation before or after.`, expCount)

	user = fmt.Sprintf("Current library (JSON):\n%s", experiencesJSON)
	return
}

// ── Skill document prompts ────────────────────────────────────────────────────

// promptGenerateSkill builds the prompt for extracting a raw skill Standard
// Operating Procedure (SOP) from a single trajectory (Phase 1, Step 5).
func promptGenerateSkill(trajectoryText string) (system, user string) {
	system = `You are a skilled AI agent architect. Analyze the trajectory and extract a reusable Standard Operating Procedure (SOP).

Guiding Principles:
1. From successful patterns: capture the effective tool sequences and reasoning chains.
   From failed attempts: document what to avoid and how to recover.
2. Keep It General: Use placeholders like [TARGET], [QUERY], [FILE_PATH], [SEARCH_TERM] instead of specific values.
3. Capture Executable Knowledge: include concrete tool call patterns and decision checkpoints.
4. Brevity Matters: Aim for approximately 600 words.

Output Structure (use this exact markdown hierarchy):
# [Skill Name]

## Description

## Version
v1.0

## Strategy Overview

## Workflow

## Tool Templates

## Watch Out For`

	user = fmt.Sprintf("Trajectory to analyze:\n\n%s", trajectoryText)
	return
}

// promptMergeSkill builds the prompt for merging a newly extracted raw skill
// into an existing global skill document for a given task class (Phase 1, Step 6).
//
// globalSkill is the current contents of the domain skill file.
// newSkill is the freshly extracted SOP from the latest trajectory.
func promptMergeSkill(globalSkill, newSkill string) (system, user string) {
	system = `You are a knowledge architect. Your job is to maintain a single, unified skill document that grows wiser with each new case.

Think of the global skill as a living document — apply these rules to every section:
- Is this part better in the new skill? → Rewrite the global with the better version.
- Is this part redundant with the global? → Delete it from the new skill (keep global).
- Is this part complementary (adds nuance)? → Merge both into a unified section.
- Is this part genuinely different (new variant)? → Add as a labelled variant in the global.

Target length: approximately 1000 words.
Output the COMPLETE updated skill document in markdown — nothing else.`

	user = fmt.Sprintf("## Current Global Skill:\n\n%s\n\n---\n\n## New Skill to Integrate:\n\n%s", globalSkill, newSkill)
	return
}

// promptRefineSkill builds the prompt for trimming an overgrown skill document
// that exceeds MaxSkillWords (Phase 1, Step 6b).
//
// Called only when wordCount(skillContent) > MaxSkillWords.
func promptRefineSkill(skillContent string) (system, user string) {
	system = `You are a skill document architect. Refine this SKILL.md to remove redundancy, generalize overly specific cases, and improve structural clarity.

Goals:
- Reduce total length to under 1000 words while preserving every unique insight.
- Merge near-identical subsections.
- Replace task-specific examples with generic, placeholder-based templates.
- Improve section headings and flow for rapid scanning.

Output ONLY the complete refined skill document in markdown — no explanation, no commentary.`

	user = fmt.Sprintf("Skill document to refine:\n\n%s", skillContent)
	return
}

// ══════════════════════════════════════════════════════════════════════════════
// Phase 2 — Inference Loop prompts
// ══════════════════════════════════════════════════════════════════════════════

// promptDecomposeTask builds the task decomposition prompt for Phase 2, Step 1.
//
// The LLM response must be a valid JSON object:
//
//	{"subtasks": [{"type": "ToolUtilization", "query": "..."}, ...]}
func promptDecomposeTask(taskDescription string) (system, user string) {
	system = `You are an Expert Visual Reasoning Strategist. Your objective is to deconstruct a complex visual task into 2–3 distinct, actionable subtasks to retrieve the most relevant methodological guidance from the experience library.

For each subtask, output a JSON object with exactly two keys:
- "type": one of "ToolUtilization", "ReasoningStrategy", or "ChallengeMitigation"
- "query": an abstracted retrieval query targeting that aspect

CRITICAL: The query MUST abstract away from specific literals.
  Correct:   "multi-class object detection with overlapping bounding boxes"
  Incorrect: "find the red car next to building B in frame_042.jpg"

Output a single valid JSON object only:
{"subtasks": [{"type": "...", "query": "..."}, {"type": "...", "query": "..."}]}`

	user = fmt.Sprintf("Task: %s", taskDescription)
	return
}

// promptRewriteExperiences builds the experience rewriting prompt for Phase 2, Step 3.
//
// experiencesText is a formatted list of the retrieved experiences.
// The LLM response must be a valid JSON object mapping experience IDs to
// rewritten guidance strings.
func promptRewriteExperiences(taskDescription, experiencesText string) (system, user string) {
	system = `You are an expert AI mentor adapting retrieved methodological experiences to strictly fit a specific visual reasoning task.

Guidelines:
1. Operational Focus: translate abstract principles into concrete, actionable steps for this exact task.
2. Pitfalls & Best Practices: highlight the specific risks and recommended patterns for this task's domain.
3. Contextual Adaptation: reframe each experience in the vocabulary and constraints of the specific task.
4. Tone: direct and instructional — address the agent as "you".

If a retrieved experience is entirely irrelevant to this task, omit it from the output (do not include its ID).

Output ONLY a valid JSON object mapping experience IDs to rewritten guidance text:
{"E1": "rewritten guidance for this task...", "E3": "..."}`

	user = fmt.Sprintf("Task: %s\n\nRetrieved experiences:\n%s", taskDescription, experiencesText)
	return
}

// promptAdaptSkill builds the skill adaptation prompt for Phase 2, Step 4.
//
// globalSkillContent is the current domain skill document.
// experiencesText contains the already-rewritten experience guidance.
func promptAdaptSkill(taskDescription, globalSkillContent, experiencesText string) (system, user string) {
	system = `You are an expert agent assistant. Tailor the general skill document to fit this specific task.

Your Goals:
1. Select What's Relevant: keep only the workflow steps and decision rules that apply to this specific task — prune what is irrelevant.
2. Integrate Experiences: weave the retrieved experience guidance into the relevant workflow steps as inline tips or preconditions.
3. Keep Templates Ready: adjust generic placeholders ([TARGET], [QUERY], etc.) to be more descriptive of this task's domain while keeping them reusable.
4. Stay Lean: approximately 400 words maximum.

CRITICAL: Output a reusable methodology guide, NOT a pre-filled answer.
Do NOT solve the task — guide the agent on HOW to approach it.
Output ONLY the adapted skill content in markdown format, starting with a # heading.`

	user = fmt.Sprintf(
		"Task: %s\n\n## Global Skill Document:\n\n%s\n\n## Retrieved & Rewritten Experiences:\n\n%s",
		taskDescription, globalSkillContent, experiencesText,
	)
	return
}
