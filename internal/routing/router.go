package routing

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	"github.com/velariumai/gorkbot/internal/config"
	"github.com/velariumai/gorkbot/internal/providers"
)

// CapabilityRouter makes intelligent routing decisions based on task context
// This is THE core decision point for v3.0 architecture
type CapabilityRouter struct {
	config     *config.GorkbotConfig
	factory    *providers.ProviderFactory
	classifier *IntentClassifier
	logger     *slog.Logger
}

// RoutingDecision contains all information needed to execute a task
type RoutingDecision struct {
	// Provider selection
	Provider      providers.AIProvider
	ProviderName  string
	FallbackChain []string

	// Model configuration
	PromptVariant string // GENERIC, NEXTGEN, GPT5, CLAUDE_THINKING, etc.
	Temperature   float32
	MaxTokens     int

	// Thinking configuration
	UseThinking    bool
	ThinkingBudget int

	// Feature activation
	UseBrowser    bool
	BrowserReason string

	ActivateMCP []string // Server names
	MCPReason   string

	DelegateToSpecialist bool
	SpecialistReason     string

	// Optimization
	CompressionStrategy string
	TokenOptimizations  []string
	UsePromptCaching    bool

	// Decision metadata
	Reasoning          string  // Explanation of why this decision was made
	Confidence         float32 // 0-1 confidence in the decision
	EstimatedCost      float64
	EstimatedLatencyMs int
}

// NewCapabilityRouter creates a new router
func NewCapabilityRouter(
	cfg *config.GorkbotConfig,
	factory *providers.ProviderFactory,
	logger *slog.Logger,
) *CapabilityRouter {
	if logger == nil {
		logger = slog.Default()
	}

	return &CapabilityRouter{
		config:     cfg,
		factory:    factory,
		classifier: NewIntentClassifier(),
		logger:     logger,
	}
}

// RouteCapabilities makes a comprehensive routing decision for a task
// This is the SINGLE decision point that determines ALL feature usage
func (cr *CapabilityRouter) RouteCapabilities(ctx context.Context, task *Task) (*RoutingDecision, error) {
	// Step 1: Classify the task intent
	intent := cr.classifier.Classify(task.Content)
	cr.logger.Debug("classified task intent", slog.String("intent", intent))

	// Step 2: Determine primary provider
	providerName := cr.selectProvider(task, intent)
	if providerName == "" {
		return nil, fmt.Errorf("no provider available for task")
	}

	// Step 3: Create provider instance
	provider, err := cr.factory.CreateProvider(providerName)
	if err != nil {
		// Try fallback chain
		fallbacks := cr.factory.GetFallbacks(providerName)
		provider, providerName, err = cr.tryFallbacks(fallbacks)
		if err != nil {
			return nil, err
		}
	}

	// Step 4: Build routing decision
	decision := &RoutingDecision{
		Provider:      provider,
		ProviderName:  providerName,
		FallbackChain: cr.factory.GetFallbacks(providerName),
		Confidence:    0.9,
	}

	// Step 5: Determine capabilities based on provider and task
	cr.selectCapabilities(decision, task, intent, provider)

	// Step 6: Estimate cost and latency
	cr.estimateCost(decision, task, provider)

	cr.logger.Debug("routing decision made",
		slog.String("provider", providerName),
		slog.String("intent", intent),
		slog.Bool("thinking", decision.UseThinking),
		slog.Bool("browser", decision.UseBrowser),
	)

	return decision, nil
}

// selectProvider determines which provider to use
func (cr *CapabilityRouter) selectProvider(task *Task, intent string) string {
	// Priority 1: Intent-based routing
	if provider, ok := cr.config.Routing.IntentClass[intent]; ok {
		return provider
	}

	// Priority 2: File-type routing
	if fileType := extractFileType(task); fileType != "" {
		if provider, ok := cr.config.Routing.FileType[fileType]; ok {
			return provider
		}
	}

	// Priority 3: Directory routing
	if directory := extractDirectory(task); directory != "" {
		if provider, ok := cr.config.Routing.Directory[directory]; ok {
			return provider
		}
	}

	// Priority 4: Default provider
	return cr.config.Capabilities.DefaultProvider
}

