package providers

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/velariumai/gorkbot/internal/events"
	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/providers"
	"github.com/velariumai/gorkbot/pkg/registry"
)

// MockProvider implements ai.AIProvider for testing
type MockProvider struct {
	name      string
	id        string
	pingErr   error
	pingSeq   []error
	pingCalls int
	modelID   string
	metadata  ai.ProviderMetadata
}

func (m *MockProvider) Name() string {
	return m.name
}

func (m *MockProvider) ID() registry.ProviderID {
	return registry.ProviderID(m.id)
}

func (m *MockProvider) Generate(ctx context.Context, prompt string) (string, error) {
	return "mock response", nil
}

func (m *MockProvider) Stream(ctx context.Context, prompt string, w io.Writer) error {
	return nil
}

func (m *MockProvider) GenerateWithHistory(ctx context.Context, history *ai.ConversationHistory) (string, error) {
	return "mock response", nil
}

func (m *MockProvider) StreamWithHistory(ctx context.Context, history *ai.ConversationHistory, w io.Writer) error {
	return nil
}

func (m *MockProvider) GetMetadata() ai.ProviderMetadata {
	if m.metadata.ID == "" {
		m.metadata.ID = m.id
		m.metadata.ContextSize = 100000
	}
	return m.metadata
}

func (m *MockProvider) WithModel(modelID string) ai.AIProvider {
	m.modelID = modelID
	return m
}

func (m *MockProvider) Ping(ctx context.Context) error {
	m.pingCalls++
	if len(m.pingSeq) > 0 {
		err := m.pingSeq[0]
		m.pingSeq = m.pingSeq[1:]
		return err
	}
	return m.pingErr
}

func (m *MockProvider) FetchModels(ctx context.Context) ([]registry.ModelDefinition, error) {
	return []registry.ModelDefinition{
		{ID: registry.ModelID(m.id), Name: m.name},
	}, nil
}

// MockProviderManager implements providers.Manager-like interface
type MockProviderManager struct {
	providers       map[string]ai.AIProvider
	sessionDisabled map[string]bool
	keys            map[string]string
}

func NewMockProviderManager() *MockProviderManager {
	return &MockProviderManager{
		providers:       make(map[string]ai.AIProvider),
		sessionDisabled: make(map[string]bool),
		keys:            make(map[string]string),
	}
}

func (m *MockProviderManager) GetProviderForModel(providerName, modelID string) (ai.AIProvider, error) {
	if prov, ok := m.providers[providerName]; ok {
		return prov.WithModel(modelID), nil
	}
	return nil, errors.New("provider not found")
}

func (m *MockProviderManager) GetBase(providerName string) (ai.AIProvider, error) {
	if prov, ok := m.providers[providerName]; ok {
		return prov, nil
	}
	return nil, errors.New("provider not found")
}

func (m *MockProviderManager) IsSessionDisabled(providerName string) bool {
	return m.sessionDisabled[providerName]
}

func (m *MockProviderManager) DisableForSession(providerName string) {
	m.sessionDisabled[providerName] = true
}

func (m *MockProviderManager) SetKey(ctx context.Context, providerName, key string, validate bool) error {
	if validate && key == "invalid" {
		return errors.New("invalid key")
	}
	m.keys[providerName] = key
	return nil
}

func (m *MockProviderManager) KeyStore() interface{} {
	return m
}

func (m *MockProviderManager) FormatStatus() string {
	return "Mock status"
}

// Test helpers
func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, nil))
}

func TestNewProviderCoordinator(t *testing.T) {
	primary := &MockProvider{name: "grok", id: "xai"}
	consultant := &MockProvider{name: "gemini", id: "google"}
	bus := events.NewBus()
	logger := newTestLogger()

	pc := NewProviderCoordinator(nil, primary, consultant, nil, bus, logger)

	if pc.Primary().Name() != "grok" {
		t.Errorf("expected primary provider to be grok")
	}
	if pc.Consultant().Name() != "gemini" {
		t.Errorf("expected consultant provider to be gemini")
	}
}

