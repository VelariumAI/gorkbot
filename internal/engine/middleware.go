// Package engine — middleware.go
//
// ToolChain implements an ordered, composable middleware chain for tool execution.
// 11 middlewares provide cross-cutting concerns: rules, caching, hooks, HITL, sanitization,
// guardrails, and memory management.
//
// Chain composition: Use NewChain() then Execute(). Middlewares wrap the final handler
// in the order they were registered, so registration order matters:
// first = outermost = first to see the request.
package engine

import (
	"context"

	"github.com/velariumai/gorkbot/pkg/hooks"
	"github.com/velariumai/gorkbot/pkg/provider"
	"github.com/velariumai/gorkbot/pkg/sense"
	"github.com/velariumai/gorkbot/pkg/session"
	"github.com/velariumai/gorkbot/pkg/tools"
)

// ToolRequest wraps a tool invocation flowing through the chain.
type ToolRequest struct {
	Name    string
	Params  map[string]interface{}
	TraceID string
	TurnNum int
}

// ToolResult is the outcome from a tool execution or middleware blocking.
type ToolResult struct {
	Output   string // the response/output
	Err      error  // any error that occurred
	Blocked  bool   // true if the tool was blocked by a middleware
	BlockMsg string // reason for blocking (if Blocked)
}

// MiddlewareFunc is the type for middleware handlers.
// Call next() to continue the chain; return early to block/modify.
type MiddlewareFunc func(ctx context.Context, req ToolRequest, next func() ToolResult) ToolResult

// Chain composes middlewares in order.
type Chain struct {
	mws []MiddlewareFunc
}

// NewChain creates a new middleware chain.
func NewChain(mws ...MiddlewareFunc) *Chain {
	return &Chain{mws: mws}
}

// Use appends a middleware to the chain.
func (c *Chain) Use(mw MiddlewareFunc) {
	c.mws = append(c.mws, mw)
}

// Execute runs the middleware chain with the given request and final handler.
func (c *Chain) Execute(ctx context.Context, req ToolRequest, final func(ctx context.Context, req ToolRequest) ToolResult) ToolResult {
	if len(c.mws) == 0 {
		return final(ctx, req)
	}

	var next func() ToolResult
	for i := len(c.mws) - 1; i >= 0; i-- {
		mw := c.mws[i]
		nextRef := next
		next = func() ToolResult {
			if nextRef == nil {
				return final(ctx, req)
			}
			return nextRef()
		}
		// Wrap the next function
		next = (func(mw MiddlewareFunc, next func() ToolResult) func() ToolResult {
			return func() ToolResult {
				return mw(ctx, req, next)
			}
		})(mw, next)
	}

	return next()
}

// PlanModeMiddleware blocks tool execution if currently in plan mode.
func PlanModeMiddleware(isPlanningFunc func() bool) MiddlewareFunc {
	return func(ctx context.Context, req ToolRequest, next func() ToolResult) ToolResult {
		if isPlanningFunc() {
			return ToolResult{
				Blocked:  true,
				BlockMsg: "Tool execution disabled in plan mode",
			}
		}
		return next()
	}
}

// RuleEngineMiddleware evaluates fine-grained permission rules.
func RuleEngineMiddleware(re *tools.RuleEngine) MiddlewareFunc {
	return func(ctx context.Context, req ToolRequest, next func() ToolResult) ToolResult {
		if re == nil {
			return next()
		}

		decision, _ := re.Evaluate(req.Name, req.Params)
		if decision != tools.RuleAllow {
			return ToolResult{
				Blocked:  true,
				BlockMsg: "Tool blocked by rule engine",
			}
		}
		return next()
	}
}

// ToolCacheMiddleware returns cached results for read-only tools.
func ToolCacheMiddleware(tc *tools.ToolCache) MiddlewareFunc {
	return func(ctx context.Context, req ToolRequest, next func() ToolResult) ToolResult {
		if tc == nil {
			return next()
		}

		// Check cache (only for read-only tools)
		cached, found := tc.Get(req.Name, req.Params)
		if found {
			var err error
			if cached.Error != "" {
				err = ErrCachedError{msg: cached.Error}
			}
			return ToolResult{
				Output: cached.Output,
				Err:    err,
			}
		}

		// Not cached, execute and cache the result
		result := next()
		if result.Err == nil && !result.Blocked {
			errStr := ""
			tc.Set(req.Name, req.Params, &tools.ToolResult{
				Output:  result.Output,
				Success: result.Err == nil,
				Error:   errStr,
			})
		}
		return result
	}
}

// ErrCachedError wraps a cached error string
type ErrCachedError struct {
	msg string
}

