package execution

import (
	"context"
	"strings"
	"time"
)

// ExecutionBudget defines time and iteration limits for a turn.
type ExecutionBudget struct {
	TurnTimeout            time.Duration
	ToolDecisionTimeout    time.Duration
	ToolExecutionTimeout   time.Duration
	VCSEFastTimeout        time.Duration
	VCSEReasonTimeout      time.Duration
	MaxToolCalls           int
	MaxRepeatedToolCalls   int
	MaxConsecutiveFailures int
	MaxIdleDuration        time.Duration
	MaxPlanRevisions       int
	ToolTimeouts           map[string]ToolBudget
}

// ToolBudget defines per-tool execution limits.
type ToolBudget struct {
	DecisionTimeout  time.Duration
	ExecutionTimeout time.Duration
	MaxOutputChars   int
	MaxErrorChars    int
}

// DefaultBudget returns sane defaults for turn and per-tool execution.
func DefaultBudget() ExecutionBudget {
	b := ExecutionBudget{
		TurnTimeout:            60 * time.Second,
		ToolDecisionTimeout:    750 * time.Millisecond,
		ToolExecutionTimeout:   15 * time.Second,
		VCSEFastTimeout:        250 * time.Millisecond,
		VCSEReasonTimeout:      5 * time.Second,
		MaxToolCalls:           12,
		MaxRepeatedToolCalls:   2,
		MaxConsecutiveFailures: 3,
		MaxIdleDuration:        3 * time.Second,
		MaxPlanRevisions:       2,
		ToolTimeouts:           map[string]ToolBudget{},
	}

	common := func(decision, execution time.Duration) ToolBudget {
		return ToolBudget{
			DecisionTimeout:  decision,
			ExecutionTimeout: execution,
			MaxOutputChars:   12000,
			MaxErrorChars:    4000,
		}
	}

	b.ToolTimeouts["read_file"] = common(50*time.Millisecond, 1*time.Second)
	b.ToolTimeouts["list_directory"] = common(50*time.Millisecond, 1*time.Second)
	b.ToolTimeouts["write_file"] = common(250*time.Millisecond, 2*time.Second)
	b.ToolTimeouts["delete_file"] = common(500*time.Millisecond, 2*time.Second)
	b.ToolTimeouts["bash"] = common(500*time.Millisecond, 10*time.Second)
	b.ToolTimeouts["structured_bash"] = common(500*time.Millisecond, 10*time.Second)
	b.ToolTimeouts["web_fetch"] = common(250*time.Millisecond, 8*time.Second)
	b.ToolTimeouts["http_request"] = common(250*time.Millisecond, 8*time.Second)
	b.ToolTimeouts["download_file"] = ToolBudget{
		DecisionTimeout:  500 * time.Millisecond,
		ExecutionTimeout: 15 * time.Second,
		MaxOutputChars:   8000,
		MaxErrorChars:    4000,
	}
	b.ToolTimeouts["git_commit"] = common(500*time.Millisecond, 10*time.Second)
	b.ToolTimeouts["git_push"] = common(750*time.Millisecond, 20*time.Second)
	b.ToolTimeouts["create_tool"] = common(750*time.Millisecond, 20*time.Second)
	b.ToolTimeouts["spawn_agent"] = common(750*time.Millisecond, 15*time.Second)

	return b
}

// BudgetForTool resolves per-tool overrides, falling back to global defaults.
func (b ExecutionBudget) BudgetForTool(toolName string) ToolBudget {
	key := strings.TrimSpace(strings.ToLower(toolName))
	if tb, ok := b.ToolTimeouts[key]; ok {
		return tb
	}
	return ToolBudget{
		DecisionTimeout:  b.ToolDecisionTimeout,
		ExecutionTimeout: b.ToolExecutionTimeout,
		MaxOutputChars:   12000,
		MaxErrorChars:    4000,
	}
}

// WithTurnTimeout adds turn timeout to context.
func (b ExecutionBudget) WithTurnTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if b.TurnTimeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, b.TurnTimeout)
}

// WithToolDecisionTimeout adds decision timeout for a tool.
func (b ExecutionBudget) WithToolDecisionTimeout(ctx context.Context, toolName string) (context.Context, context.CancelFunc) {
	timeout := b.BudgetForTool(toolName).DecisionTimeout
	if timeout <= 0 {
		timeout = b.ToolDecisionTimeout
	}
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

// WithToolExecutionTimeout adds execution timeout for a tool.
func (b ExecutionBudget) WithToolExecutionTimeout(ctx context.Context, toolName string) (context.Context, context.CancelFunc) {
	timeout := b.BudgetForTool(toolName).ExecutionTimeout
	if timeout <= 0 {
		timeout = b.ToolExecutionTimeout
	}
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}