func TestSetPrimary_WithoutProviderManager(t *testing.T) {
	xai := &MockProvider{name: "grok", id: "xai"}

	pc := NewProviderCoordinator(nil, xai, nil, nil, events.NewBus(), newTestLogger())

	// Switch (will use WithModel since no provider manager)
	// This should NOT fail - it will update the model on the existing provider
	err := pc.SetPrimary(context.Background(), "google", "gemini-2.0-flash")
	if err != nil {
		t.Errorf("SetPrimary should succeed without provider manager (fallback to WithModel): %v", err)
	}
}

func TestSetPrimary_InvalidProvider(t *testing.T) {
	xai := &MockProvider{name: "grok", id: "xai"}
	pc := NewProviderCoordinator(nil, xai, nil, nil, events.NewBus(), newTestLogger())

	// Without provider manager, SetPrimary will just call WithModel
	// This should NOT fail - it will update the model on the existing provider
	err := pc.SetPrimary(context.Background(), "nonexistent", "model")
	if err != nil {
		t.Errorf("SetPrimary should succeed without provider manager (fallback): %v", err)
	}
}

func TestSetSecondary_WithoutProviderManager(t *testing.T) {
	xai := &MockProvider{name: "grok", id: "xai"}

	pc := NewProviderCoordinator(nil, xai, nil, nil, events.NewBus(), newTestLogger())

	pc.SetCallbacks(nil, nil, nil, func() {})

	err := pc.SetSecondary(context.Background(), "google", "gemini-2.0-flash")
	if err == nil {
		t.Errorf("SetSecondary should fail without provider manager")
	}
}

func TestGetCascadeOrder_Default(t *testing.T) {
	pc := NewProviderCoordinator(nil, nil, nil, nil, nil, newTestLogger())

	cascade := pc.GetCascadeOrder()
	if len(cascade) == 0 {
		t.Errorf("expected default cascade order")
	}
	if cascade[0] != providers.ProviderXAI {
		t.Errorf("expected first provider to be XAI")
	}
}

func TestSetCascadeOrder_Custom(t *testing.T) {
	pc := NewProviderCoordinator(nil, nil, nil, nil, nil, newTestLogger())

	customOrder := []string{providers.ProviderGoogle, providers.ProviderAnthropic, providers.ProviderXAI}
	pc.SetCascadeOrder(customOrder)

	cascade := pc.GetCascadeOrder()
	if len(cascade) != 3 {
		t.Errorf("expected 3 providers in custom cascade")
	}
	if cascade[0] != providers.ProviderGoogle {
		t.Errorf("expected custom order to be respected")
	}
}

func TestSetProviderKey_NilProviderManager(t *testing.T) {
	pc := NewProviderCoordinator(nil, nil, nil, nil, nil, newTestLogger())

	err := pc.SetProviderKey(context.Background(), "xai", "valid-key")
	if err == nil {
		t.Errorf("SetProviderKey should fail without provider manager")
	}
}

func TestGetProviderStatus_NilProviderManager(t *testing.T) {
	pc := NewProviderCoordinator(nil, nil, nil, nil, nil, newTestLogger())

	status := pc.GetProviderStatus()
	if status == "" {
		t.Errorf("expected non-empty provider status")
	}
	if status != "Provider manager not initialized" {
		t.Errorf("expected proper error message")
	}
}

func TestIsProviderOutage_AIErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		expect bool
	}{
		{"ErrUnauthorized", ai.ErrUnauthorized, true},
		{"ErrProviderDown", ai.ErrProviderDown, true},
		{"ErrRateLimit", ai.ErrRateLimit, true},
		{"Status 429", errors.New("status 429"), true},
		{"Status 503", errors.New("status 503"), true},
		{"Quota error", errors.New("insufficient_quota"), true},
		{"Connection reset", errors.New("connection reset by peer"), true},
		{"Generic error", errors.New("something went wrong"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isProviderOutage(tt.err)
			if result != tt.expect {
				t.Errorf("expected %v, got %v for error: %v", tt.expect, result, tt.err)
			}
		})
	}
}

