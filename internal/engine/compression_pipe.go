package engine

import (
	"context"
	"log/slog"

	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/persist"
	"github.com/velariumai/gorkbot/pkg/sense"
)

// CompressionPipe triggers context compression when conversation history grows
// beyond a configurable token threshold. It partitions history into a recency
// window (kept verbatim) and an older segment (compressed to a summary), then
// rebuilds the history with the summary as a system message.
type CompressionPipe struct {
	compressor    *sense.Compressor
	store         *persist.Store
	sessionID     string
	logger        *slog.Logger
	ThresholdToks int // fire when history exceeds this many tokens (default 60,000)
	RecencyWindow int // last N messages kept verbatim (default 12)
}

// NewCompressionPipe creates a CompressionPipe. Returns nil when compressor is nil.
func NewCompressionPipe(compressor *sense.Compressor, store *persist.Store, sessionID string, logger *slog.Logger) *CompressionPipe {
	if compressor == nil {
		return nil
	}
	return &CompressionPipe{
		compressor:    compressor,
		store:         store,
		sessionID:     sessionID,
		logger:        logger,
		ThresholdToks: 60_000,
		RecencyWindow: 12,
	}
}

// MaybeCompress compresses history when token count exceeds ThresholdToks.
// It is a no-op when the threshold has not been reached.
func (cp *CompressionPipe) MaybeCompress(ctx context.Context, history *ai.ConversationHistory) error {
	if history.EstimateTokens() < cp.ThresholdToks {
		return nil
	}

	cp.logger.Info("compressing history", "tokens", history.EstimateTokens(), "threshold", cp.ThresholdToks)

	msgs := history.GetMessages()
	if len(msgs) <= cp.RecencyWindow {
		return nil // nothing old enough to compress
	}

	// Partition: older messages → compress; last N → keep verbatim.
	older := msgs[:len(msgs)-cp.RecencyWindow]
	recency := msgs[len(msgs)-cp.RecencyWindow:]

	// Convert to sense.ConversationMessage slice.
	senseMessages := make([]sense.ConversationMessage, 0, len(older))
	for _, m := range older {
		senseMessages = append(senseMessages, sense.ConversationMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	snapshot, err := cp.compressor.Compress(ctx, senseMessages)
	if err != nil {
		cp.logger.Warn("compression failed, keeping history unchanged", "err", err)
		return nil // non-fatal
	}

	// Optionally persist summary for cross-session restore.
	if cp.store != nil && snapshot.Summary != "" {
		if saveErr := cp.store.SaveSessionContext(ctx, cp.sessionID, snapshot.Summary, 7200); saveErr != nil {
			cp.logger.Warn("persist compressed context failed", "err", saveErr)
		}
	}

	// Rebuild history: clear → add summary system message → re-add recency.
	history.Clear()
	if snapshot.Summary != "" {
		history.AddSystemMessage("## Compressed Context\n" + snapshot.Summary)
	}
	for _, m := range recency {
		switch m.Role {
		case "user":
			history.AddUserMessage(m.Content)
		case "assistant":
			history.AddAssistantMessage(m.Content)
		default:
			history.AddSystemMessage(m.Content)
		}
	}

	cp.logger.Info("history compressed",
		"token_save", snapshot.TokenSave,
		"recency_kept", len(recency),
	)
	return nil
}
