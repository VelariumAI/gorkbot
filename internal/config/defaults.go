package config

// ApplyDefaults applies sensible defaults to configuration
func ApplyDefaults(cfg *GorkbotConfig) error {
	if cfg == nil {
		return nil
	}

	// Apply provider defaults
	for name, provider := range cfg.Providers {
		applyProviderDefaults(&provider)
		cfg.Providers[name] = provider
	}

	// Apply capability defaults
	applyCapabilityDefaults(cfg)

	// Apply routing defaults
	applyRoutingDefaults(cfg)

	// Apply specialist defaults
	applySpecialistDefaults(cfg)

	// Apply browser defaults
	applyBrowserDefaults(&cfg.Browser)

	// Apply MCP defaults
	applyMCPDefaults(&cfg.MCP)

	// Apply auto-repair defaults
	applyAutoRepairDefaults(&cfg.AutoRepair)

	// Apply context defaults
	applyContextDefaults(&cfg.Context)

	// Apply memory defaults
	applyMemoryDefaults(&cfg.Memory)

	// Apply optimization defaults
	applyOptimizationDefaults(&cfg.Optimization)

	// Apply prompts defaults
	applyPromptsDefaults(&cfg.Prompts)

	// Apply debug defaults
	applyDebugDefaults(&cfg.Debug)

	return nil
}

// applyProviderDefaults applies defaults to a provider configuration
func applyProviderDefaults(provider *ProviderConfig) {
	if provider.Timeout <= 0 {
		provider.Timeout = 60 // 60 seconds
	}

	if provider.MaxRetries == 0 {
		provider.MaxRetries = 3
	}

	if provider.Thinking.Budget == 0 && provider.Thinking.Enabled {
		provider.Thinking.Budget = 10000 // Default thinking budget
	}

	if provider.Caching.TTL == 0 && provider.Caching.Enabled {
		provider.Caching.TTL = 3600 // 1 hour
	}

	// Ensure cost fields have reasonable defaults
	if provider.CostPer1MInput == 0 {
		provider.CostPer1MInput = 1.0 // $1 per million input tokens (placeholder)
	}

	if provider.CostPer1MOutput == 0 {
		provider.CostPer1MOutput = 3.0 // $3 per million output tokens (placeholder)
	}
}

// applyCapabilityDefaults ensures all capabilities have providers
func applyCapabilityDefaults(cfg *GorkbotConfig) {
	// If a capability provider is not set, use the default provider
	if cfg.Capabilities.DefaultProvider != "" {
		if cfg.Capabilities.ThinkingProvider == "" {
			cfg.Capabilities.ThinkingProvider = cfg.Capabilities.DefaultProvider
		}
		if cfg.Capabilities.CodingProvider == "" {
			cfg.Capabilities.CodingProvider = cfg.Capabilities.DefaultProvider
		}
		if cfg.Capabilities.VisionProvider == "" {
			cfg.Capabilities.VisionProvider = cfg.Capabilities.DefaultProvider
		}
		if cfg.Capabilities.MemoryProvider == "" {
			cfg.Capabilities.MemoryProvider = cfg.Capabilities.DefaultProvider
		}
		if cfg.Capabilities.SpecialistProvider == "" {
			cfg.Capabilities.SpecialistProvider = cfg.Capabilities.DefaultProvider
		}
	}
}

// applyRoutingDefaults applies defaults to routing configuration
func applyRoutingDefaults(cfg *GorkbotConfig) {
	// Initialize maps if nil
	if cfg.Routing.IntentClass == nil {
		cfg.Routing.IntentClass = make(map[string]string)
	}
	if cfg.Routing.FileType == nil {
		cfg.Routing.FileType = make(map[string]string)
	}
	if cfg.Routing.Directory == nil {
		cfg.Routing.Directory = make(map[string]string)
	}
	if cfg.Routing.Fallback == nil {
		cfg.Routing.Fallback = make(map[string][]string)
	}

	// Default cost optimization
	if cfg.Routing.CostOptimize.BudgetPerDay == 0 {
		cfg.Routing.CostOptimize.BudgetPerDay = 100.0 // $100/day default budget
	}

	// Normalize cost/speed weights
	if cfg.Routing.CostOptimize.CostWeight == 0 && cfg.Routing.CostOptimize.SpeedWeight == 0 {
		cfg.Routing.CostOptimize.CostWeight = 0.3
		cfg.Routing.CostOptimize.SpeedWeight = 0.7
	}
}

