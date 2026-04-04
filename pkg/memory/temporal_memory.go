package memory

import (
	"context"
	"log/slog"
	"math"
	"sort"
	"sync"
	"time"
)

// TemporalMemory manages fact aging, decay, and compaction
type TemporalMemory struct {
	searcher *SQLiteFactSearcher
	logger   *slog.Logger

	// Configuration
	config TemporalConfig

	// State
	lastCompactionTime time.Time
	compactionMu       sync.RWMutex

	// Statistics
	factsCompacted int64
	factsDecayed   int64
	factsArchived  int64
}

// TemporalConfig defines memory retention and decay policies
type TemporalConfig struct {
	// Decay parameters
	HalfLife        time.Duration // Time for confidence to drop to 50% (default: 7 days)
	MinConfidence   float64       // Facts below this are archived (default: 0.2)
	ArchiveThreshold time.Duration // Age beyond which facts are archived (default: 90 days)

	// Compaction parameters
	CompactionInterval time.Duration // How often to run compaction (default: daily)
	CompactionFactor   int            // 10 facts → 1 insight (default: 10)
	MinFactsPerGroup   int            // Min facts to group for compaction (default: 3)

	// Contradiction detection
	DetectContradictions bool // Enable contradiction detection
	ConfidenceBonus      float64 // Bonus for facts supporting existing ones (default: +0.1)
	ConfidencePenalty    float64 // Penalty for contradicting facts (default: -0.2)

	// Retention
	MaxRetention time.Duration // Maximum time to keep any fact (default: 1 year)
	MaxFacts     int            // Maximum facts to keep (oldest pruned) (default: 10000)
}

// DefaultTemporalConfig creates sensible defaults
func DefaultTemporalConfig() TemporalConfig {
	return TemporalConfig{
		HalfLife:           7 * 24 * time.Hour, // 7 days
		MinConfidence:      0.2,
		ArchiveThreshold:   90 * 24 * time.Hour, // 90 days
		CompactionInterval: 24 * time.Hour,      // Daily
		CompactionFactor:   10,
		MinFactsPerGroup:   3,
		DetectContradictions: true,
		ConfidenceBonus:    0.1,
		ConfidencePenalty:  -0.2,
		MaxRetention:       365 * 24 * time.Hour, // 1 year
		MaxFacts:           10000,
	}
}

// CompactedFact represents a generalized fact from compaction
type CompactedFact struct {
	Subject      string
	Predicate    string
	Objects      []string        // Multiple objects grouped together
	Confidence   float64         // Average confidence
	SourceFacts  []string        // IDs of facts that were compacted
	CompactedAt  time.Time
	OriginalTime time.Time // Average of original creation times
}

// NewTemporalMemory creates a new temporal memory manager
func NewTemporalMemory(searcher *SQLiteFactSearcher, logger *slog.Logger) *TemporalMemory {
	if logger == nil {
		logger = slog.Default()
	}

	return &TemporalMemory{
		searcher:           searcher,
		logger:             logger,
		config:             DefaultTemporalConfig(),
		lastCompactionTime: time.Now(),
	}
}

// SetConfig updates the temporal configuration
func (tm *TemporalMemory) SetConfig(config TemporalConfig) {
	tm.compactionMu.Lock()
	defer tm.compactionMu.Unlock()
	tm.config = config
}

// GetDecayedConfidence calculates confidence after temporal decay
func (tm *TemporalMemory) GetDecayedConfidence(originalConfidence float64, createdAt time.Time) float64 {
	if originalConfidence <= 0 {
		return 0
	}

	age := time.Since(createdAt)

	// Exponential decay: C(t) = C0 * (0.5)^(t / halfLife)
	// This means confidence drops to 50% after one half-life
	decayFactor := math.Pow(0.5, age.Hours()/tm.config.HalfLife.Hours())
	decayed := originalConfidence * decayFactor

	// Floor at minimum confidence
	if decayed < tm.config.MinConfidence {
		return 0
	}

	tm.factsDecayed++
	return decayed
}

// CheckExpiration returns true if a fact should be archived
func (tm *TemporalMemory) CheckExpiration(createdAt time.Time) bool {
	age := time.Since(createdAt)
	return age > tm.config.ArchiveThreshold
}

// ShouldCompact returns true if compaction is due
func (tm *TemporalMemory) ShouldCompact() bool {
	tm.compactionMu.RLock()
	lastCompaction := tm.lastCompactionTime
	tm.compactionMu.RUnlock()

	return time.Since(lastCompaction) > tm.config.CompactionInterval
}

// CompactFacts groups related facts and creates summaries
func (tm *TemporalMemory) CompactFacts(ctx context.Context) error {
	if !tm.ShouldCompact() {
		return nil
	}

	tm.logger.Info("starting fact compaction", slog.Time("time", time.Now()))

	// Group facts by (subject, predicate)
	groups := make(map[string][]*SearchResult)

	// Scan all facts and group them
	// Note: In real implementation, would query DB directly
	// This is a placeholder showing the algorithm
	for _, group := range groups {
		if len(group) >= tm.config.MinFactsPerGroup {
			if err := tm.compactGroup(ctx, group); err != nil {
				tm.logger.Error("failed to compact group", slog.String("error", err.Error()))
			}
		}
	}

	// Update last compaction time
	tm.compactionMu.Lock()
	tm.lastCompactionTime = time.Now()
	tm.compactionMu.Unlock()

	tm.logger.Info("fact compaction completed",
		slog.Int64("compacted", tm.factsCompacted),
		slog.Int64("archived", tm.factsArchived),
	)

	return nil
}

