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

	// Topological sort (Kahn's algorithm) to detect cycles.
	_, err := topoSort(p.Steps, stepIndex)
	if err != nil {
		return nil, fmt.Errorf("pipeline %q dependency cycle detected: %w", p.Name, err)
	}

	outputs := make(map[string]string, len(p.Steps))
	var mu sync.Mutex

	// Create a done channel for each step
	dones := make(map[string]chan struct{}, len(p.Steps))
	for _, s := range p.Steps {
		dones[s.Name] = make(chan struct{})
	}

	// Context for cancellation on fatal error
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	errCh := make(chan error, len(p.Steps))

	for _, step := range p.Steps {
		wg.Add(1)
		go func(s Step) {
			defer wg.Done()

			// Wait for dependencies
			for _, dep := range s.DependsOn {
				select {
				case <-dones[dep]:
					// Dependency finished, continue
				case <-ctx.Done():
					return // Pipeline aborted
				}
			}

			// Check context before executing
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Prepare outputs for template rendering
			mu.Lock()
			allOutputs := make(map[string]string, len(outputs))
			for k, v := range outputs {
				allOutputs[k] = v
			}
			mu.Unlock()

			// Substitute template vars from prior step outputs.
			task := renderTemplate(s.Task, allOutputs)

			// Apply per-step timeout if set.
			stepCtx := ctx
			var stepCancel context.CancelFunc
			if s.Timeout > 0 {
				stepCtx, stepCancel = context.WithTimeout(ctx, s.Timeout)
			}

			result, runErr := e.runner(stepCtx, s.AgentType, task)

			if stepCancel != nil {
				stepCancel()
			}

			if runErr != nil {
				// Non-fatal: store error as output so subsequent steps can branch on it.
				// However, if we decide to abort, we write to errCh and cancel.
				// To preserve original semantics, if e.runner returns err, it's fatal.
				mu.Lock()
				outputs[s.Name] = fmt.Sprintf("ERROR: %v", runErr)
				mu.Unlock()

				errCh <- fmt.Errorf("step %q failed: %w", s.Name, runErr)
				cancel() // Abort other steps
			} else {
				mu.Lock()
				outputs[s.Name] = result
				mu.Unlock()
			}

			close(dones[s.Name])
		}(step)
	}

	wg.Wait()
	close(errCh)

	if err := <-errCh; err != nil {
		return outputs, err
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
		if strings.Contains(result, placeholder) {
			result = strings.ReplaceAll(result, placeholder, val)
		}
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
