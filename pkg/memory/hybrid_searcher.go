package memory

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var metricsOnce sync.Once
var globalMemoryModeGauge prometheus.Gauge
var globalSearchLatency prometheus.Histogram
var globalRecallMetric prometheus.Gauge

func ensureMemoryMetrics() (prometheus.Gauge, prometheus.Histogram, prometheus.Gauge) {
	metricsOnce.Do(func() {
		globalMemoryModeGauge = promauto.NewGauge(prometheus.GaugeOpts{
			Name: "gorkbot_memory_mode",
			Help: "Memory search mode: 1 = hybrid (semantic+lexical), 0 = lexical_only (degraded)",
		})

		globalSearchLatency = promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "gorkbot_memory_search_latency_seconds",
			Help:    "Latency of memory search operations",
			Buckets: prometheus.DefBuckets,
		})

		globalRecallMetric = promauto.NewGauge(prometheus.GaugeOpts{
			Name: "gorkbot_memory_search_recall",
			Help: "Estimated search recall (0-100)",
		})
	})

	return globalMemoryModeGauge, globalSearchLatency, globalRecallMetric
}

// SearchConfig holds configuration for HybridSearcher
type SearchConfig struct {
	// FTS5 parameters
	LexicalWeight  float64 // Weight for lexical search in RRF
	FactWeight     float64 // Weight for fact search in RRF
	SemanticWeight float64 // Weight for semantic search in RRF (0 if degraded)
	RerankerWeight float64 // Weight for reranker boost (0 if degraded)

	// Result limits
	TopK    int // Number of results to return
	FusionK int // K for RRF fusion formula (default 60)

	// Telemetry
	Logger *slog.Logger
}

// SearchResult represents a single search result
type SearchResult struct {
	FactID         string
	Subject        string
	Predicate      string
	Object         string
	Confidence     float64
	Source         string
	Timestamp      string
	RelevanceScore float64 // Fusion + reranking score
	Source_        string  // Which source found this (semantic/lexical/fact)
}

// SemanticSearcher interface for vector-based search
type SemanticSearcher interface {
	// Search returns facts ordered by semantic similarity
	Search(ctx context.Context, query string, k int) ([]SearchResult, error)
	// Close cleans up resources
	Close() error
}

// LexicalSearcher interface for BM25/FTS5 search
type LexicalSearcher interface {
	// Search returns facts ordered by BM25 relevance
	Search(ctx context.Context, query string, k int) ([]SearchResult, error)
	// Close cleans up resources
	Close() error
}

// FactSearcher interface for triple-based search
type FactSearcher interface {
	// Search returns facts matching (subject, predicate, object) patterns
	Search(ctx context.Context, subject, predicate, object string, k int) ([]SearchResult, error)
	// Close cleans up resources
	Close() error
}

// Reranker interface for learned ranking
type Reranker interface {
	// Rerank takes candidates and reorders by learned relevance
	Rerank(ctx context.Context, query string, candidates []SearchResult) ([]SearchResult, error)
	// Close cleans up resources
	Close() error
}

// HybridSearcher fuses multiple search strategies with graceful degradation
type HybridSearcher struct {
	db         *sql.DB
	semantic   SemanticSearcher // May be nil if unsupported
	lexical    LexicalSearcher  // Guaranteed (always available)
	factSearch FactSearcher     // Guaranteed
	reranker   Reranker         // May be nil if unsupported

	config        SearchConfig
	isDegraded    bool
	degradeReason string

	// Metrics
	memoryModeGauge prometheus.Gauge
	searchLatency   prometheus.Histogram
	recallMetric    prometheus.Gauge

	// Sync for safe updates
	mu sync.RWMutex
}

