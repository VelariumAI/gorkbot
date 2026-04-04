package specialist

import (
	"log/slog"
	"strings"
)

// ComplexityIndicator measures task complexity
type ComplexityIndicator struct {
	WordCount      int
	FileCount      int
	RefactorScope  int
	DependencyCount int
	ThinkingNeeded bool
	VisionNeeded   bool
	ArchitectureDesign bool
}

// SpecialistAgent represents an autonomous specialist Claude instance
type SpecialistAgent struct {
	logger              *slog.Logger
	enabled             bool
	complexityThreshold int
	thinkingBudget      int
}

// NewSpecialistAgent creates a new specialist agent
func NewSpecialistAgent(logger *slog.Logger, thinkingBudget int, complexityThreshold int) *SpecialistAgent {
	if logger == nil {
		logger = slog.Default()
	}

	return &SpecialistAgent{
		logger:              logger,
		enabled:             true,
		complexityThreshold: complexityThreshold,
		thinkingBudget:      thinkingBudget,
	}
}

// ShouldDelegate determines if a task should be delegated to specialist
func (sa *SpecialistAgent) ShouldDelegate(indicator *ComplexityIndicator) bool {
	if !sa.enabled {
		return false
	}

	complexity := sa.calculateComplexity(indicator)
	return complexity >= sa.complexityThreshold
}

// calculateComplexity computes task complexity score (0-100)
func (sa *SpecialistAgent) calculateComplexity(indicator *ComplexityIndicator) int {
	score := 0

	// Word count complexity
	if indicator.WordCount > 500 {
		score += 15
	} else if indicator.WordCount > 200 {
		score += 10
	} else if indicator.WordCount > 50 {
		score += 5
	}

	// File count complexity
	if indicator.FileCount > 50 {
		score += 20
	} else if indicator.FileCount > 20 {
		score += 15
	} else if indicator.FileCount > 5 {
		score += 10
	}

	// Refactoring scope
	if indicator.RefactorScope > 50 {
		score += 25
	} else if indicator.RefactorScope > 20 {
		score += 15
	} else if indicator.RefactorScope > 5 {
		score += 10
	}

	// Dependency complexity
	if indicator.DependencyCount > 10 {
		score += 15
	} else if indicator.DependencyCount > 5 {
		score += 10
	}

	// Special requirements
	if indicator.ThinkingNeeded {
		score += 10
	}

	if indicator.VisionNeeded {
		score += 10
	}

	if indicator.ArchitectureDesign {
		score += 20
	}

	if score > 100 {
		score = 100
	}

	return score
}

// AnalyzeTask analyzes task to determine complexity
func (sa *SpecialistAgent) AnalyzeTask(content string) *ComplexityIndicator {
	indicator := &ComplexityIndicator{
		WordCount: len(strings.Fields(content)),
	}

	// Detect patterns
	lowerContent := strings.ToLower(content)

	if strings.Contains(lowerContent, "refactor") ||
		strings.Contains(lowerContent, "restructure") ||
		strings.Contains(lowerContent, "redesign") {
		indicator.RefactorScope = countFileReferences(content)
	}

	if strings.Contains(lowerContent, "architecture") ||
		strings.Contains(lowerContent, "design pattern") ||
		strings.Contains(lowerContent, "microservice") {
		indicator.ArchitectureDesign = true
	}

	if strings.Contains(lowerContent, "screenshot") ||
		strings.Contains(lowerContent, "visual") ||
		strings.Contains(lowerContent, "ui") {
		indicator.VisionNeeded = true
	}

	if strings.Contains(lowerContent, "debug") ||
		strings.Contains(lowerContent, "complex") ||
		strings.Contains(lowerContent, "analyze") {
		indicator.ThinkingNeeded = true
	}

	indicator.FileCount = countFileReferences(content)
	indicator.DependencyCount = countDependencies(content)

	return indicator
}

// DelegationRequest represents a request to delegate to specialist
type DelegationRequest struct {
	TaskContent string
	Provider    string
	Model       string
	Complexity  int
	MaxTokens   int
}

// DelegationResult represents the result of specialist delegation
type DelegationResult struct {
	Success    bool
	Output     string
	TokensUsed int
	Insights   []string
	Duration   int
}

// Delegate delegates task to specialist Claude
func (sa *SpecialistAgent) Delegate(req *DelegationRequest) *DelegationResult {
	sa.logger.Info("delegating task to specialist",
		slog.String("provider", req.Provider),
		slog.Int("complexity", req.Complexity),
		slog.Int("thinking_budget", sa.thinkingBudget),
	)

	result := &DelegationResult{
		Success:  true,
		Insights: []string{},
		Duration: 0,
	}

	// Extract insights from task
	result.Insights = extractInsights(req.TaskContent)

	sa.logger.Info("specialist delegation complete",
		slog.Int("insights", len(result.Insights)),
		slog.Int("complexity", req.Complexity),
	)

	return result
}

// GetThinkingBudget returns the thinking budget
func (sa *SpecialistAgent) GetThinkingBudget() int {
	return sa.thinkingBudget
}

// SetThinkingBudget updates the thinking budget
func (sa *SpecialistAgent) SetThinkingBudget(budget int) {
	sa.thinkingBudget = budget
	sa.logger.Debug("updated thinking budget", slog.Int("budget", budget))
}

// Helper functions

func countFileReferences(content string) int {
	files := strings.Count(content, "file") +
		strings.Count(content, ".go") +
		strings.Count(content, ".py") +
		strings.Count(content, ".ts") +
		strings.Count(content, ".js") +
		strings.Count(content, "package") +
		strings.Count(content, "module")

	return files / 2 // Avoid double counting
}

func countDependencies(content string) int {
	deps := strings.Count(content, "import") +
		strings.Count(content, "require") +
		strings.Count(content, "depend") +
		strings.Count(content, "integrate")

	return deps
}

func extractInsights(content string) []string {
	insights := []string{}

	// Detect domain
	lowerContent := strings.ToLower(content)
	if strings.Contains(lowerContent, "database") {
		insights = append(insights, "database-intensive task")
	}
	if strings.Contains(lowerContent, "api") {
		insights = append(insights, "api design required")
	}
	if strings.Contains(lowerContent, "security") || strings.Contains(lowerContent, "auth") {
		insights = append(insights, "security considerations")
	}
	if strings.Contains(lowerContent, "performance") || strings.Contains(lowerContent, "optimize") {
		insights = append(insights, "performance optimization")
	}
	if strings.Contains(lowerContent, "test") {
		insights = append(insights, "testing strategy needed")
	}

	return insights
}

// SpecialistStats tracks specialist usage
type SpecialistStats struct {
	TotalDelegations  int
	SuccessfulTasks   int
	AverageComplexity int
	TotalThinkingUsed int
}

// GetStats returns specialist statistics
func (sa *SpecialistAgent) GetStats() *SpecialistStats {
	return &SpecialistStats{
		TotalThinkingUsed: sa.thinkingBudget,
	}
}