// applySpecialistDefaults applies defaults to specialist configuration
func applySpecialistDefaults(cfg *GorkbotConfig) {
	if cfg.Specialist.ThinkingBudget == 0 {
		cfg.Specialist.ThinkingBudget = 100000 // 100k tokens
	}

	if cfg.Specialist.ComplexityThreshold == 0 {
		cfg.Specialist.ComplexityThreshold = 8 // Delegate if complexity > 8
	}

	if cfg.Specialist.FilesThreshold == 0 {
		cfg.Specialist.FilesThreshold = 20 // Delegate if > 20 files affected
	}

	if cfg.Specialist.DurationThreshold == 0 {
		cfg.Specialist.DurationThreshold = 300 // Delegate if > 5 minutes estimated
	}

	if cfg.Specialist.AutonomyLevel == "" {
		cfg.Specialist.AutonomyLevel = "validated" // Default autonomy level
	}

	// If specialist provider not set, use default
	if cfg.Specialist.Provider == "" && cfg.Capabilities.DefaultProvider != "" {
		cfg.Specialist.Provider = cfg.Capabilities.SpecialistProvider
	}
}

// applyBrowserDefaults applies defaults to browser configuration
func applyBrowserDefaults(cfg *BrowserConfig) {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 // 30 seconds
	}

	if cfg.Screenshots.Quality == 0 {
		cfg.Screenshots.Quality = 80 // 80% quality
	}

	if cfg.Screenshots.MaxWidth == 0 {
		cfg.Screenshots.MaxWidth = 1280 // 1280px max width
	}
}

// applyMCPDefaults applies defaults to MCP configuration
func applyMCPDefaults(cfg *MCPConfig) {
	// Initialize slices if nil
	if cfg.Servers == nil {
		cfg.Servers = make([]string, 0)
	}
}

// applyAutoRepairDefaults applies defaults to auto-repair configuration
func applyAutoRepairDefaults(cfg *AutoRepairConfig) {
	if cfg.MaxAttempts == 0 {
		cfg.MaxAttempts = 3 // Try up to 3 times
	}

	// Initialize linters if empty
	if len(cfg.Linters) == 0 {
		// Default linters based on what's available
		cfg.Linters = []string{
			"go vet", // For Go files
			"tsc",    // For TypeScript
			"ruff",   // For Python
		}
	}

	if !cfg.UseHashline {
		cfg.UseHashline = true // Use file hashing by default
	}
}

// applyContextDefaults applies defaults to context configuration
func applyContextDefaults(cfg *ContextConfig) {
	if cfg.Caching.TTL == 0 && cfg.Caching.Enabled {
		cfg.Caching.TTL = 3600 // 1 hour
	}

	if cfg.CompressionStrategy == "" {
		cfg.CompressionStrategy = "semantic" // Default to semantic compression
	}

	if cfg.CompressionLevel == 0 {
		cfg.CompressionLevel = 5 // Medium compression (1-10 scale)
	}

	if cfg.MaxWindow == 0 {
		cfg.MaxWindow = 150000 // 150k tokens (reasonable for most models)
	}
}

// applyMemoryDefaults applies defaults to memory configuration
func applyMemoryDefaults(cfg *MemoryConfig) {
	if cfg.DecayFunction == "" {
		cfg.DecayFunction = "exponential" // Exponential decay by default
	}

	if cfg.DecayHalfLife == 0 {
		cfg.DecayHalfLife = 7 // 7 days half-life
	}

	if !cfg.AutoSync {
		cfg.AutoSync = true // Auto-sync specialist insights by default
	}
}

