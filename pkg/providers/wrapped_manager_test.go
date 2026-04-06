package providers

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/registry"
)

type mockProvider struct {
	modelID string
}

func (m *mockProvider) Generate(_ context.Context, _ string) (string, error) {
	return "gen-response", nil
}

func (m *mockProvider) GenerateWithHistory(_ context.Context, _ *ai.ConversationHistory) (string, error) {
	return "hist-response", nil
}

func (m *mockProvider) Stream(_ context.Context, _ string, out io.Writer) error {
	_, _ = out.Write([]byte("stream-out"))
	return nil
}

func (m *mockProvider) StreamWithHistory(_ context.Context, _ *ai.ConversationHistory, out io.Writer) error {
	_, _ = out.Write([]byte("stream-hist-out"))
	return nil
}

func (m *mockProvider) GetMetadata() ai.ProviderMetadata {
	return ai.ProviderMetadata{ID: m.modelID}
}

func (m *mockProvider) Name() string { return "mock" }

func (m *mockProvider) ID() registry.ProviderID { return "mock" }

func (m *mockProvider) Ping(_ context.Context) error { return nil }

func (m *mockProvider) FetchModels(_ context.Context) ([]registry.ModelDefinition, error) {
	return nil, nil
}

func (m *mockProvider) WithModel(model string) ai.AIProvider {
	return &mockProvider{modelID: model}
}

func TestWrappedProviderGenerateAndUsage(t *testing.T) {
	w := NewWrappedProvider(&mockProvider{modelID: "m1"}, nil)

	resp, err := w.Generate(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if resp != "gen-response" {
		t.Fatalf("unexpected generate response: %q", resp)
	}

	u := w.LastUsage()
	if u.TotalTokens != u.PromptTokens+u.CompletionTokens {
		t.Fatalf("usage totals inconsistent: %+v", u)
	}
}

func TestWrappedProviderStreamAndUsage(t *testing.T) {
	w := NewWrappedProvider(&mockProvider{modelID: "m1"}, nil)
	var out bytes.Buffer

	if err := w.Stream(context.Background(), "prompt", &out); err != nil {
		t.Fatalf("stream failed: %v", err)
	}
	if out.String() != "stream-out" {
		t.Fatalf("unexpected stream output: %q", out.String())
	}

	u := w.LastUsage()
	if u.TotalTokens != u.PromptTokens+u.CompletionTokens {
		t.Fatalf("usage totals inconsistent after stream: %+v", u)
	}
}

func TestWrappedProviderWithModel(t *testing.T) {
	w := NewWrappedProvider(&mockProvider{modelID: "m1"}, nil)
	derived := w.WithModel("m2")

	ww, ok := derived.(*WrappedProvider)
	if !ok {
		t.Fatalf("expected *WrappedProvider, got %T", derived)
	}
	if got := ww.GetMetadata().ID; got != "m2" {
		t.Fatalf("expected model m2, got %q", got)
	}
}

func TestTokenCounterWriterNilEncoding(t *testing.T) {
	var out bytes.Buffer
	cw := &tokenCounterWriter{target: &out, bpe: nil}
	_, _ = cw.Write([]byte("hello"))
	if got := cw.Count(); got != 0 {
		t.Fatalf("expected zero token count with nil encoder, got %d", got)
	}
}

func TestWrappedProviderDelegationMethods(t *testing.T) {
	w := NewWrappedProvider(&mockProvider{modelID: "m1"}, nil)
	if w.Name() != "mock" {
		t.Fatalf("unexpected delegated name")
	}
	if w.ID() != "mock" {
		t.Fatalf("unexpected delegated id")
	}
	if w.GetMetadata().ID != "m1" {
		t.Fatalf("unexpected delegated metadata")
	}
	if err := w.Ping(context.Background()); err != nil {
		t.Fatalf("delegated ping failed: %v", err)
	}
	if _, err := w.FetchModels(context.Background()); err != nil {
		t.Fatalf("delegated fetch models failed: %v", err)
	}

	h := ai.NewConversationHistory()
	h.AddUserMessage("hello")
	resp, err := w.GenerateWithHistory(context.Background(), h)
	if err != nil || resp != "hist-response" {
		t.Fatalf("GenerateWithHistory delegation failed, resp=%q err=%v", resp, err)
	}

	var out bytes.Buffer
	if err := w.StreamWithHistory(context.Background(), h, &out); err != nil {
		t.Fatalf("StreamWithHistory delegation failed: %v", err)
	}
	if out.String() != "stream-hist-out" {
		t.Fatalf("unexpected stream with history output: %q", out.String())
	}
}

type errProvider struct{ mockProvider }

func (e *errProvider) Generate(context.Context, string) (string, error) {
	return "", context.DeadlineExceeded
}

func TestWrappedProviderGenerateErrorPath(t *testing.T) {
	w := NewWrappedProvider(&errProvider{mockProvider{modelID: "m1"}}, nil)
	resp, err := w.Generate(context.Background(), "prompt")
	if err == nil {
		t.Fatalf("expected generate error")
	}
	if resp != "" {
		t.Fatalf("expected empty response on error")
	}
	u := w.LastUsage()
	if u.TotalTokens != 0 {
		t.Fatalf("expected usage to stay zero on error, got %+v", u)
	}
}

func TestWrappedProviderCountTokensAndRecordUsageQueue(t *testing.T) {
	w := NewWrappedProvider(&mockProvider{modelID: "m1"}, nil)
	if got := w.countTokens(""); got != 0 {
		t.Fatalf("expected zero count for empty text, got %d", got)
	}
	if got := w.countTokens("hello world"); got <= 0 {
		t.Fatalf("expected positive token count")
	}

	orig := usageLogCh
	defer func() { usageLogCh = orig }()
	usageLogCh = make(chan logEntry, 1)
	usageLogCh <- logEntry{} // fill channel to force default/drop branch

	done := make(chan struct{})
	go func() {
		w.recordUsage(3, 4)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("recordUsage should not block when usage channel is full")
	}
	if got := w.LastUsage().TotalTokens; got != 7 {
		t.Fatalf("unexpected total tokens after recordUsage: %d", got)
	}
}

func TestDrainUsageLogFlushOnClose(t *testing.T) {
	orig := usageLogCh
	defer func() { usageLogCh = orig }()
	usageLogCh = make(chan logEntry, 2)

	logPath := filepath.Join(t.TempDir(), "usage", "usage_log.jsonl")
	done := make(chan struct{})
	go func() {
		drainUsageLog(logPath)
		close(done)
	}()
	usageLogCh <- logEntry{Model: "m1", InputTokens: 1, OutputTokens: 2}
	close(usageLogCh)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("drainUsageLog did not exit after channel close")
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading usage log: %v", err)
	}
	if !strings.Contains(string(data), "\"model\":\"m1\"") {
		t.Fatalf("expected usage log entry, got %s", string(data))
	}
}