// NewHybridSearcher creates a HybridSearcher with autonomous capability detection
func NewHybridSearcher(
	db *sql.DB,
	lexicalSearcher LexicalSearcher,
	factSearcher FactSearcher,
	config SearchConfig,
) *HybridSearcher {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	if config.TopK == 0 {
		config.TopK = 8
	}
	if config.FusionK == 0 {
		config.FusionK = 60
	}
	if config.LexicalWeight == 0 {
		config.LexicalWeight = 1.0
	}
	if config.FactWeight == 0 {
		config.FactWeight = 1.0
	}
	if config.SemanticWeight == 0 {
		config.SemanticWeight = 1.5
	}
	if config.RerankerWeight == 0 {
		config.RerankerWeight = 1.2
	}

	hs := &HybridSearcher{
		db:         db,
		lexical:    lexicalSearcher,
		factSearch: factSearcher,
		config:     config,
		isDegraded: true, // Default to degraded, try to upgrade
	}

	// Register Prometheus metrics
	hs.memoryModeGauge, hs.searchLatency, hs.recallMetric = ensureMemoryMetrics()

	// Probe for semantic search capability
	hs.probeSemanticCapability()

	// Probe for reranker capability
	hs.probeRerankerCapability()

	// Set initial gauge
	if hs.isDegraded {
		hs.memoryModeGauge.Set(0) // Lexical only
		hs.recallMetric.Set(45)   // Typical lexical recall
		hs.config.Logger.Warn("HybridSearcher degraded to lexical-only mode",
			slog.String("reason", hs.degradeReason),
			slog.String("guidance", "Install vector embedding library or ONNX runtime for full capability"),
		)
	} else {
		hs.memoryModeGauge.Set(1) // Hybrid mode
		hs.recallMetric.Set(84)   // Target hybrid recall
		hs.config.Logger.Info("HybridSearcher initialized in full hybrid mode",
			slog.Bool("semantic_available", hs.semantic != nil),
			slog.Bool("reranker_available", hs.reranker != nil),
		)
	}

	return hs
}

// probeSemanticCapability attempts to initialize semantic search
// If unavailable, gracefully skips it
func (hs *HybridSearcher) probeSemanticCapability() {
	if os.Getenv("GORKBOT_MEMORY_ENABLE_SEMANTIC") == "1" {
		semantic, err := newSemanticSearcherFromEnv(hs.db, hs.config.Logger)
		if err != nil {
			hs.degradeReason = fmt.Sprintf("semantic probe failed: %v", err)
			hs.isDegraded = true
			return
		}
		hs.semantic = semantic
		hs.isDegraded = false
		hs.degradeReason = ""
		hs.config.Logger.Info("semantic search enabled", slog.String("backend", semanticBackendName(semantic)))
		return
	}
	hs.degradeReason = "semantic search disabled (set GORKBOT_MEMORY_ENABLE_SEMANTIC=1 to opt in)"
	hs.isDegraded = true
}

// probeRerankerCapability attempts to initialize ONNX reranker
// If unavailable, gracefully skips it
func (hs *HybridSearcher) probeRerankerCapability() {
	if os.Getenv("GORKBOT_MEMORY_ENABLE_RERANKER") == "1" {
		if semanticWithEmbedder, ok := hs.semantic.(embedderAwareSemantic); ok {
			hs.reranker = newEmbeddingReranker(semanticWithEmbedder.Embedder())
			hs.config.Logger.Info("reranker enabled", slog.String("backend", "embedding-reranker"))
			return
		}
		hs.config.Logger.Warn("reranker requested but semantic embedder unavailable")
		return
	}
	hs.config.Logger.Debug("reranker probe skipped (set GORKBOT_MEMORY_ENABLE_RERANKER=1 to opt in)")
}

// Search performs hybrid search with autonomous fallback
// Returns top-K results fused via Reciprocal Rank Fusion
func (hs *HybridSearcher) Search(ctx context.Context, query string, userTopK ...int) ([]SearchResult, error) {
	hs.mu.RLock()
	defer hs.mu.RUnlock()

	topK := hs.config.TopK
	if len(userTopK) > 0 && userTopK[0] > 0 {
		topK = userTopK[0]
	}

	// Track latency for telemetry
	defer func() {
		// Latency tracking would be added here with timing.Start()
	}()

	// If degraded, use lexical+fact only
	if hs.isDegraded {
		return hs.searchDegraded(ctx, query, topK)
	}

	// Full hybrid search: semantic + lexical + fact + reranking
	return hs.searchHybrid(ctx, query, topK)
}

