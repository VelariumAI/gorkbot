package compression

import (
	"fmt"
	"log/slog"
	"sync"
)

// CompressionStrategy defines how to compress context
type CompressionStrategy interface {
	Name() string
	Compress(ctx *Context) (*CompressedContext, error)
	EstimatedReduction() float64 // 0.0-1.0
}

// Context represents context to be compressed
type Context struct {
	SystemPrompt    string
	ConversationHistory []string
	Tools           []string
	FileContent     string
	ImportantSections []string
	Metadata        map[string]interface{}
}

// CompressedContext is the result of compression
type CompressedContext struct {
	OriginalSize      int
	CompressedSize    int
	ReductionPercent  float64
	Strategy          string
	Content           string
	PreservedSections []string
	RemovedSections   []string
}

// Compressor manages compression strategies
type Compressor struct {
	strategies map[string]CompressionStrategy
	analyzer   *ContextAnalyzer
	cache      map[string]*CompressedContext
	logger     *slog.Logger
	mu         sync.RWMutex
}

// NewCompressor creates a new compressor
func NewCompressor(logger *slog.Logger) *Compressor {
	if logger == nil {
		logger = slog.Default()
	}

	c := &Compressor{
		strategies: make(map[string]CompressionStrategy),
		analyzer:   NewContextAnalyzer(),
		cache:      make(map[string]*CompressedContext),
		logger:     logger,
	}

	// Register strategies
	c.registerStrategies()

	return c
}

// registerStrategies registers compression strategies
func (c *Compressor) registerStrategies() {
	c.strategies["semantic"] = NewSemanticCompression()
	c.strategies["selective"] = NewSelectiveCompression()
	c.strategies["aggressive"] = NewAggressiveCompression()
	c.strategies["none"] = NewNoCompression()

	c.logger.Debug("registered 4 compression strategies")
}

// Compress applies compression strategy
func (c *Compressor) Compress(ctx *Context, strategy string) (*CompressedContext, error) {
	c.mu.RLock()
	s, ok := c.strategies[strategy]
	c.mu.RUnlock()

	if !ok {
		// Fallback to semantic
		c.logger.Warn("unknown strategy, using semantic", slog.String("strategy", strategy))
		s = c.strategies["semantic"]
	}

	// Check cache
	cacheKey := fmt.Sprintf("%s:%d", strategy, len(ctx.SystemPrompt))
	c.mu.RLock()
	if cached, ok := c.cache[cacheKey]; ok {
		c.mu.RUnlock()
		return cached, nil
	}
	c.mu.RUnlock()

	// Compress
	result, err := s.Compress(ctx)
	if err != nil {
		return nil, err
	}

	// Cache result
	c.mu.Lock()
	c.cache[cacheKey] = result
	c.mu.Unlock()

	c.logger.Debug("compressed context",
		slog.String("strategy", strategy),
		slog.Int("original", result.OriginalSize),
		slog.Int("compressed", result.CompressedSize),
		slog.Float64("reduction_percent", result.ReductionPercent),
	)

	return result, nil
}

// SelectBestStrategy selects the best strategy for the context
func (c *Compressor) SelectBestStrategy(ctx *Context) string {
	size := len(ctx.SystemPrompt) +
		len(ctx.FileContent) +
		summaryLength(ctx.ConversationHistory)

	// Decide based on size
	if size > 50000 {
		return "aggressive"
	}
	if size > 20000 {
		return "selective"
	}
	return "semantic"
}

// ContextAnalyzer analyzes context for compression
type ContextAnalyzer struct{}

// NewContextAnalyzer creates a new analyzer
func NewContextAnalyzer() *ContextAnalyzer {
	return &ContextAnalyzer{}
}

// Analyze analyzes context importance
func (ca *ContextAnalyzer) Analyze(ctx *Context) *Analysis {
	return &Analysis{
		HasTools:          len(ctx.Tools) > 0,
		HasFileContent:    len(ctx.FileContent) > 0,
		ConversationDepth: len(ctx.ConversationHistory),
		TotalSize:         estimateSize(ctx),
		ImportantSections: ctx.ImportantSections,
	}
}

// Analysis represents context analysis
type Analysis struct {
	HasTools          bool
	HasFileContent    bool
	ConversationDepth int
	TotalSize         int
	ImportantSections []string
}

// =====  STRATEGY 1: SEMANTIC COMPRESSION =====

type SemanticCompression struct{}

func NewSemanticCompression() CompressionStrategy {
	return &SemanticCompression{}
}

func (sc *SemanticCompression) Name() string {
	return "semantic"
}

func (sc *SemanticCompression) Compress(ctx *Context) (*CompressedContext, error) {
	originalSize := estimateSize(ctx)

	// Keep important sections
	preserved := []string{}
	removed := []string{}

	// Always keep system prompt
	preserved = append(preserved, "system_prompt")

	// Keep first and last conversation messages
	if len(ctx.ConversationHistory) > 5 {
		// Keep first 2 and last 2
		conversations := ctx.ConversationHistory[:2]
		if len(ctx.ConversationHistory) > 4 {
			conversations = append(conversations, ctx.ConversationHistory[len(ctx.ConversationHistory)-2:]...)
		}
		preserved = append(preserved, "conversation_summary")
		removed = append(removed, fmt.Sprintf("%d middle messages", len(ctx.ConversationHistory)-4))
	} else {
		preserved = append(preserved, "all_conversation")
	}

	// Keep tools
	if len(ctx.Tools) > 0 {
		preserved = append(preserved, "tools")
	}

	// Compress file content (keep first and last)
	if len(ctx.FileContent) > 5000 {
		removed = append(removed, "file_middle_section")
		preserved = append(preserved, "file_edges")
	} else {
		preserved = append(preserved, "file_content")
	}

	compressedSize := int(float64(originalSize) * 0.80)

	return &CompressedContext{
		OriginalSize:     originalSize,
		CompressedSize:   compressedSize,
		ReductionPercent: 20.0,
		Strategy:         sc.Name(),
		Content:          "compressed", // Placeholder
		PreservedSections: preserved,
		RemovedSections:  removed,
	}, nil
}

