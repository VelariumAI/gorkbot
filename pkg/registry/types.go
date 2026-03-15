package registry

import (
	"context"
	"time"
)

// ModelID is a unique identifier for a model (e.g., "xai/grok-3")
type ModelID string

// ProviderID identifies the model source (e.g., "xai", "google")
type ProviderID string

// ModelStatus represents the availability state of a model
type ModelStatus int

const (
	StatusActive ModelStatus = iota
	StatusDeprecated
	StatusOffline
)

// CapabilitySet defines the standardized features of a model
type CapabilitySet struct {
	MaxContextTokens  int
	SupportsVision    bool
	SupportsTools     bool
	SupportsJSONMode  bool
	SupportsStreaming bool
	SupportsThinking  bool    // Supports native thinking/reasoning mode (e.g. Gemini 2.5+, thinking variants)
	InputCostPer1M    float64 // Normalized pricing (USD)
	OutputCostPer1M   float64
}

// ModelDefinition represents a fully normalized model entry
type ModelDefinition struct {
	ID           ModelID
	Provider     ProviderID
	Name         string
	Description  string
	Capabilities CapabilitySet
	Status       ModelStatus
	LastUpdated  time.Time
}

// ModelProvider defines the interface for fetching model lists from an API
type ModelProvider interface {
	ID() ProviderID
	FetchModels(ctx context.Context) ([]ModelDefinition, error)
}