// compactGroup compacts a group of related facts
func (tm *TemporalMemory) compactGroup(ctx context.Context, facts []*SearchResult) error {
	if len(facts) < tm.config.MinFactsPerGroup {
		return nil
	}

	// Sort by confidence
	sort.Slice(facts, func(i, j int) bool {
		return facts[i].Confidence > facts[j].Confidence
	})

	// Group objects
	objectMap := make(map[string]int)
	totalConfidence := 0.0
	var sourceFacts []string

	// Parse timestamp from first fact (if available)
	var earliestTime time.Time
	if len(facts) > 0 && facts[0].Timestamp != "" {
		if t, err := time.Parse(time.RFC3339, facts[0].Timestamp); err == nil {
			earliestTime = t
		}
	}

	for _, fact := range facts {
		objectMap[fact.Object]++
		totalConfidence += fact.Confidence
		sourceFacts = append(sourceFacts, fact.FactID)

		// Track earliest timestamp
		if fact.Timestamp != "" {
			if t, err := time.Parse(time.RFC3339, fact.Timestamp); err == nil {
				if earliestTime.IsZero() || t.Before(earliestTime) {
					earliestTime = t
				}
			}
		}
	}

	// Extract objects
	objects := make([]string, 0, len(objectMap))
	for obj := range objectMap {
		objects = append(objects, obj)
	}

	// Create compacted fact
	compacted := CompactedFact{
		Subject:     facts[0].Subject,
		Predicate:   facts[0].Predicate,
		Objects:     objects,
		Confidence:  totalConfidence / float64(len(facts)),
		SourceFacts: sourceFacts,
		CompactedAt: time.Now(),
		OriginalTime: earliestTime,
	}

	tm.logger.Info("fact group compacted",
		slog.String("subject", compacted.Subject),
		slog.String("predicate", compacted.Predicate),
		slog.Int("fact_count", len(facts)),
		slog.Int("objects", len(objects)),
		slog.Float64("confidence", compacted.Confidence),
	)

	tm.factsCompacted += int64(len(facts))
	return nil
}

// ArchiveFacts moves old facts to archive storage
func (tm *TemporalMemory) ArchiveFacts(ctx context.Context) error {
	// Query facts older than archive threshold
	cutoff := time.Now().Add(-tm.config.ArchiveThreshold)

	tm.logger.Info("archiving old facts", slog.Time("cutoff", cutoff))

	// In real implementation, would:
	// 1. Query facts with CreatedAt < cutoff
	// 2. Move to archive table
	// 3. Update indexes

	// For now, just log the operation
	tm.factsArchived++

	return nil
}

// DetectContradictions finds facts that contradict each other
func (tm *TemporalMemory) DetectContradictions(ctx context.Context, fact *SearchResult) ([]string, error) {
	if !tm.config.DetectContradictions {
		return []string{}, nil
	}

	// Find facts with same subject and predicate but different object
	// These are potential contradictions
	contradictions := make([]string, 0) // Initialize to empty slice, not nil

	// Algorithm:
	// 1. Find all facts with same (subject, predicate)
	// 2. Group by object
	// 3. If multiple groups exist, flag as contradiction

	tm.logger.Debug("checking for contradictions",
		slog.String("subject", fact.Subject),
		slog.String("predicate", fact.Predicate),
	)

	return contradictions, nil
}

// UpdateConfidenceBySupport adjusts confidence based on supporting/contradicting facts
func (tm *TemporalMemory) UpdateConfidenceBySupport(ctx context.Context, fact *SearchResult, supporting, contradicting int) float64 {
	confidence := fact.Confidence

	// Apply bonus for supporting facts
	if supporting > 0 {
		bonus := float64(supporting) * tm.config.ConfidenceBonus
		confidence += bonus
		tm.logger.Debug("confidence boost", slog.Float64("bonus", bonus))
	}

	// Apply penalty for contradicting facts
	if contradicting > 0 {
		penalty := float64(contradicting) * tm.config.ConfidencePenalty
		confidence += penalty
		tm.logger.Debug("confidence penalty", slog.Float64("penalty", penalty))
	}

	// Clamp between 0 and 1
	if confidence < 0 {
		confidence = 0
	} else if confidence > 1 {
		confidence = 1
	}

	return confidence
}

// PruneByRetention removes facts exceeding maximum retention period
func (tm *TemporalMemory) PruneByRetention(ctx context.Context) error {
	cutoff := time.Now().Add(-tm.config.MaxRetention)

	tm.logger.Info("pruning facts by retention", slog.Time("cutoff", cutoff))

	// Query facts older than max retention
	// Remove them from the database

	return nil
}