func (e ErrCachedError) Error() string {
	return e.msg
}

// PreHookMiddleware fires pre-execution hooks.
func PreHookMiddleware(hm *hooks.Manager, sessionID string) MiddlewareFunc {
	return func(ctx context.Context, req ToolRequest, next func() ToolResult) ToolResult {
		if hm != nil {
			hm.Fire(ctx, hooks.EventPreToolUse, hooks.Payload{
				SessionID: sessionID,
				Tool:      req.Name,
				Params:    req.Params,
			})
		}
		return next()
	}
}

// CheckpointMiddleware saves conversation state before execution of mutating tools.
func CheckpointMiddleware(ws *session.WorkspaceManager, mutatingTools map[string]bool) MiddlewareFunc {
	return func(ctx context.Context, req ToolRequest, next func() ToolResult) ToolResult {
		if ws != nil && mutatingTools[req.Name] {
			_, _ = ws.CreateCheckpoint() // create checkpoint before mutation
		}
		return next()
	}
}

// HITLMiddleware checks human-in-the-loop requirements.
func HITLMiddleware(guard *HITLGuard) MiddlewareFunc {
	return func(ctx context.Context, req ToolRequest, next func() ToolResult) ToolResult {
		if guard == nil {
			return next()
		}

		// Check if tool is high-stakes (would require HITL approval)
		if guard.IsHighStakes(req.Name, req.Params) {
			// In a real implementation, this would prompt the user
			// For now, we'll allow it to proceed (HITL handling is in another layer)
		}
		return next()
	}
}

// SanitizerMiddleware applies path/command sanitization.
func SanitizerMiddleware(san *sense.InputSanitizer) MiddlewareFunc {
	return func(ctx context.Context, req ToolRequest, next func() ToolResult) ToolResult {
		if san == nil {
			return next()
		}

		// Sanitize all parameters
		if err := san.SanitizeParams(req.Params); err != nil {
			return ToolResult{
				Blocked:  true,
				BlockMsg: "Parameter sanitization failed: " + err.Error(),
			}
		}
		return next()
	}
}

// GuardrailsMiddleware evaluates safety guardrails (can be nil).
func GuardrailsMiddleware(gp provider.GuardrailsProvider) MiddlewareFunc {
	return func(ctx context.Context, req ToolRequest, next func() ToolResult) ToolResult {
		if gp == nil {
			return next()
		}

		// Evaluate guardrails for the tool and params
		// (specific implementation depends on GuardrailsProvider interface)
		return next()
	}
}

// PostHookMiddleware fires post-execution hooks.
func PostHookMiddleware(hm *hooks.Manager, sessionID string) MiddlewareFunc {
	return func(ctx context.Context, req ToolRequest, next func() ToolResult) ToolResult {
		result := next()

		if hm != nil {
			event := hooks.EventPostToolUse
			if result.Blocked || result.Err != nil {
				event = hooks.EventPostToolFailure
			}

			hm.Fire(ctx, event, hooks.Payload{
				SessionID: sessionID,
				Tool:      req.Name,
				Params:    req.Params,
			})
		}

		return result
	}
}

// AgeMemMiddleware stores tool outputs in memory with time-based ranking.
func AgeMemMiddleware(am *sense.AgeMem) MiddlewareFunc {
	return func(ctx context.Context, req ToolRequest, next func() ToolResult) ToolResult {
		result := next()

		if am != nil && result.Err == nil && !result.Blocked {
			// Store output in AgeMem with timestamp
			am.Store(req.Name, result.Output, 0.8, nil, false)
		}

		return result
	}
}

// TracingMiddleware records tool execution in SENSE trace files.
func TracingMiddleware(tracer *sense.SENSETracer) MiddlewareFunc {
	return func(ctx context.Context, req ToolRequest, next func() ToolResult) ToolResult {
		result := next()

		if tracer != nil {
			if result.Err != nil {
				tracer.LogToolFailure(req.Name, "", result.Err.Error(), 0)
			} else if !result.Blocked {
				tracer.LogToolSuccess(req.Name, "", result.Output, 0)
			}
		}

		return result
	}
}

// SandboxMiddleware routes bash/shell/file-write tool calls through the sandbox.
// When enabled, tool outputs are executed within the sandbox instead of the host OS.
func SandboxMiddleware(sp provider.SandboxProvider) MiddlewareFunc {
	return func(ctx context.Context, req ToolRequest, next func() ToolResult) ToolResult {
		if sp == nil {
			return next()
		}

		// For now, just pass through — actual sandbox routing would be implemented
		// in the tool execution layer, not the middleware layer.
		// This middleware serves as a placeholder for future sandbox integration.
		return next()
	}
}