// applyOptimizationDefaults applies defaults to optimization configuration
func applyOptimizationDefaults(cfg *OptimizationConfig) {
	if cfg.DailyBudget == 0 {
		cfg.DailyBudget = 100.0 // $100/day default budget
	}

	// Normalize weights
	if cfg.CostWeight == 0 && cfg.SpeedWeight == 0 {
		cfg.CostWeight = 0.3
		cfg.SpeedWeight = 0.7
	}

	if cfg.Batching.Size == 0 && cfg.Batching.Enabled {
		cfg.Batching.Size = 5 // Batch 5 items at a time
	}
}

// applyPromptsDefaults applies defaults to prompts configuration
func applyPromptsDefaults(cfg *PromptsConfig) {
	if cfg.Default == "" {
		cfg.Default = "generic" // Fallback to generic prompt
	}

	// Initialize maps if nil
	if cfg.PerProvider == nil {
		cfg.PerProvider = make(map[string]string)
	}

	if cfg.Custom == nil {
		cfg.Custom = make(map[string]string)
	}
}

// applyDebugDefaults applies defaults to debug configuration
func applyDebugDefaults(cfg *DebugConfig) {
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info" // Default log level
	}

	if cfg.LogFile == "" {
		cfg.LogFile = "~/.config/gorkbot/gorkbot.log"
	}
}

// DefaultConfig returns a minimal working configuration
func DefaultConfig() *GorkbotConfig {
	cfg := &GorkbotConfig{
		Providers: make(map[string]ProviderConfig),
		Capabilities: CapabilitiesConfig{
			ThinkingProvider:   "default",
			CodingProvider:     "default",
			VisionProvider:     "default",
			MemoryProvider:     "default",
			SpecialistProvider: "default",
			DefaultProvider:    "default",
		},
		Routing: RoutingConfig{
			IntentClass: make(map[string]string),
			FileType:    make(map[string]string),
			Directory:   make(map[string]string),
			Fallback:    make(map[string][]string),
		},
		Specialist: SpecialistConfig{
			Provider:            "default",
			ThinkingBudget:      100000,
			ComplexityThreshold: 8,
			FilesThreshold:      20,
			DurationThreshold:   300,
			AutonomyLevel:       "validated",
			Enabled:             true,
		},
		Browser: BrowserConfig{
			Enabled:        true,
			Headless:       true,
			VisionProvider: "default",
			Timeout:        30,
		},
		MCP: MCPConfig{
			Enabled: true,
			Servers: []string{},
		},
		AutoRepair: AutoRepairConfig{
			Enabled:     true,
			Linters:     []string{"go vet", "tsc", "ruff"},
			UseHashline: true,
			MaxAttempts: 3,
		},
		Context: ContextConfig{
			CompressionStrategy: "semantic",
			CompressionLevel:    5,
			UseAST:              true,
			MaxWindow:           150000,
		},
		Memory: MemoryConfig{
			Enabled:       true,
			DecayFunction: "exponential",
			DecayHalfLife: 7,
			AutoSync:      true,
			FactProvider:  "default",
		},
		Optimization: OptimizationConfig{
			DailyBudget: 100.0,
			CostWeight:  0.3,
			SpeedWeight: 0.7,
			PreferCheap: false,
		},
		Prompts: PromptsConfig{
			Default:     "generic",
			PerProvider: make(map[string]string),
			Custom:      make(map[string]string),
		},
		Debug: DebugConfig{
			Enabled:        false,
			LogLevel:       "info",
			LogFile:        "~/.config/gorkbot/gorkbot.log",
			VerboseRouting: false,
			DumpAPIs:       false,
		},
		Advanced: AdvancedConfig{
			Headers:          make(map[string]string),
			ProviderTimeouts: make(map[string]int),
			CustomRules:      []CustomRoutingRule{},
		},
	}

	cfg.Providers["default"] = ProviderConfig{
		APIKey:  "placeholder",
		Model:   "default",
		BaseURL: "http://localhost",
		Timeout: 60,
	}
	// Apply all defaults to nested structs
	ApplyDefaults(cfg)
	return cfg
}