func TestSelectConsultant_StaticConsultant(t *testing.T) {
	primary := &MockProvider{name: "grok", id: "xai"}
	consultant := &MockProvider{name: "gemini", id: "google"}

	pc := NewProviderCoordinator(nil, primary, consultant, nil, nil, newTestLogger())

	selected := pc.SelectConsultant(context.Background(), "test task")
	if selected.Name() != "gemini" {
		t.Errorf("expected static consultant to be returned")
	}
}

func TestSelectConsultant_NoDiscovery(t *testing.T) {
	primary := &MockProvider{name: "grok", id: "xai"}

	pc := NewProviderCoordinator(nil, primary, nil, nil, nil, newTestLogger())

	// Without discovery, should return nil
	selected := pc.SelectConsultant(context.Background(), "test task")
	if selected != nil {
		t.Errorf("expected nil when no discovery available")
	}
}

func TestConcurrentAccess(t *testing.T) {
	primary := &MockProvider{name: "grok", id: "xai"}

	pc := NewProviderCoordinator(nil, primary, nil, nil, events.NewBus(), newTestLogger())

	// Run concurrent reads and writes
	go func() {
		for i := 0; i < 10; i++ {
			pc.Primary()
		}
	}()

	go func() {
		for i := 0; i < 10; i++ {
			pc.SetCascadeOrder([]string{providers.ProviderGoogle})
		}
	}()

	go func() {
		for i := 0; i < 10; i++ {
			pc.GetCascadeOrder()
		}
	}()

	// No panic = success (race detector would catch issues)
	t.Log("Concurrent access test passed")
}

func TestPingWithRetry_SucceedsAfterTransient(t *testing.T) {
	p := &MockProvider{
		name:    "grok",
		id:      "xai",
		pingSeq: []error{ai.ErrRateLimit, nil},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := pingWithRetry(ctx, p, 3, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("expected retry to succeed, got error: %v", err)
	}
	if p.pingCalls != 2 {
		t.Fatalf("expected 2 ping attempts, got %d", p.pingCalls)
	}
}

func TestPingWithRetry_DoesNotRetryPermanent(t *testing.T) {
	p := &MockProvider{
		name:    "grok",
		id:      "xai",
		pingSeq: []error{ai.ErrUnauthorized},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := pingWithRetry(ctx, p, 3, 10*time.Millisecond)
	if err == nil {
		t.Fatalf("expected permanent error, got nil")
	}
	if p.pingCalls != 1 {
		t.Fatalf("expected no retries for permanent error, got %d attempts", p.pingCalls)
	}
}

func TestPingWithRetry_RespectsContextCancel(t *testing.T) {
	p := &MockProvider{
		name:    "grok",
		id:      "xai",
		pingSeq: []error{ai.ErrRateLimit, ai.ErrRateLimit, ai.ErrRateLimit},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := pingWithRetry(ctx, p, 3, 10*time.Millisecond)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got: %v", err)
	}
	if p.pingCalls != 1 {
		t.Fatalf("expected exactly one ping before cancel short-circuit, got %d", p.pingCalls)
	}
}

type tempNetErr struct{}

func (tempNetErr) Error() string   { return "temporary network error" }
func (tempNetErr) Timeout() bool   { return false }
func (tempNetErr) Temporary() bool { return true }

func TestIsRetryableProviderErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "rate_limit", err: ai.ErrRateLimit, want: true},
		{name: "temporary_net", err: tempNetErr{}, want: true},
		{name: "status_503_message", err: errors.New("provider returned status 503"), want: true},
		{name: "permanent_auth", err: ai.ErrUnauthorized, want: false},
		{name: "generic", err: errors.New("boom"), want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isRetryableProviderErr(tc.err)
			if got != tc.want {
				t.Fatalf("isRetryableProviderErr(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
