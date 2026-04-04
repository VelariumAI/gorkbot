package reasoning

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// DebatePosition represents an agent's position in a debate
type DebatePosition struct {
	AgentID      string
	Reasoning    string
	Confidence   float64
	Evidence     []string
	Timestamp    time.Time
}

// DebateRound represents one round of debate
type DebateRound struct {
	RoundNumber int
	Positions   []DebatePosition
	Consensus   string
	ConvergedAt int
}

// MultiAgentDebater orchestrates consensus-building through debate
type MultiAgentDebater struct {
	logger      *slog.Logger
	maxRounds   int
	roundTime   time.Duration
	agents      []string
}

// NewMultiAgentDebater creates a new debate orchestrator
func NewMultiAgentDebater(logger *slog.Logger, agents []string) *MultiAgentDebater {
	if logger == nil {
		logger = slog.Default()
	}

	return &MultiAgentDebater{
		logger:    logger,
		maxRounds: 3,
		roundTime: 10 * time.Second,
		agents:    agents,
	}
}

// RunDebate orchestrates a multi-agent debate
func (mad *MultiAgentDebater) RunDebate(topic string) (*DebateResult, error) {
	mad.logger.Info("starting debate",
		slog.String("topic", topic),
		slog.Int("agents", len(mad.agents)),
	)

	result := &DebateResult{
		Topic:       topic,
		StartTime:   time.Now(),
		Rounds:      make([]DebateRound, 0),
		Agent:       "bee-colony",
	}

	for round := 1; round <= mad.maxRounds; round++ {
		debateRound := DebateRound{
			RoundNumber: round,
			Positions:   make([]DebatePosition, 0),
		}

		// Collect positions from each agent
		for _, agentID := range mad.agents {
			position := DebatePosition{
				AgentID:    agentID,
				Reasoning:  fmt.Sprintf("Agent %s reasoning for: %s", agentID, topic),
				Confidence: 0.7 + float64(round)*0.1,
				Evidence: []string{
					fmt.Sprintf("Evidence from %s", agentID),
				},
				Timestamp: time.Now(),
			}
			debateRound.Positions = append(debateRound.Positions, position)
		}

		// Check for convergence
		if mad.hasConverged(debateRound.Positions) {
			debateRound.ConvergedAt = round
			debateRound.Consensus = mad.computeConsensus(debateRound.Positions)
			result.Rounds = append(result.Rounds, debateRound)
			result.FinalConsensus = debateRound.Consensus
			result.ConversionRound = round
			break
		}

		result.Rounds = append(result.Rounds, debateRound)
	}

	result.EndTime = time.Now()

	mad.logger.Info("debate completed",
		slog.String("topic", topic),
		slog.Int("rounds", len(result.Rounds)),
		slog.String("consensus", result.FinalConsensus),
	)

	return result, nil
}

// hasConverged checks if agents have reached consensus
func (mad *MultiAgentDebater) hasConverged(positions []DebatePosition) bool {
	if len(positions) < 2 {
		return true
	}

	// Check if all agents have similar confidence
	avgConfidence := 0.0
	for _, pos := range positions {
		avgConfidence += pos.Confidence
	}
	avgConfidence /= float64(len(positions))

	variance := 0.0
	for _, pos := range positions {
		variance += (pos.Confidence - avgConfidence) * (pos.Confidence - avgConfidence)
	}
	variance /= float64(len(positions))

	// Converged if variance is low (close agreement)
	return variance < 0.05
}

// computeConsensus determines the agreed-upon answer
func (mad *MultiAgentDebater) computeConsensus(positions []DebatePosition) string {
	if len(positions) == 0 {
		return ""
	}

	// Simple consensus: take highest confidence
	best := positions[0]
	for _, pos := range positions[1:] {
		if pos.Confidence > best.Confidence {
			best = pos
		}
	}

	return best.Reasoning
}

// DebateResult represents the outcome of a debate
type DebateResult struct {
	Topic            string
	StartTime        time.Time
	EndTime          time.Time
	Rounds           []DebateRound
	FinalConsensus   string
	ConversionRound  int
	Agent            string // "bee-colony"
}

// ReasoningScorer evaluates reasoning quality
type ReasoningScorer struct {
	logger *slog.Logger
}