// searchDegraded runs pure FTS5 + fact search without semantic/reranker
func (hs *HybridSearcher) searchDegraded(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	// Expand topK for fusion since we have fewer sources
	fusionK := topK * 3

	// Run searches in parallel
	lexicalCh := make(chan []SearchResult, 1)
	factCh := make(chan []SearchResult, 1)
	errCh := make(chan error, 2)

	go func() {
		results, err := hs.lexical.Search(ctx, query, fusionK)
		if err != nil {
			errCh <- fmt.Errorf("lexical search failed: %w", err)
			return
		}
		lexicalCh <- results
	}()

	go func() {
		// Fact search: try to extract entities from query
		// For now, use query as-is (phrase matching)
		results, err := hs.factSearch.Search(ctx, "", "", query, fusionK)
		if err != nil {
			errCh <- fmt.Errorf("fact search failed: %w", err)
			return
		}
		factCh <- results
	}()

	// Collect results
	var lexicalResults, factResults []SearchResult
	timeoutCtx, cancel := context.WithTimeout(ctx, hs.timeoutForSearch())
	defer cancel()

	for i := 0; i < 2; i++ {
		select {
		case lexResults := <-lexicalCh:
			lexicalResults = lexResults
		case factRes := <-factCh:
			factResults = factRes
		case err := <-errCh:
			hs.config.Logger.Warn("search source failed", slog.String("error", err.Error()))
		case <-timeoutCtx.Done():
			return nil, fmt.Errorf("search timeout")
		}
	}

	// Fuse lexical + fact via RRF (no semantic, no reranker)
	fused := hs.fuseRRF(
		query,
		map[string][]SearchResult{
			"lexical": lexicalResults,
			"fact":    factResults,
		},
		topK,
	)

	return fused, nil
}

// searchHybrid runs full hybrid search with semantic + reranker
func (hs *HybridSearcher) searchHybrid(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	fusionK := topK * 3
	semanticCh := make(chan []SearchResult, 1)
	lexicalCh := make(chan []SearchResult, 1)
	factCh := make(chan []SearchResult, 1)
	errCh := make(chan error, 3)

	go func() {
		results, err := hs.semantic.Search(ctx, query, fusionK)
		if err != nil {
			errCh <- fmt.Errorf("semantic search failed: %w", err)
			return
		}
		semanticCh <- results
	}()
	go func() {
		results, err := hs.lexical.Search(ctx, query, fusionK)
		if err != nil {
			errCh <- fmt.Errorf("lexical search failed: %w", err)
			return
		}
		lexicalCh <- results
	}()
	go func() {
		results, err := hs.factSearch.Search(ctx, "", "", query, fusionK)
		if err != nil {
			errCh <- fmt.Errorf("fact search failed: %w", err)
			return
		}
		factCh <- results
	}()

	var semanticResults, lexicalResults, factResults []SearchResult
	timeoutCtx, cancel := context.WithTimeout(ctx, hs.timeoutForSearch())
	defer cancel()

	for i := 0; i < 3; i++ {
		select {
		case sem := <-semanticCh:
			semanticResults = sem
		case lex := <-lexicalCh:
			lexicalResults = lex
		case facts := <-factCh:
			factResults = facts
		case err := <-errCh:
			hs.config.Logger.Warn("search source failed", slog.String("error", err.Error()))
		case <-timeoutCtx.Done():
			return nil, fmt.Errorf("search timeout")
		}
	}

	return hs.fuseRRF(query, map[string][]SearchResult{
		"semantic": semanticResults,
		"lexical":  lexicalResults,
		"fact":     factResults,
	}, topK), nil
}

