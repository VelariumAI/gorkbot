package ai

import (
	"context"
	"io"

	"github.com/velariumai/gorkbot/pkg/registry"
)

// ProviderMetadata carries information about the underlying AI model.
type ProviderMetadata struct {
	ID          string
	Name        string
	Description string
	ContextSize int
}

// AIProvider defines the standard interface for any AI backend (Grok, Gemini, etc.).
type AIProvider interface {
	// Generate returns a complete text response for the given prompt.
	// Deprecated: Use GenerateWithHistory for conversation context
	Generate(ctx context.Context, prompt string) (string, error)

	// GenerateWithHistory returns a response with full conversation context
	GenerateWithHistory(ctx context.Context, history *ConversationHistory) (string, error)

	// Stream writes the response chunks to the provided writer as they arrive.
	Stream(ctx context.Context, prompt string, out io.Writer) error

	// StreamWithHistory writes response chunks with conversation context
	StreamWithHistory(ctx context.Context, history *ConversationHistory, out io.Writer) error

	// GetMetadata returns details about the model being used.
	GetMetadata() ProviderMetadata

	// Name returns the display name of the provider (e.g., "Grok", "Gemini").
	Name() string

	// ID returns the unique provider identifier (e.g., "xai", "google").
	ID() registry.ProviderID

	// Ping checks connectivity to the provider.
	Ping(ctx context.Context) error

	// FetchModels returns a list of models supported by this provider.
	// This is used by the Model Registry to dynamically update capabilities.
	FetchModels(ctx context.Context) ([]registry.ModelDefinition, error)

	// WithModel returns a new instance of the provider configured to use the specified model.
	// This allows the Registry/Router to select specific models dynamically.
	WithModel(model string) AIProvider
}

// ThinkingBudgetProvider is an optional interface for providers that support extended thinking.
// Providers implementing this interface can have their thinking budget configured dynamically.
type ThinkingBudgetProvider interface {
	// SetThinkingBudget sets the thinking budget for extended thinking support.
	SetThinkingBudget(budget int)
	// GetThinkingBudget returns the current thinking budget.
	GetThinkingBudget() int
}
