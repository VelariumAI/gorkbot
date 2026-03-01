// Package pipeline provides a sequential/DAG pipeline execution engine for Gorkbot subagents.
package pipeline

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Step defines a single pipeline step.
type Step struct {
	Name      string        `json:"name"`
	AgentType string        `json:"agent_type"`
	Task      string        `json:"task"`       // may reference {{outputs.stepname}} template vars
	DependsOn []string      `json:"depends_on"` // names of steps that must complete first
	Timeout   time.Duration `json:"timeout_seconds"`
}

// Pipeline is an ordered set of steps with a shared name.
type Pipeline struct {
	Name  string `json:"name"`
	Steps []Step `json:"steps"`
}

// StepResult holds the output of a completed step.
type StepResult struct {
	Name     string
	Output   string
	Error    error
	Duration time.Duration
}

// AgentRunner is the function signature that spawns an agent and awaits its result.
// The caller (orchestrator) provides this so pipeline has no engine import cycle.
type AgentRunner func(ctx context.Context, agentType, task string) (string, error)

// Engine executes Pipelines using the provided AgentRunner.
type Engine struct {
	runner AgentRunner
}

// NewEngine creates a new PipelineEngine with the given agent runner.
func NewEngine(runner AgentRunner) *Engine {
	return &Engine{runner: runner}
}

// Execute runs all pipeline steps respecting declared dependencies.
// Returns a map of step name → output string, and the first fatal error if any.
func (e *Engine) Execute(ctx context.Context, p Pipeline) (map[string]string, error) {
	if len(p.Steps) == 0 {
		return nil, fmt.Errorf("pipeline %q has no steps", p.Name)
	}

	// Build name→step index for validation.
	stepIndex := make(map[string]int, len(p.Steps))
	for i, s := range p.Steps {
		if _, dup := stepIndex[s.Name]; dup {
			return nil, fmt.Errorf("duplicate step name %q in pipeline %q", s.Name, p.Name)
		}
		stepIndex[s.Name] = i
	}

	// Validate dependency references.
	for _, s := range p.Steps {
		for _, dep := range s.DependsOn {
			if _, ok := stepIndex[dep]; !ok {
				return nil, fmt.Errorf("step %q depends on unknown step %q", s.Name, dep)
			}
		}
	}

	// Topological sort (Kahn's algorithm).
	sorted, err := topoSort(p.Steps, stepIndex)
	if err != nil {
		return nil, fmt.Errorf("pipeline %q dependency cycle detected: %w", p.Name, err)
	}

	outputs := make(map[string]string, len(sorted))
	var mu sync.Mutex

	for _, step := range sorted {
		select {
		case <-ctx.Done():
			return outputs, ctx.Err()
		default:
		}

		// Substitute template vars from prior step outputs.
		task := renderTemplate(step.Task, outputs)

		// Apply per-step timeout if set.
		stepCtx := ctx
		var cancel context.CancelFunc
		if step.Timeout > 0 {
			stepCtx, cancel = context.WithTimeout(ctx, step.Timeout)
		}

		start := time.Now()
		result, runErr := e.runner(stepCtx, step.AgentType, task)
		_ = time.Since(start)

		if cancel != nil {
			cancel()
		}

		if runErr != nil {
			// Non-fatal: store error as output so subsequent steps can branch on it.
			mu.Lock()
			outputs[step.Name] = fmt.Sprintf("ERROR: %v", runErr)
			mu.Unlock()
			// Fatal: abort pipeline.
			return outputs, fmt.Errorf("step %q failed: %w", step.Name, runErr)
		}

		mu.Lock()
		outputs[step.Name] = result
		mu.Unlock()
	}

	return outputs, nil
}

// FormatResults returns a human-readable summary of pipeline outputs.
func FormatResults(pipelineName string, outputs map[string]string, steps []Step) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Pipeline: %s\n\n", pipelineName))
	for _, step := range steps {
		out, ok := outputs[step.Name]
		if !ok {
			continue
		}
		sb.WriteString(fmt.Sprintf("### Step: %s (agent: %s)\n", step.Name, step.AgentType))
		// Truncate very long outputs for display.
		if len(out) > 800 {
			out = out[:800] + "\n... [truncated]"
		}
		sb.WriteString(out)
		sb.WriteString("\n\n")
	}
	return sb.String()
}

// renderTemplate replaces {{outputs.stepname}} placeholders with actual outputs.
func renderTemplate(task string, outputs map[string]string) string {
	result := task
	for name, val := range outputs {
		placeholder := "{{outputs." + name + "}}"
		result = strings.ReplaceAll(result, placeholder, val)
	}
	return result
}

// topoSort performs Kahn's topological sort on steps.
func topoSort(steps []Step, index map[string]int) ([]Step, error) {
	// Build in-degree map.
	inDegree := make(map[string]int, len(steps))
	dependents := make(map[string][]string, len(steps)) // dep → steps that depend on dep

	for _, s := range steps {
		if _, ok := inDegree[s.Name]; !ok {
			inDegree[s.Name] = 0
		}
		for _, dep := range s.DependsOn {
			inDegree[s.Name]++
			dependents[dep] = append(dependents[dep], s.Name)
		}
	}

	// Queue all zero-in-degree steps.
	var queue []string
	for _, s := range steps {
		if inDegree[s.Name] == 0 {
			queue = append(queue, s.Name)
		}
	}

	var sorted []Step
	for len(queue) > 0 {
		// Take first (stable ordering by original position).
		name := queue[0]
		queue = queue[1:]
		sorted = append(sorted, steps[index[name]])

		for _, dependent := range dependents[name] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	if len(sorted) != len(steps) {
		return nil, fmt.Errorf("cycle detected among %d unresolvable steps", len(steps)-len(sorted))
	}
	return sorted, nil
}
