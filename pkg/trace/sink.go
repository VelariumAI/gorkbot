package trace

import (
	"context"
	"strings"
	"time"
)

// Mode controls canonical trace detail emitted at runtime.
type Mode string

const (
	ModeOff     Mode = "off"
	ModeMinimal Mode = "minimal"
	ModeAudit   Mode = "audit"
	ModeDebug   Mode = "debug"
	ModeReplay  Mode = "replay"
)

// Sink receives canonical trace events.
type Sink interface {
	Emit(ctx context.Context, event Event) error
	Close() error
}

func ParseMode(raw string) Mode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "minimal":
		return ModeMinimal
	case "audit":
		return ModeAudit
	case "debug":
		return ModeDebug
	case "replay":
		return ModeReplay
	default:
		return ModeOff
	}
}

func Emit(ctx context.Context, sink Sink, mode Mode, event Event) error {
	if sink == nil || mode == ModeOff {
		return nil
	}
	normalized := ApplyMode(mode, event).Normalized()
	if normalized.Timestamp.IsZero() {
		normalized.Timestamp = time.Now().UTC()
	}
	if err := normalized.Validate(); err != nil {
		return err
	}
	return sink.Emit(ctx, normalized)
}

func ApplyMode(mode Mode, event Event) Event {
	out := event
	switch mode {
	case ModeMinimal:
		out.Decision = ""
		out.ArtifactRefs = nil
		out.ValidationRefs = nil
		out.ReceiptRefs = nil
		out.TokenUsage = TokenUsage{}
		out.CostEstimate = CostEstimate{}
		out.Metadata = BoundMetadata(out.Metadata)
	case ModeAudit:
		out.Metadata = BoundMetadata(out.Metadata)
		out.ArtifactRefs = boundRefs(out.ArtifactRefs)
		out.ValidationRefs = boundRefs(out.ValidationRefs)
		out.ReceiptRefs = boundRefs(out.ReceiptRefs)
	case ModeDebug:
		out.Metadata = BoundMetadata(out.Metadata)
	case ModeReplay:
		out.Metadata = BoundMetadata(out.Metadata)
		if out.RedactionState == "" {
			out.RedactionState = RedactionRedacted
		}
	default:
		out.Metadata = BoundMetadata(out.Metadata)
	}
	return out
}
