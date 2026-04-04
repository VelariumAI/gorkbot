package engine

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	engproviders "github.com/velariumai/gorkbot/internal/engine/providers"
	"github.com/velariumai/gorkbot/pkg/adaptive"
	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/selfimprove"
	"github.com/velariumai/gorkbot/pkg/sre"
	"github.com/velariumai/gorkbot/pkg/tools"
)

type testTool struct {
	name   string
	result *tools.ToolResult
	err    error
}

func (t *testTool) Name() string                 { return t.name }
func (t *testTool) Description() string          { return "test tool" }
func (t *testTool) Category() tools.ToolCategory { return tools.CategoryMeta }
func (t *testTool) Parameters() json.RawMessage  { return json.RawMessage(`{"type":"object"}`) }
func (t *testTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
	return t.result, t.err
}
func (t *testTool) RequiresPermission() bool                 { return false }
func (t *testTool) DefaultPermission() tools.PermissionLevel { return tools.PermissionAlways }
func (t *testTool) OutputFormat() tools.OutputFormat         { return tools.FormatText }

func TestTraceLogger(t *testing.T) {
	t1 := NewTraceLogger("", true)
	if t1.Enabled() {
		t.Fatalf("expected disabled trace when no dir")
	}
	t1.LogToolCall("x", nil)
	t1.Close()

	dir := t.TempDir()
	t2 := NewTraceLogger(dir, true)
	if !t2.Enabled() {
		t.Fatalf("expected enabled trace logger")
	}
	t2.LogToolCall("bash", map[string]interface{}{"cmd": "echo hi"})
	t2.LogToolResult("bash", strings.Repeat("x", 5000), true, 10*time.Millisecond)
	t2.LogLLMRequest("grok-3", 123)
	t2.LogLLMResponse("grok-3", 55, 20*time.Millisecond)
	t2.LogModeChange("NORMAL", "PLAN")
	t2.LogHook("pre_tool_use", "blocked", true)
	path := t2.TracePath()
	if path == "" {
		t.Fatalf("expected trace file path")
	}
	t2.Close()
	if t2.Enabled() {
		t.Fatalf("expected disabled after close")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace file failed: %v", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		t.Fatalf("expected trace file contents")
	}
}

func TestWatchdogStreamMonitor(t *testing.T) {
	sm := NewStreamMonitor()
	if sev := sm.WriteToken("short"); sev != SeverityNone {
		t.Fatalf("expected none severity for tiny token")
	}

	// Force unstructured repetition.
	tok := strings.Repeat("a", 60)
	_ = sm.WriteToken(tok)
	_ = sm.WriteToken(tok)
	_ = sm.WriteToken(tok)
	sev := sm.WriteToken(tok)
	if sev != SeverityCritical && sev != SeverityWarning {
		t.Fatalf("expected warning/critical for repetitive stream, got %v", sev)
	}
	if diag := sm.GetDiagnostics(); diag == "" {
		t.Fatalf("expected diagnostics text")
	}

	// Force code-block branch for diagnostics.
	sm2 := NewStreamMonitor()
	_ = sm2.WriteToken("```" + strings.Repeat("b", 120))
	if !strings.Contains(sm2.GetDiagnostics(), "code block") {
		t.Fatalf("expected code block diagnostics")
	}
}

func TestPluginHooks_InitPlugins(t *testing.T) {
	orch := &Orchestrator{}
	if err := orch.InitPlugins(); err == nil {
		t.Fatalf("expected error when registry is nil")
	}

	origWD, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	defer os.Chdir(origWD)

	orch2 := &Orchestrator{Registry: tools.NewRegistry(nil), Logger: slog.Default()}
	if err := orch2.InitPlugins(); err != nil {
		t.Fatalf("expected non-fatal plugin init, got: %v", err)
	}
}

