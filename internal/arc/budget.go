package arc

import (
	"time"

	"github.com/velariumai/gorkbot/internal/platform"
)

// PlatformClass categorizes the host device for resource budgeting.
type PlatformClass int

const (
	// PlatformMobileLight is a device with less than 4 GB RAM.
	PlatformMobileLight PlatformClass = iota
	// PlatformMobileMid is a mid-range mobile device (4–12 GB RAM, e.g. S23 Ultra 12 GB).
	PlatformMobileMid
	// PlatformDesktopMid is a desktop/server with 12–32 GB RAM.
	PlatformDesktopMid
	// PlatformDesktopHeavy is a high-end desktop/server with more than 32 GB RAM.
	PlatformDesktopHeavy
)

// String returns a human-readable platform class name.
func (p PlatformClass) String() string {
	switch p {
	case PlatformMobileLight:
		return "mobile_light"
	case PlatformMobileMid:
		return "mobile_mid"
	case PlatformDesktopMid:
		return "desktop_mid"
	case PlatformDesktopHeavy:
		return "desktop_heavy"
	default:
		return "unknown"
	}
}

// CostTier signals the preferred model cost level for this workflow.
type CostTier int

const (
	// CostTierCheap — conversational/factual: use mini/flash/haiku variants.
	CostTierCheap CostTier = iota
	// CostTierStandard — analytical/creative: standard models.
	CostTierStandard
	// CostTierPremium — agentic/security: best available models.
	CostTierPremium
)

// ResourceBudget contains the computed resource limits for one request.
type ResourceBudget struct {
	MaxTokens    int
	Temperature  float64
	MaxToolCalls int
	Timeout      time.Duration
	Platform     PlatformClass
	Workflow     WorkflowType
	CostTier     CostTier
}

// SystemDetector converts a HALProfile into a PlatformClass using total RAM.
func SystemDetector(hal platform.HALProfile) PlatformClass {
	totalMB := hal.TotalRAMMB
	switch {
	case totalMB <= 4096:
		return PlatformMobileLight
	case totalMB <= 12288:
		return PlatformMobileMid
	case totalMB <= 32768:
		return PlatformDesktopMid
	default:
		return PlatformDesktopHeavy
	}
}

type budgetEntry struct {
	maxTokens    int
	maxToolCalls int
	temperature  float64
	timeout      time.Duration
	costTier     CostTier
}

// budgetTable defines resource limits per workflow class × platform.
// Rows: platform classes. Columns indexed by WorkflowType.
var budgetTable = map[PlatformClass][workflowClassCount]budgetEntry{
	PlatformMobileLight: {
		WorkflowConversational:  {maxTokens: 2048, maxToolCalls: 2, temperature: 0.7, timeout: 20 * time.Second, costTier: CostTierCheap},
		WorkflowFactual:         {maxTokens: 4096, maxToolCalls: 3, temperature: 0.3, timeout: 25 * time.Second, costTier: CostTierCheap},
		WorkflowAnalytical:      {maxTokens: 8192, maxToolCalls: 6, temperature: 0.5, timeout: 45 * time.Second, costTier: CostTierStandard},
		WorkflowAgentic:         {maxTokens: 8192, maxToolCalls: 8, temperature: 0.4, timeout: 60 * time.Second, costTier: CostTierPremium},
		WorkflowCreative:        {maxTokens: 4096, maxToolCalls: 2, temperature: 0.8, timeout: 30 * time.Second, costTier: CostTierCheap},
		WorkflowSecurityCritical: {maxTokens: 8192, maxToolCalls: 12, temperature: 0.3, timeout: 90 * time.Second, costTier: CostTierPremium},
	},
	PlatformMobileMid: {
		WorkflowConversational:  {maxTokens: 4096, maxToolCalls: 2, temperature: 0.7, timeout: 25 * time.Second, costTier: CostTierCheap},
		WorkflowFactual:         {maxTokens: 8192, maxToolCalls: 4, temperature: 0.3, timeout: 30 * time.Second, costTier: CostTierCheap},
		WorkflowAnalytical:      {maxTokens: 16384, maxToolCalls: 8, temperature: 0.5, timeout: 60 * time.Second, costTier: CostTierStandard},
		WorkflowAgentic:         {maxTokens: 16384, maxToolCalls: 12, temperature: 0.4, timeout: 90 * time.Second, costTier: CostTierPremium},
		WorkflowCreative:        {maxTokens: 8192, maxToolCalls: 3, temperature: 0.8, timeout: 40 * time.Second, costTier: CostTierStandard},
		WorkflowSecurityCritical: {maxTokens: 16384, maxToolCalls: 15, temperature: 0.3, timeout: 120 * time.Second, costTier: CostTierPremium},
	},
	PlatformDesktopMid: {
		WorkflowConversational:  {maxTokens: 4096, maxToolCalls: 3, temperature: 0.7, timeout: 30 * time.Second, costTier: CostTierCheap},
		WorkflowFactual:         {maxTokens: 16384, maxToolCalls: 5, temperature: 0.3, timeout: 45 * time.Second, costTier: CostTierCheap},
		WorkflowAnalytical:      {maxTokens: 32768, maxToolCalls: 10, temperature: 0.5, timeout: 90 * time.Second, costTier: CostTierStandard},
		WorkflowAgentic:         {maxTokens: 32768, maxToolCalls: 15, temperature: 0.4, timeout: 120 * time.Second, costTier: CostTierPremium},
		WorkflowCreative:        {maxTokens: 16384, maxToolCalls: 4, temperature: 0.8, timeout: 60 * time.Second, costTier: CostTierStandard},
		WorkflowSecurityCritical: {maxTokens: 32768, maxToolCalls: 20, temperature: 0.3, timeout: 180 * time.Second, costTier: CostTierPremium},
	},
	PlatformDesktopHeavy: {
		WorkflowConversational:  {maxTokens: 8192, maxToolCalls: 3, temperature: 0.7, timeout: 30 * time.Second, costTier: CostTierCheap},
		WorkflowFactual:         {maxTokens: 32768, maxToolCalls: 5, temperature: 0.3, timeout: 60 * time.Second, costTier: CostTierCheap},
		WorkflowAnalytical:      {maxTokens: 65536, maxToolCalls: 15, temperature: 0.5, timeout: 120 * time.Second, costTier: CostTierStandard},
		WorkflowAgentic:         {maxTokens: 65536, maxToolCalls: 20, temperature: 0.4, timeout: 180 * time.Second, costTier: CostTierPremium},
		WorkflowCreative:        {maxTokens: 32768, maxToolCalls: 5, temperature: 0.8, timeout: 90 * time.Second, costTier: CostTierStandard},
		WorkflowSecurityCritical: {maxTokens: 65536, maxToolCalls: 25, temperature: 0.3, timeout: 240 * time.Second, costTier: CostTierPremium},
	},
}

// ComputeBudget returns the ResourceBudget for the given platform and workflow.
func ComputeBudget(pc PlatformClass, wf WorkflowType) ResourceBudget {
	table, ok := budgetTable[pc]
	if !ok {
		table = budgetTable[PlatformMobileMid]
	}
	wfIdx := int(wf)
	if wfIdx < 0 || wfIdx >= workflowClassCount {
		wfIdx = int(WorkflowFactual)
	}
	entry := table[wfIdx]

	return ResourceBudget{
		MaxTokens:    entry.maxTokens,
		Temperature:  entry.temperature,
		MaxToolCalls: entry.maxToolCalls,
		Timeout:      entry.timeout,
		Platform:     pc,
		Workflow:     wf,
		CostTier:     entry.costTier,
	}
}
