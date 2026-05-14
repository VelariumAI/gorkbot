package sense

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/velariumai/gorkbot/pkg/trace"
)

type captureSink struct {
	events []trace.Event
}

func (c *captureSink) Emit(_ context.Context, e trace.Event) error {
	c.events = append(c.events, e)
	return nil
}

func (c *captureSink) Close() error { return nil }

func TestSENSETracerLegacyJSONLCompatibility(t *testing.T) {
	t.Setenv("GORKBOT_TRACE_MODE", "off")
	dir := t.TempDir()
	tr := NewSENSETracer(dir, "sid-1")
	tr.LogToolSuccess("read_file", `{"path":"a"}`, "ok", 3)
	tr.Close()

	files, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		t.Fatalf("glob failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one trace file, got %d", len(files))
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 {
		t.Fatalf("expected at least one trace line")
	}
	var ev SENSETrace
	if err := json.Unmarshal([]byte(lines[0]), &ev); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if ev.Kind != KindToolSuccess {
		t.Fatalf("expected tool_success, got %q", ev.Kind)
	}
	if ev.ToolName != "read_file" {
		t.Fatalf("expected tool name, got %q", ev.ToolName)
	}
}

func TestSENSETracerCanonicalAdapter(t *testing.T) {
	t.Setenv("GORKBOT_TRACE_MODE", "off")
	dir := t.TempDir()
	tr := NewSENSETracer(dir, "sid-2")
	sink := &captureSink{}
	tr.SetCanonicalSink(sink, trace.ModeReplay)
	tr.LogToolFailure("bash", `{"cmd":"x"}`, "timeout", 9)
	tr.Close()

	if len(sink.events) == 0 {
		t.Fatalf("expected canonical events")
	}
	got := sink.events[0]
	if got.Component != "sense" || got.EventKind != "tool_failure" {
		t.Fatalf("unexpected canonical event: %+v", got)
	}
	if got.Operator != trace.OperatorExecute {
		t.Fatalf("unexpected operator: %s", got.Operator)
	}
}