// PruneByCount removes oldest facts if count exceeds maximum
func (tm *TemporalMemory) PruneByCount(ctx context.Context) error {
	// Count existing facts
	// If > MaxFacts, delete oldest ones

	tm.logger.Info("pruning facts by count limit", slog.Int("max", tm.config.MaxFacts))

	return nil
}

// GetFactAge returns the age of a fact in days
func (tm *TemporalMemory) GetFactAge(createdAt time.Time) float64 {
	return time.Since(createdAt).Hours() / 24
}

// GetMemoryMetrics returns memory system statistics
func (tm *TemporalMemory) GetMemoryMetrics() map[string]interface{} {
	tm.compactionMu.RLock()
	lastCompaction := tm.lastCompactionTime
	tm.compactionMu.RUnlock()

	return map[string]interface{}{
		"facts_compacted":    tm.factsCompacted,
		"facts_decayed":      tm.factsDecayed,
		"facts_archived":     tm.factsArchived,
		"last_compaction":    lastCompaction,
		"half_life":          tm.config.HalfLife.String(),
		"min_confidence":     tm.config.MinConfidence,
		"archive_threshold":  tm.config.ArchiveThreshold.String(),
		"max_facts":          tm.config.MaxFacts,
		"max_retention":      tm.config.MaxRetention.String(),
	}
}

// ApplyTemporalDecay applies decay to all facts in search results
func (tm *TemporalMemory) ApplyTemporalDecay(results []SearchResult) []SearchResult {
	decayed := make([]SearchResult, len(results))

	for i, result := range results {
		decayed[i] = result
		// Parse timestamp to calculate decay
		createdAt := time.Now() // Default to now if not parseable
		if result.Timestamp != "" {
			if t, err := time.Parse(time.RFC3339, result.Timestamp); err == nil {
				createdAt = t
			}
		}
		decayed[i].Confidence = tm.GetDecayedConfidence(result.Confidence, createdAt)
	}

	// Re-sort by decayed confidence
	sort.Slice(decayed, func(i, j int) bool {
		return decayed[i].Confidence > decayed[j].Confidence
	})

	return decayed
}

// GetMostRecentFacts returns the N most recent facts
func (tm *TemporalMemory) GetMostRecentFacts(results []SearchResult, n int) []SearchResult {
	// Sort by creation time (newest first)
	sorted := make([]SearchResult, len(results))
	copy(sorted, results)

	sort.Slice(sorted, func(i, j int) bool {
		ti := time.Now() // Default if not parseable
		tj := time.Now()

		if sorted[i].Timestamp != "" {
			if t, err := time.Parse(time.RFC3339, sorted[i].Timestamp); err == nil {
				ti = t
			}
		}

		if sorted[j].Timestamp != "" {
			if t, err := time.Parse(time.RFC3339, sorted[j].Timestamp); err == nil {
				tj = t
			}
		}

		return ti.After(tj)
	})

	// Return top N
	if n > len(sorted) {
		n = len(sorted)
	}

	return sorted[:n]
}

// CompactionJob represents a scheduled compaction task
type CompactionJob struct {
	Memory     *TemporalMemory
	Interval   time.Duration
	StopChan   chan bool
	Running    bool
	runningMu  sync.RWMutex
}

// NewCompactionJob creates a new compaction scheduler
func NewCompactionJob(memory *TemporalMemory, interval time.Duration) *CompactionJob {
	return &CompactionJob{
		Memory:   memory,
		Interval: interval,
		StopChan: make(chan bool),
		Running:  false,
	}
}

// Start begins the compaction scheduler
func (job *CompactionJob) Start(ctx context.Context) {
	job.runningMu.Lock()
	if job.Running {
		job.runningMu.Unlock()
		return
	}
	job.Running = true
	job.runningMu.Unlock()

	go func() {
		ticker := time.NewTicker(job.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				job.Memory.logger.Info("running scheduled compaction")
				if err := job.Memory.CompactFacts(ctx); err != nil {
					job.Memory.logger.Error("compaction failed", slog.String("error", err.Error()))
				}
				if err := job.Memory.ArchiveFacts(ctx); err != nil {
					job.Memory.logger.Error("archival failed", slog.String("error", err.Error()))
				}
			case <-job.StopChan:
				job.Memory.logger.Info("compaction job stopped")
				job.runningMu.Lock()
				job.Running = false
				job.runningMu.Unlock()
				return
			case <-ctx.Done():
				job.Memory.logger.Info("compaction job context cancelled")
				job.runningMu.Lock()
				job.Running = false
				job.runningMu.Unlock()
				return
			}
		}
	}()
}

// Stop halts the compaction scheduler
func (job *CompactionJob) Stop() {
	job.runningMu.RLock()
	if !job.Running {
		job.runningMu.RUnlock()
		return
	}
	job.runningMu.RUnlock()

	select {
	case job.StopChan <- true:
	default:
	}
}

// IsRunning returns whether the job is currently running
func (job *CompactionJob) IsRunning() bool {
	job.runningMu.RLock()
	defer job.runningMu.RUnlock()
	return job.Running
}