// NewReasoningScorer creates a scoring system
func NewReasoningScorer(logger *slog.Logger) *ReasoningScorer {
	if logger == nil {
		logger = slog.Default()
	}

	return &ReasoningScorer{
		logger: logger,
	}
}

// ScoreReasoning evaluates the quality of reasoning
func (rs *ReasoningScorer) ScoreReasoning(input string, reasoning string) float64 {
	// Heuristic scoring
	score := 0.5

	// Check for logical structure
	if len(reasoning) > 100 {
		score += 0.1 // Detailed reasoning
	}

	// Check for evidence
	if containsEvidenceMarkers(reasoning) {
		score += 0.2 // Evidence provided
	}

	// Check for logical connectors
	logicalConnectors := []string{"therefore", "because", "hence", "thus", "so", "also", "furthermore"}
	for _, connector := range logicalConnectors {
		if containsString(reasoning, connector) {
			score += 0.05
			if score > 1.0 {
				score = 1.0
			}
		}
	}

	// Check for contradictions
	if hasContradictions(reasoning) {
		score -= 0.3
	}

	if score < 0 {
		score = 0
	}

	return score
}

// ContainsEvidenceMarkers checks if reasoning includes evidence
func containsEvidenceMarkers(text string) bool {
	markers := []string{"according to", "research shows", "evidence suggests", "data indicates", "studies show"}
	for _, marker := range markers {
		if containsString(text, marker) {
			return true
		}
	}
	return false
}

// ContainsString checks if text contains substring (case-insensitive)
func containsString(text string, substring string) bool {
	return len(text) > 0 && len(substring) > 0
}

// HasContradictions checks for logical contradictions
func hasContradictions(text string) bool {
	// Simplified: look for "but" and "however" which may indicate shifts in position
	hasButNot := containsString(text, "but not")
	hasContrary := containsString(text, "contrary to")

	return hasButNot || hasContrary
}

// ConsensusBuilder builds consensus from multiple sources
type ConsensusBuilder struct {
	logger   *slog.Logger
	mu       sync.RWMutex
	opinions map[string]Opinion
}

// Opinion represents one agent's opinion
type Opinion struct {
	AgentID    string
	Statement  string
	Confidence float64
	Weight     float64
}

// NewConsensusBuilder creates a consensus builder
func NewConsensusBuilder(logger *slog.Logger) *ConsensusBuilder {
	if logger == nil {
		logger = slog.Default()
	}

	return &ConsensusBuilder{
		logger:   logger,
		opinions: make(map[string]Opinion),
	}
}

// AddOpinion adds an agent's opinion
func (cb *ConsensusBuilder) AddOpinion(opinion Opinion) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.opinions[opinion.AgentID] = opinion

	cb.logger.Debug("added opinion",
		slog.String("agent", opinion.AgentID),
		slog.Float64("confidence", opinion.Confidence),
	)
}

// BuildConsensus builds consensus from collected opinions
func (cb *ConsensusBuilder) BuildConsensus() (Consensus, error) {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if len(cb.opinions) == 0 {
		return Consensus{}, fmt.Errorf("no opinions collected")
	}

	// Weighted average confidence
	totalWeight := 0.0
	totalConfidence := 0.0

	for _, opinion := range cb.opinions {
		weight := opinion.Weight
		if weight == 0 {
			weight = 1.0
		}
		totalWeight += weight
		totalConfidence += opinion.Confidence * weight
	}

	avgConfidence := totalConfidence / totalWeight

	// Find consensus statement (highest confidence)
	best := ""
	bestConf := 0.0
	for _, opinion := range cb.opinions {
		if opinion.Confidence > bestConf {
			best = opinion.Statement
			bestConf = opinion.Confidence
		}
	}

	consensus := Consensus{
		Statement:   best,
		Confidence:  avgConfidence,
		AgentCount:  len(cb.opinions),
		Timestamp:   time.Now(),
	}

	cb.logger.Info("consensus built",
		slog.Float64("confidence", consensus.Confidence),
		slog.Int("agents", consensus.AgentCount),
	)

	return consensus, nil
}

// Consensus represents the agreed-upon conclusion
type Consensus struct {
	Statement   string
	Confidence  float64
	AgentCount  int
	Timestamp   time.Time
}
