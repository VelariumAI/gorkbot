package memory

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/registry"
)

// MockAIProvider for testing
type MockAIProvider struct {
	response string
	err      error
}

func (m *MockAIProvider) Generate(ctx context.Context, prompt string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func (m *MockAIProvider) GenerateWithHistory(ctx context.Context, history *ai.ConversationHistory) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func (m *MockAIProvider) Stream(ctx context.Context, prompt string, out io.Writer) error {
	return nil
}

func (m *MockAIProvider) StreamWithHistory(ctx context.Context, history *ai.ConversationHistory, out io.Writer) error {
	return nil
}

func (m *MockAIProvider) GetMetadata() ai.ProviderMetadata {
	return ai.ProviderMetadata{
		ID:          "mock",
		Name:        "Mock Provider",
		Description: "Mock provider for testing",
		ContextSize: 4096,
	}
}

func (m *MockAIProvider) Name() string {
	return "Mock"
}

func (m *MockAIProvider) ID() registry.ProviderID {
	return registry.ProviderID("mock")
}

func (m *MockAIProvider) Ping(ctx context.Context) error {
	return nil
}

func (m *MockAIProvider) FetchModels(ctx context.Context) ([]registry.ModelDefinition, error) {
	return []registry.ModelDefinition{}, nil
}

func (m *MockAIProvider) WithModel(model string) ai.AIProvider {
	return m
}

func TestFactExtractor_ExtractFacts_JSON(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Mock response with JSON facts
	response := `
Here are the extracted facts:
[
  {"subject": "Alice", "predicate": "works_at", "object": "Acme Corp", "confidence": 0.95},
  {"subject": "Acme Corp", "predicate": "located_in", "object": "New York", "confidence": 0.90},
  {"subject": "Alice", "predicate": "speaks", "object": "English", "confidence": 0.99}
]
`

	provider := &MockAIProvider{response: response}
	extractor := NewFactExtractor(provider, "test-model", logger)

	ctx := context.Background()
	req := ExtractionRequest{
		Text: "Alice works at Acme Corp in New York and speaks English",
	}

	result, err := extractor.ExtractFacts(ctx, req)
	if err != nil {
		t.Fatalf("failed to extract facts: %v", err)
	}

	if len(result.Facts) != 3 {
		t.Errorf("expected 3 facts, got %d", len(result.Facts))
	}

	// Verify facts
	if result.Facts[0].Subject != "Alice" {
		t.Errorf("expected subject 'Alice', got '%s'", result.Facts[0].Subject)
	}

	if result.Facts[0].Predicate != "works_at" {
		t.Errorf("expected predicate 'works_at', got '%s'", result.Facts[0].Predicate)
	}

	if result.Facts[0].Confidence != 0.95 {
		t.Errorf("expected confidence 0.95, got %f", result.Facts[0].Confidence)
	}

	t.Logf("✓ Extracted %d facts with confidence scores", len(result.Facts))
}

func TestFactExtractor_ExtractFacts_Sentiment(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Response with positive sentiment
	response := `
I love this amazing product! It's great and excellent.
[
  {"subject": "product", "predicate": "quality", "object": "excellent", "confidence": 0.9}
]
`

	provider := &MockAIProvider{response: response}
	extractor := NewFactExtractor(provider, "test-model", logger)

	ctx := context.Background()
	result, _ := extractor.ExtractFacts(ctx, ExtractionRequest{Text: "Test text"})

	if result.Sentiment != "positive" {
		t.Errorf("expected positive sentiment, got %s", result.Sentiment)
	}

	t.Logf("✓ Sentiment detected: %s", result.Sentiment)
}

func TestFactExtractor_Deduplication(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Response with duplicate facts
	response := `
[
  {"subject": "Bob", "predicate": "likes", "object": "coffee", "confidence": 0.9},
  {"subject": "Bob", "predicate": "likes", "object": "coffee", "confidence": 0.85},
  {"subject": "Bob", "predicate": "likes", "object": "tea", "confidence": 0.7}
]
`

	provider := &MockAIProvider{response: response}
	extractor := NewFactExtractor(provider, "test-model", logger)

	ctx := context.Background()
	result, _ := extractor.ExtractFacts(ctx, ExtractionRequest{Text: "Bob likes coffee"})

	// Should deduplicate to 2 unique facts
	if len(result.Facts) != 2 {
		t.Errorf("expected 2 unique facts after dedup, got %d", len(result.Facts))
	}

	stats := extractor.GetStats()
	if dedup, ok := stats["deduplicated"].(int64); ok && dedup > 0 {
		t.Logf("✓ Deduplicated %d duplicate facts", dedup)
	}
}

func TestFactExtractor_EmptyText(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	provider := &MockAIProvider{response: ""}
	extractor := NewFactExtractor(provider, "test-model", logger)

	ctx := context.Background()
	_, err := extractor.ExtractFacts(ctx, ExtractionRequest{Text: ""})

	if err == nil {
		t.Error("expected error for empty text")
	}

	t.Logf("✓ Empty text validation works")
}

func TestFactExtractor_Confidence(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	response := `
[
  {"subject": "X", "predicate": "is", "object": "Y", "confidence": 0.99},
  {"subject": "A", "predicate": "equals", "object": "B"}
]
`

	provider := &MockAIProvider{response: response}
	extractor := NewFactExtractor(provider, "test-model", logger)

	ctx := context.Background()
	result, _ := extractor.ExtractFacts(ctx, ExtractionRequest{Text: "Test"})

	// First fact has explicit confidence
	if result.Facts[0].Confidence != 0.99 {
		t.Errorf("expected confidence 0.99, got %f", result.Facts[0].Confidence)
	}

	// Second fact should get default confidence
	if result.Facts[1].Confidence != 0.7 {
		t.Errorf("expected default confidence 0.7, got %f", result.Facts[1].Confidence)
	}

	t.Logf("✓ Confidence handling: explicit=%.2f, default=%.2f",
		result.Facts[0].Confidence, result.Facts[1].Confidence)
}

func TestFactExtractor_ExtractedAt(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	response := `
[
  {"subject": "fact", "predicate": "type", "object": "example", "confidence": 0.8}
]
`

	provider := &MockAIProvider{response: response}
	extractor := NewFactExtractor(provider, "test-model", logger)

	ctx := context.Background()
	result, _ := extractor.ExtractFacts(ctx, ExtractionRequest{Text: "Test"})

	if result.Facts[0].ExtractedAt.IsZero() {
		t.Error("ExtractedAt should be set")
	}

	t.Logf("✓ Fact timestamp recorded: %v", result.Facts[0].ExtractedAt)
}

func TestFactExtractor_Topics(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	response := `
[
  {"subject": "Python", "predicate": "supports", "object": "async/await", "confidence": 0.95},
  {"subject": "Golang", "predicate": "supports", "object": "goroutines", "confidence": 0.95}
]
`

	provider := &MockAIProvider{response: response}
	extractor := NewFactExtractor(provider, "test-model", logger)

	ctx := context.Background()
	result, _ := extractor.ExtractFacts(ctx, ExtractionRequest{Text: "Test about programming languages"})

	if len(result.Topics) == 0 {
		t.Error("expected topics to be extracted")
	}

	t.Logf("✓ Topics extracted: %v", result.Topics)
}

func TestFactExtractor_Context(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	response := `
[
  {"subject": "item", "predicate": "value", "object": "data", "confidence": 0.8}
]
`

	provider := &MockAIProvider{response: response}
	extractor := NewFactExtractor(provider, "test-model", logger)

	ctx := context.Background()
	req := ExtractionRequest{
		Text:    "Some text",
		Context: "Background information for extraction",
		Language: "en",
	}

	result, _ := extractor.ExtractFacts(ctx, req)

	if result.ErrorMsg != "" {
		t.Errorf("unexpected error: %s", result.ErrorMsg)
	}

	t.Logf("✓ Extraction with context successful")
}

func TestFactExtractor_ProviderError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	testErr := fmt.Errorf("provider unavailable")
	provider := &MockAIProvider{err: testErr}
	extractor := NewFactExtractor(provider, "test-model", logger)

	ctx := context.Background()
	result, err := extractor.ExtractFacts(ctx, ExtractionRequest{Text: "Test"})

	if err == nil {
		t.Error("expected error from provider")
	}

	if result.ErrorMsg == "" {
		t.Error("expected error message in response")
	}

	t.Logf("✓ Provider errors handled gracefully")
}

