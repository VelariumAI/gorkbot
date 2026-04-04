package providers

import (
	"context"
)

// AIProvider is the common interface all AI providers must implement
type AIProvider interface {
	// Core execution methods
	Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResponse, error)
	Stream(ctx context.Context, req *ExecuteRequest) (<-chan *StreamChunk, error)

	// Health and capabilities
	HealthCheck(ctx context.Context) error
	Capabilities() *ProviderCapabilities

	// Cost tracking and rate limiting
	EstimateCost(req *ExecuteRequest) float64
	GetRateLimit() *RateLimit

	// Lifecycle
	Name() string
	Close() error
}

// ExecuteRequest represents a request to execute a task with an AI provider
type ExecuteRequest struct {
	// Core parameters
	Messages     []*Message     `json:"messages"`
	SystemPrompt string         `json:"system_prompt,omitempty"`
	Tools        []*Tool        `json:"tools,omitempty"`
	ModelOverride string         `json:"model_override,omitempty"` // Override default model

	// Model parameters
	Temperature   float32 `json:"temperature,omitempty"`
	TopP          float32 `json:"top_p,omitempty"`
	MaxTokens     int     `json:"max_tokens,omitempty"`
	StopSequences []string `json:"stop_sequences,omitempty"`

	// Extended thinking (if provider supports)
	ThinkingBudget int `json:"thinking_budget,omitempty"`

	// Metadata
	RequestID string `json:"request_id,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// ExecuteResponse represents the response from an AI provider
type ExecuteResponse struct {
	// Generated content
	Content string `json:"content"`

	// Tool calls (if any)
	ToolCalls []*ToolCall `json:"tool_calls,omitempty"`

	// Thinking (if extended thinking was used)
	Thinking string `json:"thinking,omitempty"`

	// Token usage
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`

	// Cost in USD
	Cost float64 `json:"cost"`

	// Metadata
	Model     string `json:"model"`
	Provider  string `json:"provider"`
	RequestID string `json:"request_id"`

	// Timestamps
	StartTime   int64 `json:"start_time"`   // Unix millis
	EndTime     int64 `json:"end_time"`     // Unix millis
	Duration    int64 `json:"duration_ms"`  // Milliseconds

	// Raw response (for debugging)
	RawResponse map[string]interface{} `json:"raw_response,omitempty"`
}

// StreamChunk represents a chunk of streamed response
type StreamChunk struct {
	// Type of chunk
	Type ChunkType `json:"type"` // "text", "tool_call", "thinking", "delta", "done", "error"

	// Delta text (for text chunks)
	Text string `json:"text,omitempty"`

	// Tool call (for tool_call chunks)
	ToolCall *ToolCall `json:"tool_call,omitempty"`

	// Thinking text (for thinking chunks)
	Thinking string `json:"thinking,omitempty"`

	// Error (for error chunks)
	Error string `json:"error,omitempty"`

	// Metadata
	Model     string `json:"model,omitempty"`
	Provider  string `json:"provider,omitempty"`
	RequestID string `json:"request_id,omitempty"`

	// Final token counts (in done chunk)
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
	Cost         float64 `json:"cost,omitempty"`
}

// ChunkType represents the type of stream chunk
type ChunkType string

const (
	ChunkTypeText     ChunkType = "text"
	ChunkTypeThinking ChunkType = "thinking"
	ChunkTypeToolCall ChunkType = "tool_call"
	ChunkTypeDelta    ChunkType = "delta"
	ChunkTypeDone     ChunkType = "done"
	ChunkTypeError    ChunkType = "error"
)

// Message represents a message in a conversation
type Message struct {
	Role    string `json:"role"`                       // "user", "assistant", "system"
	Content string `json:"content"`                    // Text content
	ToolID  string `json:"tool_use_id,omitempty"`      // ID of tool use
	Name    string `json:"name,omitempty"`             // Function name
	Images  []Image `json:"images,omitempty"`          // Images (if vision supported)
}

// Image represents an image in a message
type Image struct {
	Type    string `json:"type"`    // "base64", "url", "path"
	Data    string `json:"data"`    // Base64 encoded or URL
	MediaType string `json:"media_type,omitempty"` // "image/jpeg", "image/png", etc.
}