// selectCapabilities determines which features to activate
func (cr *CapabilityRouter) selectCapabilities(
	decision *RoutingDecision,
	task *Task,
	intent string,
	provider providers.AIProvider,
) {
	caps := provider.Capabilities()

	// Thinking capability
	decision.UseThinking = shouldUseThinking(task, intent, caps)
	if decision.UseThinking {
		decision.ThinkingBudget = cr.config.Specialist.ThinkingBudget
	}

	// Browser capability
	decision.UseBrowser = shouldUseBrowser(task, intent)
	if decision.UseBrowser {
		decision.BrowserReason = "task requires visual verification"
	}

	// MCP capability
	decision.ActivateMCP = shouldActivateMCP(task)
	if len(decision.ActivateMCP) > 0 {
		decision.MCPReason = fmt.Sprintf("task mentions: %v", decision.ActivateMCP)
	}

	// Specialist delegation
	decision.DelegateToSpecialist = shouldDelegateToSpecialist(task, cr.config)
	if decision.DelegateToSpecialist {
		decision.SpecialistReason = fmt.Sprintf("task complexity %d exceeds threshold %d",
			estimateComplexity(task), cr.config.Specialist.ComplexityThreshold)
	}

	// Optimization settings
	decision.CompressionStrategy = cr.config.Context.CompressionStrategy
	decision.UsePromptCaching = cr.selectPromptCaching(caps)

	// Prompt variant
	decision.PromptVariant = cr.selectPromptVariant(provider, caps)

	// Temperature
	decision.Temperature = 0.7 // Default
	if intent == "creative" || intent == "brainstorming" {
		decision.Temperature = 0.9
	} else if intent == "logic" || intent == "analysis" {
		decision.Temperature = 0.3
	}

	// Max tokens
	decision.MaxTokens = 2000 // Default
	if decision.UseThinking {
		decision.MaxTokens = 4000
	}
}

// selectPromptVariant selects the best prompt variant for the provider
func (cr *CapabilityRouter) selectPromptVariant(
	provider providers.AIProvider,
	caps *providers.ProviderCapabilities,
) string {
	// Check if user has override
	override, ok := cr.config.Prompts.PerProvider[provider.Name()]
	if ok {
		return override
	}

	// Auto-select based on provider capabilities
	if caps.SupportsThinking {
		return "claude_thinking"
	}
	if provider.Name() == "openai" {
		return "gpt5"
	}
	if provider.Name() == "google" {
		return "gemini3"
	}

	// Fallback to generic
	return "generic"
}

// selectPromptCaching determines if prompt caching should be used
func (cr *CapabilityRouter) selectPromptCaching(caps *providers.ProviderCapabilities) bool {
	if !cr.config.Context.Caching.Enabled {
		return false
	}

	return caps.SupportsPromptCaching
}

// tryFallbacks attempts providers in fallback chain
func (cr *CapabilityRouter) tryFallbacks(fallbacks []string) (
	providers.AIProvider, string, error) {
	for _, fallback := range fallbacks {
		provider, err := cr.factory.CreateProvider(fallback)
		if err == nil {
			cr.logger.Info("using fallback provider", slog.String("provider", fallback))
			return provider, fallback, nil
		}
	}
	return nil, "", fmt.Errorf("all providers unavailable, exhausted fallback chain")
}

// estimateCost estimates the cost for this decision
func (cr *CapabilityRouter) estimateCost(
	decision *RoutingDecision,
	task *Task,
	provider providers.AIProvider,
) {
	// Rough estimation
	estimatedInputTokens := len(task.Content) / 4 // ~4 chars per token
	estimatedOutputTokens := decision.MaxTokens

	inputCost := (float64(estimatedInputTokens) / 1e6) * provider.Capabilities().CostPer1MInputTokens
	outputCost := (float64(estimatedOutputTokens) / 1e6) * provider.Capabilities().CostPer1MOutputTokens

	decision.EstimatedCost = inputCost + outputCost

	// Estimate latency (rough)
	avgLatency := provider.Capabilities().AverageLatencyMs
	if avgLatency == 0 {
		avgLatency = 1000 // 1 second default
	}
	decision.EstimatedLatencyMs = avgLatency
}

// Helper functions for capability detection

func shouldUseThinking(task *Task, intent string, caps *providers.ProviderCapabilities) bool {
	if !caps.SupportsThinking {
		return false
	}

	// Use thinking for complex tasks
	complexity := estimateComplexity(task)
	if complexity >= 7 {
		return true
	}

	// Use thinking for certain intents
	thinkingIntents := map[string]bool{
		"architecture": true,
		"refactoring":  true,
		"debugging":    true,
		"analysis":     true,
		"research":     true,
		"planning":     true,
	}

	return thinkingIntents[intent]
}

func shouldUseBrowser(task *Task, intent string) bool {
	// Use browser for tasks mentioning visual/UI elements
	visualKeywords := []string{"ui", "visual", "screenshot", "render", "layout", "click", "browser", "web", "screen"}
	for _, keyword := range visualKeywords {
		if strings.Contains(strings.ToLower(task.Content), keyword) {
			return true
		}
	}

	// Use for certain intents
	browserIntents := map[string]bool{
		"validation": true,
	}

	return browserIntents[intent]
}

