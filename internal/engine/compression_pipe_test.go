package engine

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/sense"
)

type compressorGenStub struct {
	out string
	err error
}

func (g *compressorGenStub) Generate(ctx context.Context, prompt string) (string, error) {
	if g.err != nil {
		return "", g.err
	}
	if g.out != "" {
		return g.out, nil
	}
	return "## Session Context\nsummary", nil
}

func TestNewCompressionPipe_NilCompressor(t *testing.T) {
	if cp := NewCompressionPipe(nil, nil, "s1", slog.Default()); cp != nil {
		t.Fatalf("expected nil compression pipe when compressor is nil")
	}
}

func TestCompressionPipe_MaybeCompress_NoOpPaths(t *testing.T) {
	comp := sense.NewCompressor(&compressorGenStub{})
	cp := NewCompressionPipe(comp, nil, "s1", slog.Default())
	cp.ThresholdToks = 10000
	cp.RecencyWindow = 3

	h := ai.NewConversationHistory()
	h.AddUserMessage("short")
	h.AddAssistantMessage("short")

	before := h.GetMessages()
	if err := cp.MaybeCompress(context.Background(), h, false); err != nil {
		t.Fatalf("maybe compress failed: %v", err)
	}
	after := h.GetMessages()
	if len(after) != len(before) {
		t.Fatalf("expected unchanged history for below-threshold path")
	}

	cp.ThresholdToks = 0
	if err := cp.MaybeCompress(context.Background(), h, false); err != nil {
		t.Fatalf("maybe compress failed: %v", err)
	}
	if h.Count() != 2 {
		t.Fatalf("expected unchanged history when <= recency window")
	}
}

func TestCompressionPipe_MaybeCompress_SuccessAndRolePreservation(t *testing.T) {
	comp := sense.NewCompressor(&compressorGenStub{out: "## Session Context\ncompressed"})
	cp := NewCompressionPipe(comp, nil, "s1", slog.Default())
	cp.ThresholdToks = 0
	cp.RecencyWindow = 2

	h := ai.NewConversationHistory()
	h.AddUserMessage("older user message")
	h.AddAssistantMessage("older assistant message")
	h.AddToolCallMessage([]ai.ToolCallEntry{{ID: "c1", ToolName: "search", Arguments: "{}"}})
	h.AddToolResultMessage("c1", "search", "result payload")

	if err := cp.MaybeCompress(context.Background(), h, false); err != nil {
		t.Fatalf("maybe compress failed: %v", err)
	}

	msgs := h.GetMessages()
	if len(msgs) != 3 {
		t.Fatalf("expected summary + recency messages, got %d", len(msgs))
	}
	if msgs[0].Role != "system" || msgs[0].Content == "" {
		t.Fatalf("expected system summary message at index 0")
	}
	if msgs[1].Role != "assistant" || len(msgs[1].ToolCalls) == 0 {
		t.Fatalf("expected assistant tool-call message preserved")
	}
	if msgs[2].Role != "tool" || msgs[2].ToolCallID != "c1" {
		t.Fatalf("expected tool result message preserved")
	}
}

func TestCompressionPipe_MaybeCompress_CompressorErrorNonFatal(t *testing.T) {
	comp := sense.NewCompressor(&compressorGenStub{err: errors.New("compress fail")})
	cp := NewCompressionPipe(comp, nil, "s1", slog.Default())
	cp.ThresholdToks = 0
	cp.RecencyWindow = 1

	h := ai.NewConversationHistory()
	h.AddUserMessage("older user")
	h.AddAssistantMessage("recent assistant")

	before := h.GetMessages()
	if err := cp.MaybeCompress(context.Background(), h, false); err != nil {
		t.Fatalf("expected non-fatal error path, got err: %v", err)
	}
	after := h.GetMessages()
	if len(after) != len(before) {
		t.Fatalf("expected history unchanged on compressor failure")
	}
}
