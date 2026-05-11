package providers

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/registry"
)

type fetchOnlyProvider struct {
	models []registry.ModelDefinition
	err    error
}

func (f *fetchOnlyProvider) Generate(context.Context, string) (string, error) { return "", nil }
func (f *fetchOnlyProvider) GenerateWithHistory(context.Context, *ai.ConversationHistory) (string, error) {
	return "", nil
}
func (f *fetchOnlyProvider) Stream(context.Context, string, io.Writer) error { return nil }
func (f *fetchOnlyProvider) StreamWithHistory(context.Context, *ai.ConversationHistory, io.Writer) error {
	return nil
}
func (f *fetchOnlyProvider) GetMetadata() ai.ProviderMetadata { return ai.ProviderMetadata{ID: "m"} }
func (f *fetchOnlyProvider) Name() string                     { return "fetch" }
func (f *fetchOnlyProvider) ID() registry.ProviderID          { return "fetch" }
func (f *fetchOnlyProvider) Ping(context.Context) error       { return nil }
func (f *fetchOnlyProvider) FetchModels(context.Context) ([]registry.ModelDefinition, error) {
	return f.models, f.err
}
func (f *fetchOnlyProvider) WithModel(string) ai.AIProvider { return f }

func TestManagerSessionDisableAndGetBase(t *testing.T) {
	m := &Manager{
		bases:           map[string]ai.AIProvider{"xai": &mockProvider{modelID: "m"}},
		logger:          slog.Default(),
		failureRates:    map[string]float64{},
		sessionDisabled: map[string]bool{},
	}

	m.DisableForSession("xai")
	if !m.IsSessionDisabled("xai") {
		t.Fatalf("expected provider to be disabled")
	}
	if _, err := m.GetBase("xai"); err == nil {
		t.Fatalf("expected disabled provider error")
	}

	m.EnableForSession("xai")
	if m.IsSessionDisabled("xai") {
		t.Fatalf("expected provider to be enabled")
	}
	if _, err := m.GetBase("xai"); err != nil {
		t.Fatalf("expected provider to be available, got %v", err)
	}
}

func TestManagerPollProviderAndList(t *testing.T) {
	models := []registry.ModelDefinition{{ID: "m1"}}
	m := &Manager{
		bases:           map[string]ai.AIProvider{"xai": &fetchOnlyProvider{models: models}},
		logger:          slog.Default(),
		failureRates:    map[string]float64{},
		sessionDisabled: map[string]bool{},
	}

	got, err := m.PollProvider(context.Background(), "xai")
	if err != nil {
		t.Fatalf("poll failed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected one model")
	}

	all := m.ListAvailableModels(context.Background())
	if len(all["xai"]) != 1 {
		t.Fatalf("expected one listed model")
	}
}

func TestManagerReliabilityAndReporting(t *testing.T) {
	m := &Manager{
		failureRates: map[string]float64{},
	}
	m.RecordOutcome("m1", true)
	if rate := m.FailureRate("m1"); rate <= 0 {
		t.Fatalf("expected positive failure rate")
	}
	m.RecordOutcome("m1", false)
	if rate := m.FailureRate("m1"); rate <= 0 || rate >= 1 {
		t.Fatalf("expected bounded EWMA, got %v", rate)
	}
	if got := m.ConfidenceReport(); !strings.Contains(got, "m1") {
		t.Fatalf("expected confidence report to include model id")
	}
}

func TestProviderNameAndWebsite(t *testing.T) {
	nameCases := map[string]string{
		ProviderXAI:        "xAI",
		ProviderGoogle:     "Google",
		ProviderAnthropic:  "Anthropic",
		ProviderOpenAI:     "OpenAI",
		ProviderMiniMax:    "MiniMax",
		ProviderOpenRouter: "OpenRouter",
		ProviderMoonshot:   "Moonshot",
		"custom":           "Custom",
		"":                 "",
	}
	for in, want := range nameCases {
		if got := ProviderName(in); got != want {
			t.Fatalf("ProviderName(%q)=%q want %q", in, got, want)
		}
	}

	siteCases := map[string]string{
		ProviderXAI:        "console.x.ai",
		ProviderGoogle:     "aistudio.google.com",
		ProviderAnthropic:  "console.anthropic.com/settings/keys",
		ProviderOpenAI:     "platform.openai.com/api-keys",
		ProviderMiniMax:    "platform.minimaxi.com/user-center/basic-information",
		ProviderOpenRouter: "openrouter.ai/settings/keys",
		ProviderMoonshot:   "platform.moonshot.cn/console/api-keys",
		"unknown":          "",
	}
	for in, want := range siteCases {
		if got := ProviderWebsite(in); got != want {
			t.Fatalf("ProviderWebsite(%q)=%q want %q", in, got, want)
		}
	}
}

func TestGlobalProviderManager(t *testing.T) {
	pm := &Manager{}
	SetGlobalProviderManager(pm)
	if GetGlobalProviderManager() != pm {
		t.Fatalf("expected global manager singleton to match")
	}
}