func TestFactExtractor_Stats(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	response := `
[
  {"subject": "A", "predicate": "B", "object": "C", "confidence": 0.8}
]
`

	provider := &MockAIProvider{response: response}
	extractor := NewFactExtractor(provider, "haiku", logger)

	ctx := context.Background()
	extractor.ExtractFacts(ctx, ExtractionRequest{Text: "Test 1"})
	extractor.ExtractFacts(ctx, ExtractionRequest{Text: "Test 2"})

	stats := extractor.GetStats()

	if extracted, ok := stats["extracted"].(int64); ok && extracted > 0 {
		t.Logf("✓ Stats tracked: %d facts extracted", extracted)
	} else {
		t.Error("expected extracted count in stats")
	}

	if model, ok := stats["model"].(string); ok && model == "haiku" {
		t.Logf("✓ Model tracked: %s", model)
	}
}

func TestFactExtractor_AlternativeFormat(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Response in alternative text format (not JSON)
	response := `
Extracted facts:
subject: Charlie
predicate: manages
object: team
confidence: 0.85

subject: team
predicate: has_size
object: 5
confidence: 0.90
`

	provider := &MockAIProvider{response: response}
	extractor := NewFactExtractor(provider, "test-model", logger)

	ctx := context.Background()
	result, _ := extractor.ExtractFacts(ctx, ExtractionRequest{Text: "Charlie manages a team of 5"})

	if len(result.Facts) > 0 {
		t.Logf("✓ Alternative format parsing works: %d facts", len(result.Facts))
	}
}

