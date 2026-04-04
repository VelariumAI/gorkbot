package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type pbTestLayer struct {
	name string
	out  string
}

func (l *pbTestLayer) Name() string                  { return l.name }
func (l *pbTestLayer) Build(ctx BuildContext) string { return l.out }

func TestPromptBuilder_BasicOperations(t *testing.T) {
	pb := NewPromptBuilder()
	if len(pb.Layers()) < 5 {
		t.Fatalf("expected default layers")
	}

	pb.AddLayer(&pbTestLayer{name: "extra", out: "x"})
	pb.PrependLayer(&pbTestLayer{name: "first", out: "y"})
	if !pb.ReplaceLayer("extra", &pbTestLayer{name: "extra", out: "z"}) {
		t.Fatalf("expected replace layer success")
	}
	if pb.ReplaceLayer("missing", &pbTestLayer{name: "m", out: ""}) {
		t.Fatalf("expected replace missing layer to return false")
	}

	pb.DebugHeaders = true
	out := pb.Build(BuildContext{})
	if !strings.Contains(out, "LAYER: first") || !strings.Contains(out, "LAYER: extra") {
		t.Fatalf("expected debug headers in output")
	}
}

func TestIdentitySoulBootstrapRuntimeChannelLayers(t *testing.T) {
	d := t.TempDir()
	agentPath := filepath.Join(d, "AGENT.md")
	soulPath := filepath.Join(d, "SOUL.md")
	bootPath := filepath.Join(d, "BOOTSTRAP.md")
	if err := os.WriteFile(agentPath, []byte("agent identity"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(soulPath, []byte("soul tone"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bootPath, []byte("workspace guide"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := BuildContext{
		WorkDir:   d,
		SessionID: "sess-1",
		Model:     "grok-3",
		Platform:  "linux",
		Channel:   "telegram",
		ExtraVars: map[string]string{"Key": "Val"},
	}

	id := (&IdentityLayer{}).Build(ctx)
	if !strings.Contains(id, "Agent Identity") || !strings.Contains(id, "agent identity") {
		t.Fatalf("unexpected identity layer output: %s", id)
	}

	soul := (&SoulLayer{}).Build(ctx)
	if !strings.Contains(soul, "Personality") || !strings.Contains(soul, "soul tone") {
		t.Fatalf("unexpected soul layer output: %s", soul)
	}

	boot := (&BootstrapLayer{
		AvailableAgents: []AgentRef{{ID: "a1", Name: "Agent1", Description: "desc"}},
		CronSummary:     "cron info",
	}).Build(ctx)
	if !strings.Contains(boot, "Workspace Guide") || !strings.Contains(boot, "Available Agents") || !strings.Contains(boot, "Scheduled Tasks") {
		t.Fatalf("unexpected bootstrap output: %s", boot)
	}

	runtime := (&RuntimeLayer{}).Build(ctx)
	if !strings.Contains(runtime, "Runtime Context") || !strings.Contains(runtime, "sess-1") || !strings.Contains(runtime, "grok-3") {
		t.Fatalf("unexpected runtime output: %s", runtime)
	}

	ch := (&ChannelHintLayer{}).Build(ctx)
	if !strings.Contains(ch, "Channel") || !strings.Contains(strings.ToLower(ch), "telegram") {
		t.Fatalf("unexpected channel hint output: %s", ch)
	}
}

func TestLayerFallbacksAndHelpers(t *testing.T) {
	ctx := BuildContext{}
	if got := (&IdentityLayer{Identity: "fallback"}).Build(ctx); !strings.Contains(got, "fallback") {
		t.Fatalf("expected fallback identity")
	}
	if got := (&SoulLayer{}).Build(ctx); got != "" {
		t.Fatalf("expected empty soul without files")
	}
	if got := (&ChannelHintLayer{DefaultChannel: "api"}).Build(ctx); !strings.Contains(strings.ToLower(got), "api/websocket") {
		t.Fatalf("unexpected default channel hint: %s", got)
	}

	if channelHint("unknown-x") == "" {
		t.Fatalf("expected fallback channel hint for unknown channel")
	}
	if channelHint("") != "You are responding via \"\"." {
		t.Fatalf("expected explicit quote-wrapped empty-channel fallback")
	}

	d := t.TempDir()
	p := filepath.Join(d, "x.md")
	if got := readPromptFile(p); got != "" {
		t.Fatalf("expected empty read for missing file")
	}
	if err := os.WriteFile(p, []byte("  hello  \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readPromptFile(p); got != "hello" {
		t.Fatalf("unexpected readPromptFile output: %q", got)
	}
}

func TestToolSuppressionLayer(t *testing.T) {
	normal := (&ToolSuppressionLayer{DetectDiagnosticQuery: true, UserQuery: "write a function"}).Build(BuildContext{})
	if !strings.Contains(normal, "NEVER call diagnostic tools") {
		t.Fatalf("expected normal-query suppression guidance")
	}
	if !strings.Contains(normal, "Anti-Workaround Rules") {
		t.Fatalf("expected anti-workaround section")
	}

	diag := (&ToolSuppressionLayer{DetectDiagnosticQuery: true, UserQuery: "show system health status"}).Build(BuildContext{})
	if !strings.Contains(diag, "EXCEPTION: This query is about system diagnostics") {
		t.Fatalf("expected diagnostic exception guidance")
	}
}