func TestReasoningHooksAndSREAdapters(t *testing.T) {
	orch := &Orchestrator{ConversationHistory: ai.NewConversationHistory(), Logger: slog.Default()}

	// Nil-safe paths
	if ws := orch.prepareGrounding(context.Background(), "task"); ws != nil {
		t.Fatalf("expected nil grounding state when SRE is nil")
	}
	orch.runEnsembleIfNeeded(context.Background(), adaptive.RouteDecision{})
	orch.prepareSREContext(1)
	if orch.appendSRECorrectionCheck("resp") {
		t.Fatalf("expected no correction when SRE is nil")
	}
	orch.anchorToolResult("x", "out", true)

	// InitSRE + basic non-nil paths
	provider := &taskTestProvider{
		name: "primary",
		generateH: func(ctx context.Context, history *ai.ConversationHistory) (string, error) {
			return `{"entities":["a"],"constraints":["b"],"facts":["c"],"anchors":{"k":"v"},"confidence":0.9}`, nil
		},
	}
	orch.ProviderCoord = engproviders.NewProviderCoordinator(nil, provider, nil, nil, nil, nil)
	orch.InitSRE(sre.SREConfig{GroundingEnabled: true, CoSEnabled: true, EnsembleEnabled: false})
	if orch.SRE == nil {
		t.Fatalf("expected SRE initialized")
	}

	ws := orch.prepareGrounding(context.Background(), "task")
	if ws == nil {
		t.Fatalf("expected grounding world model state")
	}
	orch.prepareSREContext(1)
	_ = orch.appendSRECorrectionCheck("response")
	orch.anchorToolResult("tool", "output", true)
}

func TestSelfImproveAdaptersAndWrappers(t *testing.T) {
	// sparkAdapter
	if got := (&sparkAdapter{}).GetLastState(); got != nil {
		t.Fatalf("expected nil spark state with nil adapter")
	}

	fw := NewFreeWillEngine()
	fw.enabled = true
	fwa := &freeWillAdapter{fw: fw}
	if err := fwa.AddObservation(context.Background(), selfimprove.FreeWillObsInput{ToolName: "x", Latency: 1, Confidence: 0.8, Context: "ctx"}); err != nil {
		t.Fatalf("unexpected add observation err: %v", err)
	}

	// Fill queue to force queue-full branch.
	for len(fw.observationQueue) < cap(fw.observationQueue) {
		select {
		case fw.observationQueue <- FreeWillObservation{}:
		default:
			break
		}
	}
	if err := fwa.AddObservation(context.Background(), selfimprove.FreeWillObsInput{}); err == nil || !strings.Contains(err.Error(), "queue full") {
		t.Fatalf("expected queue full error, got %v", err)
	}

	fw.proposalQueue <- FreeWillProposal{ProposedChange: "x", ConfidenceScore: 80, RiskLevel: "medium"}
	props := fwa.GetPendingProposals()
	if len(props) == 0 {
		t.Fatalf("expected drained pending proposals")
	}
	if len(fwa.GetPendingProposals()) != 0 {
		t.Fatalf("expected proposal queue drained")
	}

	if riskStringToFloat("low") != 0.2 || riskStringToFloat("critical") != 1.0 || riskStringToFloat("unknown") != 0.5 {
		t.Fatalf("unexpected riskStringToFloat mapping")
	}

	ha := &harnessAdapter{cwd: ""}
	if ha.FailingCount() != 0 || ha.TotalCount() != 0 || ha.ActiveFeatureID() != "" {
		t.Fatalf("unexpected harness adapter empty-cwd behavior")
	}
	if (&researchAdapter{}).BufferedCount() != 0 {
		t.Fatalf("expected research adapter buffered count 0")
	}

	reg := tools.NewRegistry(nil)
	ta := &toolRegistryAdapter{reg: reg}
	if _, err := ta.ExecuteTool(context.Background(), "missing", nil); err == nil {
		t.Fatalf("expected missing tool error")
	}
	okTool := &testTool{name: "ok", result: &tools.ToolResult{Success: true, Output: "ok"}}
	if err := reg.Register(okTool); err != nil {
		t.Fatalf("register tool failed: %v", err)
	}
	if out, err := ta.ExecuteTool(context.Background(), "ok", nil); err != nil || out != "ok" {
		t.Fatalf("unexpected tool execute result out=%q err=%v", out, err)
	}
	failTool := &testTool{name: "fail", result: &tools.ToolResult{Success: false, Error: "bad"}}
	if err := reg.Register(failTool); err != nil {
		t.Fatalf("register fail tool failed: %v", err)
	}
	if _, err := ta.ExecuteTool(context.Background(), "fail", nil); err == nil {
		t.Fatalf("expected failed tool error")
	}
	errTool := &testTool{name: "err", err: errors.New("boom")}
	if err := reg.Register(errTool); err != nil {
		t.Fatalf("register err tool failed: %v", err)
	}
	if _, err := ta.ExecuteTool(context.Background(), "err", nil); err == nil {
		t.Fatalf("expected execute error")
	}

	n := ""
	notify := &notifyAdapter{callback: func(s string) { n = s }}
	notify.Notify("hello")
	if n != "hello" {
		t.Fatalf("expected notify callback")
	}
	(&notifyAdapter{}).Notify("ignored")

	obs := &obsAdapter{hub: nil}
	obs.RecordSICycleStart()
	obs.RecordSIProposal()
	obs.RecordSIAccepted()
	obs.RecordSIRolledBack()
	obs.RecordSIFailed()
	obs.RecordSIExecutionLatency(time.Millisecond)
	obs.RecordSIScoreDelta(0.1)
	obs.RecordSIGateRejectionReason("x")
	obs.RecordSIToolError("tool")
	obs.RecordSIRollbackLatency(time.Millisecond)

	orch := &Orchestrator{Logger: slog.Default(), Registry: tools.NewRegistry(nil)}
	if err := orch.StartSelfImprove(); err == nil {
		t.Fatalf("expected StartSelfImprove to fail when SPARK missing")
	}
	if enabled := orch.ToggleSelfImprove(); enabled {
		t.Fatalf("expected ToggleSelfImprove false when init fails")
	}
	orch.StopSelfImprove() // nil-safe
	orch.SetSINotifyCallback(func(string) {})
	orch.SetEvolvePhaseCallback(func() {})
	if snap := orch.SISnapshot(); snap.Enabled {
		t.Fatalf("expected disabled SI snapshot with nil driver")
	}
	if snap := orch.TriggerSICycle(context.Background()); snap.Enabled {
		t.Fatalf("expected disabled trigger snapshot with nil driver")
	}

	orch.launchSIPostTask("ignored") // nil-safe when SI/FreeWill missing
	orch.SIDriver = nil
	orch.FreeWillEngine = NewFreeWillEngine()
	orch.launchSIPostTask("task summary")
}