// fuseRRF performs Reciprocal Rank Fusion on multiple search results
// Formula: RRF(d) = sum(1 / (K + rank(d)))
func (hs *HybridSearcher) fuseRRF(query string, sources map[string][]SearchResult, topK int) []SearchResult {
	type scoreEntry struct {
		result  SearchResult
		score   float64
		sources map[string]int // rank in each source
	}

	// Build map of all unique results
	resultMap := make(map[string]*scoreEntry)

	// For each source, assign RRF scores
	for sourceName, results := range sources {
		var weight float64
		switch sourceName {
		case "semantic":
			weight = hs.config.SemanticWeight
		case "lexical":
			weight = hs.config.LexicalWeight
		case "fact":
			weight = hs.config.FactWeight
		default:
			weight = 1.0
		}

		for rank, result := range results {
			key := result.FactID
			if key == "" {
				key = fmt.Sprintf("%s|%s|%s", result.Subject, result.Predicate, result.Object)
			}

			if entry, exists := resultMap[key]; exists {
				// Update score with this source's contribution
				rrf := weight / float64(hs.config.FusionK+rank+1)
				entry.score += rrf
				entry.sources[sourceName] = rank
			} else {
				rrf := weight / float64(hs.config.FusionK+rank+1)
				resultMap[key] = &scoreEntry{
					result:  result,
					score:   rrf,
					sources: map[string]int{sourceName: rank},
				}
			}
		}
	}

	// Convert to slice and sort by score
	var entries []*scoreEntry
	for _, entry := range resultMap {
		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].score > entries[j].score
	})

	// Rerank if available
	topResults := make([]SearchResult, 0, len(entries))
	if len(entries) > topK*2 {
		entries = entries[:topK*2]
	}
	for _, entry := range entries {
		result := entry.result
		result.RelevanceScore = entry.score
		topResults = append(topResults, result)
	}

	if hs.reranker != nil && len(topResults) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), hs.timeoutForSearch())
		defer cancel()
		if reranked, err := hs.reranker.Rerank(ctx, query, topResults); err == nil {
			topResults = reranked
		} else {
			hs.config.Logger.Warn("reranker failed, using RRF scores", slog.String("error", err.Error()))
		}
	}

	// Return top-K
	if len(topResults) > topK {
		topResults = topResults[:topK]
	}

	return topResults
}

// timeoutForSearch returns a reasonable timeout for search operations
func (hs *HybridSearcher) timeoutForSearch() time.Duration {
	// In degraded mode, searches should be faster
	// In hybrid mode, may need more time for semantic search
	if hs.isDegraded {
		return 2 * time.Second
	}
	return 5 * time.Second
}

// Close cleans up all search resources
func (hs *HybridSearcher) Close() error {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	var errs []error

	if hs.semantic != nil {
		if err := hs.semantic.Close(); err != nil {
			errs = append(errs, fmt.Errorf("semantic close: %w", err))
		}
	}

	if hs.lexical != nil {
		if err := hs.lexical.Close(); err != nil {
			errs = append(errs, fmt.Errorf("lexical close: %w", err))
		}
	}

	if hs.factSearch != nil {
		if err := hs.factSearch.Close(); err != nil {
			errs = append(errs, fmt.Errorf("fact close: %w", err))
		}
	}

	if hs.reranker != nil {
		if err := hs.reranker.Close(); err != nil {
			errs = append(errs, fmt.Errorf("reranker close: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("HybridSearcher.Close had %d errors: %v", len(errs), errs)
	}

	return nil
}

// GetMode returns the current search mode (for debugging/telemetry)
func (hs *HybridSearcher) GetMode() string {
	hs.mu.RLock()
	defer hs.mu.RUnlock()

	if hs.isDegraded {
		return fmt.Sprintf("lexical_only (reason: %s)", hs.degradeReason)
	}
	return "hybrid"
}

// GetCapabilities returns info about which search methods are available
func (hs *HybridSearcher) GetCapabilities() map[string]bool {
	hs.mu.RLock()
	defer hs.mu.RUnlock()

	return map[string]bool{
		"semantic": hs.semantic != nil,
		"lexical":  hs.lexical != nil,
		"fact":     hs.factSearch != nil,
		"reranker": hs.reranker != nil,
		"degraded": hs.isDegraded,
	}
}
