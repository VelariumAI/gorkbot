package subagents

import (
	"context"
	"fmt"
	"sync/atomic"
)

// budgetKeyType is the context key type for SessionBudget.
type budgetKeyType struct{}

// BudgetKey is the key used to store/retrieve a SessionBudget in context.
var BudgetKey = budgetKeyType{}

// SessionBudget tracks total turns consumed across a parent+subagent chain.
// All operations are lock-free via atomics.
type SessionBudget struct {
	total    int64
	used     atomic.Int64
	refunded atomic.Int64
}

// NewSessionBudget creates a budget with the given total turn limit.
func NewSessionBudget(total int) *SessionBudget {
	return &SessionBudget{total: int64(total)}
}

// Consume attempts to consume n turns. Returns false if budget is exceeded.
// The consumption is not rolled back on false — caller should stop immediately.
func (b *SessionBudget) Consume(n int) bool {
	newUsed := b.used.Add(int64(n))
	net := newUsed - b.refunded.Load()
	return net <= b.total
}

// Refund returns n turns to the budget (e.g., when a subagent finishes early).
func (b *SessionBudget) Refund(n int) {
	b.refunded.Add(int64(n))
}

// Remaining returns the number of turns still available.
func (b *SessionBudget) Remaining() int {
	net := b.used.Load() - b.refunded.Load()
	remaining := b.total - net
	if remaining < 0 {
		return 0
	}
	return int(remaining)
}

// Report returns a human-readable budget status string.
func (b *SessionBudget) Report() string {
	net := b.used.Load() - b.refunded.Load()
	return fmt.Sprintf("%d/%d turns used", net, b.total)
}

// WithBudget stores a SessionBudget in context.
func WithBudget(ctx context.Context, budget *SessionBudget) context.Context {
	return context.WithValue(ctx, BudgetKey, budget)
}

// BudgetFromContext retrieves the SessionBudget from context, or nil if not set.
func BudgetFromContext(ctx context.Context) *SessionBudget {
	if b, ok := ctx.Value(BudgetKey).(*SessionBudget); ok {
		return b
	}
	return nil
}