func TestFactExtractor_Metadata(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	response := `
[
  {"subject": "item", "predicate": "property", "object": "value", "confidence": 0.8, "metadata": {"source": "test"}}
]
`

	provider := &MockAIProvider{response: response}
	extractor := NewFactExtractor(provider, "test-model", logger)

	ctx := context.Background()
	result, _ := extractor.ExtractFacts(ctx, ExtractionRequest{Text: "Test"})

	if len(result.Facts) > 0 && result.Facts[0].Metadata != nil {
		t.Logf("✓ Metadata preserved in facts")
	}
}

// Benchmark tests

func BenchmarkFactExtractor_ExtractFacts(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	response := `
[
  {"subject": "A", "predicate": "B", "object": "C", "confidence": 0.8},
  {"subject": "D", "predicate": "E", "object": "F", "confidence": 0.9}
]
`

	provider := &MockAIProvider{response: response}
	extractor := NewFactExtractor(provider, "test-model", logger)

	ctx := context.Background()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		extractor.ExtractFacts(ctx, ExtractionRequest{Text: "Test text"})
	}
}

func BenchmarkFactExtractor_Deduplication(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	facts := make([]ExtractedFact, 100)
	for i := 0; i < 100; i++ {
		facts[i] = ExtractedFact{
			Subject:    "subject",
			Predicate:  "predicate",
			Object:     "object",
			Confidence: 0.8,
		}
	}

	extractor := NewFactExtractor(nil, "test-model", logger)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		extractor.deduplicateFacts(facts)
	}
}