func TestToolSuppressionLayerNameAndHITLHelpers(t *testing.T) {
	if (&ToolSuppressionLayer{}).Name() != "tool_suppression" {
		t.Fatalf("unexpected tool suppression layer name")
	}
	if !isLocalhost("http://localhost:8080") || !isLocalhost("http://127.0.0.1") || !isLocalhost("http://[::1]") {
		t.Fatalf("expected localhost detection")
	}
	if isLocalhost("https://example.com") {
		t.Fatalf("did not expect remote URL to be localhost")
	}

	h1 := hashParams(map[string]interface{}{"a": 1})
	h2 := hashParams(map[string]interface{}{"a": 1})
	if h1 == "" || h1 != h2 {
		t.Fatalf("expected deterministic non-empty param hash")
	}
	if hashParams(nil) != "" {
		t.Fatalf("expected empty hash for nil params")
	}

	g := NewHITLGuard()
	g.SetMemory(nil)
	if g.Memory != nil {
		t.Fatalf("expected memory to remain nil")
	}
}

func TestTraceLoggerConcurrentWriteSmoke(t *testing.T) {
	dir := t.TempDir()
	tl := NewTraceLogger(dir, true)
	if !tl.Enabled() {
		t.Fatalf("expected enabled trace logger")
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50; i++ {
			tl.LogToolCall("tool", map[string]interface{}{"i": i})
		}
	}()
	for i := 0; i < 50; i++ {
		tl.LogLLMRequest("model", i)
	}
	<-done
	path := tl.TracePath()
	tl.Close()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open trace failed: %v", err)
	}
	defer f.Close()
	buf := make([]byte, 1)
	if _, err := f.Read(buf); err != nil && err != io.EOF {
		t.Fatalf("read trace failed: %v", err)
	}
}