func shouldActivateMCP(task *Task) []string {
	// Extract MCP servers mentioned in task
	mcpServers := []string{}

	keywords := map[string]string{
		"jira":      "jira",
		"issue":     "jira",
		"aws":       "aws",
		"github":    "github",
		"git":       "github",
		"slack":     "slack",
		"pagerduty": "pagerduty",
	}

	content := strings.ToLower(task.Content)
	for keyword, server := range keywords {
		if strings.Contains(content, keyword) {
			mcpServers = append(mcpServers, server)
		}
	}

	return mcpServers
}

func shouldDelegateToSpecialist(task *Task, cfg *config.GorkbotConfig) bool {
	if !cfg.Specialist.Enabled {
		return false
	}

	complexity := estimateComplexity(task)
	files := countFiles(task)

	return complexity >= cfg.Specialist.ComplexityThreshold ||
		files >= cfg.Specialist.FilesThreshold
}

func estimateComplexity(task *Task) int {
	// Heuristic: longer content = more complex
	complexity := 1

	if len(task.Content) > 1000 {
		complexity += 2
	}
	if len(task.Content) > 5000 {
		complexity += 2
	}
	if len(task.Content) > 10000 {
		complexity += 2
	}

	// Keywords suggesting complexity
	complexKeywords := []string{"architecture", "refactor", "performance", "optimization"}
	for _, keyword := range complexKeywords {
		if strings.Contains(strings.ToLower(task.Content), keyword) {
			complexity += 2
		}
	}

	if complexity > 10 {
		complexity = 10
	}

	return complexity
}

func countFiles(task *Task) int {
	re := regexp.MustCompile(`(?i)(\d+)\s+files`)
	if matches := re.FindStringSubmatch(task.Content); len(matches) == 2 {
		if n, err := strconv.Atoi(matches[1]); err == nil {
			return n
		}
	}

	count := strings.Count(task.Content, "pkg/") +
		strings.Count(task.Content, "/") +
		strings.Count(task.Content, ".go") +
		strings.Count(task.Content, ".ts") +
		strings.Count(task.Content, ".py")

	// Rough estimate: every 5 mentions = 1 file
	return count / 5
}

func extractFileType(task *Task) string {
	// Extract file extension from task
	extensions := []string{".go", ".ts", ".tsx", ".js", ".py", ".rs", ".java"}
	content := strings.ToLower(task.Content)

	for _, ext := range extensions {
		if strings.Contains(content, ext) {
			return "*" + ext
		}
	}

	return ""
}

func extractDirectory(task *Task) string {
	// Extract directory patterns from task
	patterns := []string{
		"src/",
		"pkg/",
		"internal/",
		"cmd/",
		"test/",
		"tests/",
	}

	content := strings.ToLower(task.Content)
	for _, pattern := range patterns {
		if strings.Contains(content, pattern) {
			return "/" + pattern + "**"
		}
	}

	return ""
}

// Task represents a work item for routing
type Task struct {
	Content  string
	Path     string
	Language string
	Keywords []string
	Context  map[string]interface{}
}

// NewTask creates a new task
func NewTask(content string) *Task {
	return &Task{
		Content:  content,
		Keywords: extractKeywords(content),
		Context:  make(map[string]interface{}),
	}
}

func extractKeywords(content string) []string {
	// Simple keyword extraction
	keywords := []string{}
	words := strings.Fields(strings.ToLower(content))

	importantWords := map[string]bool{
		"refactor":     true,
		"test":         true,
		"fix":          true,
		"optimize":     true,
		"debug":        true,
		"architecture": true,
		"design":       true,
	}

	for _, word := range words {
		// Remove punctuation
		word = strings.TrimSpace(word)
		word = strings.TrimRight(word, ".,!?;:\"'")
		if importantWords[word] {
			keywords = append(keywords, word)
		}
	}

	return keywords
}

// RouteForCapability routes a request for a specific capability
func (cr *CapabilityRouter) RouteForCapability(
	ctx context.Context,
	capability string,
) (providers.AIProvider, error) {
	return cr.factory.SelectProvider(ctx, capability)
}

// OptimizeForBudget selects a cheaper provider if budget is tight
func (cr *CapabilityRouter) OptimizeForBudget(
	decision *RoutingDecision,
	remainingBudget float64,
) {
	if decision.EstimatedCost > remainingBudget && len(decision.FallbackChain) > 0 {
		// Try to find a cheaper provider in fallback chain
		for _, fallbackName := range decision.FallbackChain {
			fallback, err := cr.factory.CreateProvider(fallbackName)
			if err == nil && fallback.Capabilities().CostPer1MInputTokens < decision.Provider.Capabilities().CostPer1MInputTokens {
				cr.logger.Info("optimizing for budget: using cheaper fallback",
					slog.String("original", decision.ProviderName),
					slog.String("fallback", fallbackName))
				decision.Provider = fallback
				decision.ProviderName = fallbackName
				cr.estimateCost(decision, &Task{Content: ""}, fallback)
				break
			}
		}
	}
}