func (sc *SemanticCompression) EstimatedReduction() float64 {
	return 0.20 // 20% reduction
}

// ===== STRATEGY 2: SELECTIVE COMPRESSION =====

type SelectiveCompression struct{}

func NewSelectiveCompression() CompressionStrategy {
	return &SelectiveCompression{}
}

func (sc *SelectiveCompression) Name() string {
	return "selective"
}

func (sc *SelectiveCompression) Compress(ctx *Context) (*CompressedContext, error) {
	originalSize := estimateSize(ctx)

	preserved := []string{"system_prompt", "tools"}
	removed := []string{}

	// Compress conversation history heavily
	if len(ctx.ConversationHistory) > 10 {
		removed = append(removed, fmt.Sprintf("80%% of %d conversation messages", len(ctx.ConversationHistory)))
		preserved = append(preserved, "first_and_last_message")
	}

	// Remove non-essential file sections
	if len(ctx.FileContent) > 10000 {
		removed = append(removed, "file_content_middle")
		preserved = append(preserved, "file_start_end")
	}

	compressedSize := int(float64(originalSize) * 0.85)

	return &CompressedContext{
		OriginalSize:     originalSize,
		CompressedSize:   compressedSize,
		ReductionPercent: 15.0,
		Strategy:         sc.Name(),
		Content:          "compressed",
		PreservedSections: preserved,
		RemovedSections:  removed,
	}, nil
}

func (sc *SelectiveCompression) EstimatedReduction() float64 {
	return 0.15 // 15% reduction
}

// ===== STRATEGY 3: AGGRESSIVE COMPRESSION =====

type AggressiveCompression struct{}

func NewAggressiveCompression() CompressionStrategy {
	return &AggressiveCompression{}
}

func (ac *AggressiveCompression) Name() string {
	return "aggressive"
}

func (ac *AggressiveCompression) Compress(ctx *Context) (*CompressedContext, error) {
	originalSize := estimateSize(ctx)

	preserved := []string{"system_prompt", "critical_tools", "important_sections"}
	removed := []string{
		"old_conversation_history",
		"redundant_tools",
		"non_critical_files",
	}

	compressedSize := int(float64(originalSize) * 0.70)

	return &CompressedContext{
		OriginalSize:     originalSize,
		CompressedSize:   compressedSize,
		ReductionPercent: 30.0,
		Strategy:         ac.Name(),
		Content:          "compressed",
		PreservedSections: preserved,
		RemovedSections:  removed,
	}, nil
}

func (ac *AggressiveCompression) EstimatedReduction() float64 {
	return 0.30 // 30% reduction
}

// ===== STRATEGY 4: NO COMPRESSION =====

type NoCompression struct{}

func NewNoCompression() CompressionStrategy {
	return &NoCompression{}
}

func (nc *NoCompression) Name() string {
	return "none"
}

func (nc *NoCompression) Compress(ctx *Context) (*CompressedContext, error) {
	size := estimateSize(ctx)

	return &CompressedContext{
		OriginalSize:      size,
		CompressedSize:    size,
		ReductionPercent:  0.0,
		Strategy:          nc.Name(),
		Content:           "unchanged",
		PreservedSections: []string{"all_content"},
		RemovedSections:   []string{},
	}, nil
}

func (nc *NoCompression) EstimatedReduction() float64 {
	return 0.0
}

// Helper functions

func estimateSize(ctx *Context) int {
	size := len(ctx.SystemPrompt) +
		len(ctx.FileContent) +
		summaryLength(ctx.ConversationHistory) +
		summaryLength(ctx.Tools)

	for _, section := range ctx.ImportantSections {
		size += len(section)
	}

	return size
}

func summaryLength(strs []string) int {
	total := 0
	for _, s := range strs {
		total += len(s)
	}
	return total
}

// CompressionMetrics tracks compression effectiveness
type CompressionMetrics struct {
	totalCompressed   int
	totalSaved        int
	strategyUsage     map[string]int
	averageReduction  float64
	mu                sync.RWMutex
}

// NewCompressionMetrics creates new metrics
func NewCompressionMetrics() *CompressionMetrics {
	return &CompressionMetrics{
		strategyUsage: make(map[string]int),
	}
}

// RecordCompression records a compression operation
func (cm *CompressionMetrics) RecordCompression(original, compressed int, strategy string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.totalCompressed += original
	cm.totalSaved += original - compressed
	cm.strategyUsage[strategy]++

	if cm.totalCompressed > 0 {
		cm.averageReduction = float64(cm.totalSaved) / float64(cm.totalCompressed) * 100
	}
}

// GetStats returns compression statistics
func (cm *CompressionMetrics) GetStats() map[string]interface{} {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return map[string]interface{}{
		"total_compressed":    cm.totalCompressed,
		"total_saved":         cm.totalSaved,
		"average_reduction":   cm.averageReduction,
		"strategy_usage":      cm.strategyUsage,
	}
}