// Tool represents a tool/function the AI can call
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema interface{} `json:"input_schema"` // JSON Schema
	Category    string `json:"category,omitempty"` // "shell", "file", "web", "git", etc.
}

// ToolCall represents a call to a tool made by the AI
type ToolCall struct {
	ID       string `json:"id"`
	ToolName string `json:"tool_name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ProviderCapabilities describes what a provider can do
type ProviderCapabilities struct {
	// Thinking/reasoning support
	SupportsThinking bool `json:"supports_thinking"`
	ThinkingBudget   int  `json:"thinking_budget"` // Max tokens

	// Vision support
	SupportsVision bool `json:"supports_vision"`

	// Tool calling
	SupportsNativeTools bool `json:"supports_native_tools"` // Response API or similar

	// Caching support (prompt/KV caching)
	SupportsPromptCaching bool `json:"supports_prompt_caching"`

	// Streaming support
	SupportsStreaming bool `json:"supports_streaming"`

	// Context window size
	MaxContextWindow int `json:"max_context_window"`

	// Cost per million tokens (input/output)
	CostPer1MInputTokens  float64 `json:"cost_per_1m_input_tokens"`
	CostPer1MOutputTokens float64 `json:"cost_per_1m_output_tokens"`

	// Rate limits
	RequestsPerMinute int `json:"requests_per_minute"`
	TokensPerMinute   int `json:"tokens_per_minute"`

	// Other capabilities
	SupportsImages     bool   `json:"supports_images"`
	SupportsAudio      bool   `json:"supports_audio"`
	SupportsVideo      bool   `json:"supports_video"`
	SupportsVideoFrames int    `json:"supports_video_frames"` // Max frames
	DefaultTemperature float32 `json:"default_temperature"`

	// Latency characteristics
	AverageLatencyMs int `json:"average_latency_ms"`
}

// RateLimit describes rate limiting for a provider
type RateLimit struct {
	// Request rate limit
	RequestsPerMinute int `json:"requests_per_minute"`
	RequestsPerDay    int `json:"requests_per_day"`

	// Token rate limit
	TokensPerMinute int `json:"tokens_per_minute"`
	TokensPerDay    int `json:"tokens_per_day"`

	// Current usage
	CurrentRequestCount int `json:"current_request_count"`
	CurrentTokenCount   int `json:"current_token_count"`

	// Reset times (Unix timestamps)
	MinuteResetAt int64 `json:"minute_reset_at"`
	DayResetAt    int64 `json:"day_reset_at"`
}

// HealthCheckResult represents the result of a health check
type HealthCheckResult struct {
	Healthy   bool   `json:"healthy"`
	Message   string `json:"message"`
	Latency   int64  `json:"latency_ms"`
	Timestamp int64  `json:"timestamp"`
}

// CostEstimate represents an estimated cost for a request
type CostEstimate struct {
	InputCost  float64 `json:"input_cost"`
	OutputCost float64 `json:"output_cost"`
	TotalCost  float64 `json:"total_cost"`

	EstimatedInputTokens  int `json:"estimated_input_tokens"`
	EstimatedOutputTokens int `json:"estimated_output_tokens"`

	Model    string `json:"model"`
	Provider string `json:"provider"`
}

// ProviderError represents an error from a provider
type ProviderError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Provider  string `json:"provider"`
	Retriable bool   `json:"retriable"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

func (pe *ProviderError) Error() string {
	return pe.Message
}

// IsRetriable returns true if this error might succeed on retry
func (pe *ProviderError) IsRetriable() bool {
	return pe.Retriable
}

// IsFinal returns true if this error will not succeed on retry
func (pe *ProviderError) IsFinal() bool {
	return !pe.Retriable
}

// BatchRequest represents a batch of requests to process
type BatchRequest struct {
	Requests  []*ExecuteRequest `json:"requests"`
	Parallel  bool              `json:"parallel"`
	Timeout   int               `json:"timeout_seconds"`
}

// BatchResponse represents a batch response
type BatchResponse struct {
	Responses []*ExecuteResponse `json:"responses"`
	Errors    []error            `json:"errors,omitempty"`
	Duration  int64              `json:"duration_ms"`
}
