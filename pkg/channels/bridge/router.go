package bridge

import (
	"context"
	"sync"

	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/persist"
)

// SessionRouter maps incoming channel users to per-canonical-identity
// ConversationHistory instances, providing cross-channel session continuity.
type SessionRouter struct {
	registry  *Registry
	histories sync.Map // canonical_id (string) → *ai.ConversationHistory
	store     *persist.Store
}

// NewSessionRouter creates a SessionRouter.
func NewSessionRouter(registry *Registry, store *persist.Store) *SessionRouter {
	return &SessionRouter{registry: registry, store: store}
}

// GetHistory returns the ConversationHistory for the given platform user,
// creating and optionally seeding it from the persist store if first access.
func (sr *SessionRouter) GetHistory(platform, platformUserID string) *ai.ConversationHistory {
	canonicalID, err := sr.registry.GetOrCreate(platform, platformUserID, "")
	if err != nil {
		// Fallback: return a fresh unshared history so the user still gets a response.
		return ai.NewConversationHistory()
	}

	actual, loaded := sr.histories.LoadOrStore(canonicalID, ai.NewConversationHistory())
	h := actual.(*ai.ConversationHistory)

	if !loaded && sr.store != nil {
		// First access for this canonical ID: try to restore compressed context.
		// We use a background context because this is a sync path on the first message.
		if summary, ok, err := sr.store.GetLatestContext(context.Background()); err == nil && ok {
			h.AddSystemMessage("## Restored Context\n" + summary)
		}
	}
	return h
}

// ClearHistory removes the in-memory history for a user (does not touch persist store).
func (sr *SessionRouter) ClearHistory(platform, platformUserID string) {
	if canonicalID, err := sr.registry.GetOrCreate(platform, platformUserID, ""); err == nil {
		sr.histories.Delete(canonicalID)
	}
}
