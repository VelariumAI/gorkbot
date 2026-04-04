package billing

import (
	"testing"
)

func TestBillingManager_TrackTurn(t *testing.T) {
	bm := &BillingManager{
		Session: make(map[string]*SessionUsage),
	}
	bm.Config = bm.defaultConfig()

	// Track a turn
	bm.TrackTurn("xai", "grok-3", 1000, 500)

	// Verify tracking
	if bm.TotalSession == 0 {
		t.Error("expected non-zero total session cost")
	}
	if usage, ok := bm.Session["grok-3"]; !ok || usage.InputTokens != 1000 {
		t.Error("expected grok-3 usage to be tracked")
	}
}

func TestBillingManager_CalculateCost(t *testing.T) {
	bm := &BillingManager{
		Session: make(map[string]*SessionUsage),
	}
	bm.Config = bm.defaultConfig()

	// Calculate cost without tracking
	cost := bm.CalculateCost("xai", "grok-3", 1000000, 500000)

	// Should calculate based on pricing
	if cost == 0 {
		t.Error("expected non-zero cost for 1M+500k tokens")
	}

	// Verify no tracking happened
	if len(bm.Session) > 0 {
		t.Error("expected no session tracking from CalculateCost")
	}
}

func TestBillingManager_GetCostString(t *testing.T) {
	bm := &BillingManager{
		Session:      make(map[string]*SessionUsage),
		TotalSession: 0,
	}

	// Test zero cost
	if s := bm.GetCostString(); s != "$0.00" {
		t.Errorf("expected $0.00 for zero cost, got %s", s)
	}

	// Test small cost (< $0.01)
	bm.TotalSession = 0.005
	s := bm.GetCostString()
	if s != "$0.0050" {
		t.Errorf("expected $0.0050, got %s", s)
	}

	// Test regular cost (>= $0.01)
	bm.TotalSession = 0.25
	s = bm.GetCostString()
	if s != "$0.25" {
		t.Errorf("expected $0.25, got %s", s)
	}
}

func TestBillingManager_MultipleModels(t *testing.T) {
	bm := &BillingManager{
		Session: make(map[string]*SessionUsage),
	}
	bm.Config = bm.defaultConfig()

	// Track multiple models
	bm.TrackTurn("xai", "grok-3", 1000, 500)
	bm.TrackTurn("google", "gemini-2.0-flash", 2000, 1000)

	// Verify both are tracked
	if len(bm.Session) != 2 {
		t.Errorf("expected 2 models tracked, got %d", len(bm.Session))
	}

	// Verify costs are separate
	grok := bm.Session["grok-3"]
	gemini := bm.Session["gemini-2.0-flash"]
	if grok == nil || gemini == nil {
		t.Fatal("expected both models to be tracked")
	}
	if grok.TotalCost == 0 || gemini.TotalCost == 0 {
		t.Error("expected both to have non-zero costs")
	}
}

func TestBillingManager_DefaultPricing(t *testing.T) {
	bm := &BillingManager{
		Session: make(map[string]*SessionUsage),
	}

	// Test with default config
	config := bm.defaultConfig()
	bm.Config = config

	// Verify all major providers are configured
	providers := []string{"anthropic", "google", "xai", "openai"}
	for _, prov := range providers {
		if _, ok := config.Providers[prov]; !ok {
			t.Errorf("expected provider %s in default config", prov)
		}
	}
}

func TestBillingManager_GetSessionReport(t *testing.T) {
	bm := &BillingManager{
		Session:      make(map[string]*SessionUsage),
		TotalSession: 0,
	}

	// Test empty report
	report := bm.GetSessionReport()
	if report == "" {
		t.Error("expected non-empty report for empty session")
	}

	// Add some usage
	bm.Session["grok-3"] = &SessionUsage{
		InputTokens:  1000,
		OutputTokens: 500,
		TotalCost:    0.025,
	}
	bm.TotalSession = 0.025

	report = bm.GetSessionReport()
	if len(report) == 0 {
		t.Error("expected non-empty report")
	}
	if !contains(report, "grok-3") {
		t.Error("expected grok-3 in report")
	}
	if !contains(report, "$") {
		t.Error("expected cost symbol in report")
	}
}

func TestBillingManager_Concurrency(t *testing.T) {
	bm := &BillingManager{
		Session: make(map[string]*SessionUsage),
	}
	bm.Config = bm.defaultConfig()

	// Track turns concurrently
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			bm.TrackTurn("xai", "grok-3", 100*(idx+1), 50*(idx+1))
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify total tracked correctly
	cost := bm.GetTotalSessionCost()
	if cost == 0 {
		t.Error("expected non-zero total cost after concurrent tracking")
	}
}

func TestBillingManager_ConfigEdgeCases(t *testing.T) {
	bm := &BillingManager{
		Session: make(map[string]*SessionUsage),
	}
	bm.Config = bm.defaultConfig()

	// Calculate cost for non-existent provider should return 0 gracefully
	cost := bm.CalculateCost("nonexistent", "model", 1000, 500)
	if cost != 0 {
		t.Errorf("unknown provider should return 0 cost, got %f", cost)
	}

	// Zero tokens should result in zero cost
	zeroCost := bm.CalculateCost("xai", "grok-3", 0, 0)
	if zeroCost != 0 {
		t.Errorf("zero tokens should cost zero, got %f", zeroCost)
	}
}

func TestBillingManager_CostStringEdgeCases(t *testing.T) {
	bm := &BillingManager{
		Session:      make(map[string]*SessionUsage),
		TotalSession: 0,
	}

	// Very small cost
	bm.TotalSession = 0.001
	s := bm.GetCostString()
	if s != "$0.0010" {
		t.Errorf("expected $0.0010, got %s", s)
	}

	// Large cost
	bm.TotalSession = 1234.567
	s = bm.GetCostString()
	if !contains(s, "1234") {
		t.Errorf("expected large cost in string, got %s", s)
	}
}

func TestBillingManager_SessionUsageMetadata(t *testing.T) {
	bm := &BillingManager{
		Session: make(map[string]*SessionUsage),
	}
	bm.Config = bm.defaultConfig()

	// Track a turn
	bm.TrackTurn("xai", "grok-3", 2000, 1000)

	usage := bm.Session["grok-3"]
	if usage.InputTokens != 2000 {
		t.Errorf("input tokens mismatch: %d", usage.InputTokens)
	}
	if usage.OutputTokens != 1000 {
		t.Errorf("output tokens mismatch: %d", usage.OutputTokens)
	}
	if usage.TotalCost == 0 {
		t.Error("total cost should be calculated")
	}

	// Track another turn
	bm.TrackTurn("xai", "grok-3", 3000, 1500)

	usage = bm.Session["grok-3"]
	if usage.InputTokens != 5000 {  // 2000 + 3000
		t.Errorf("accumulated input tokens mismatch: %d", usage.InputTokens)
	}
	if usage.OutputTokens != 2500 {  // 1000 + 1500
		t.Errorf("accumulated output tokens mismatch: %d", usage.OutputTokens)
	}
}

func TestBillingManager_ProviderPricing(t *testing.T) {
	bm := &BillingManager{
		Session: make(map[string]*SessionUsage),
	}
	bm.Config = bm.defaultConfig()

	// Different providers should have different costs
	costXAI := bm.CalculateCost("xai", "grok-3", 10000, 5000)
	costGoogle := bm.CalculateCost("google", "gemini-2.0-flash", 10000, 5000)
	costAnthrop := bm.CalculateCost("anthropic", "claude-opus", 10000, 5000)

	// At least one should be different
	if costXAI == costGoogle && costGoogle == costAnthrop {
		t.Error("different providers should have different pricing")
	}
}

func TestBillingManager_LargeTokenCounts(t *testing.T) {
	bm := &BillingManager{
		Session: make(map[string]*SessionUsage),
	}
	bm.Config = bm.defaultConfig()

	// Large token counts
	bm.TrackTurn("xai", "grok-3", 100000, 50000)

	if bm.TotalSession == 0 {
		t.Error("large token counts should result in non-zero cost")
	}

	cost := bm.GetTotalSessionCost()
	if cost != bm.TotalSession {
		t.Errorf("GetTotalSessionCost mismatch: %f vs %f", cost, bm.TotalSession)
	}
}

func TestSessionUsage_TokenAccumulation(t *testing.T) {
	su := &SessionUsage{
		InputTokens:  1000,
		OutputTokens: 500,
		TotalCost:    0.01,
	}

	// Calculate total tokens
	total := su.InputTokens + su.OutputTokens
	if total != 1500 {
		t.Errorf("total tokens should be 1500, got %d", total)
	}

	// Cost should be positive
	if su.TotalCost <= 0 {
		t.Error("cost should be positive")
	}
}

func TestBillingManager_ConcurrentTrackingAccuracy(t *testing.T) {
	bm := &BillingManager{
		Session: make(map[string]*SessionUsage),
	}
	bm.Config = bm.defaultConfig()

	// Track turns concurrently for different models
	done := make(chan bool)

	for i := 0; i < 5; i++ {
		go func(idx int) {
			bm.TrackTurn("xai", "grok-3", 100*(idx+1), 50*(idx+1))
			done <- true
		}(i)
	}

	for i := 0; i < 5; i++ {
		<-done
	}

	// Verify total input tokens accumulated correctly
	usage := bm.Session["grok-3"]
	expectedInput := 100 + 200 + 300 + 400 + 500  // 1500
	if usage.InputTokens != expectedInput {
		t.Errorf("concurrent tracking: expected %d input tokens, got %d", expectedInput, usage.InputTokens)
	}
}

func TestBillingManager_GetSessionReport_Detailed(t *testing.T) {
	bm := &BillingManager{
		Session:      make(map[string]*SessionUsage),
		TotalSession: 0,
	}
	bm.Config = bm.defaultConfig()

	// Track multiple models
	bm.TrackTurn("xai", "grok-3", 5000, 2500)
	bm.TrackTurn("google", "gemini", 3000, 1500)
	bm.TrackTurn("anthropic", "claude-opus", 2000, 1000)

	report := bm.GetSessionReport()

	// Verify report contains key information
	if !contains(report, "grok-3") {
		t.Error("report should contain grok-3")
	}
	if !contains(report, "gemini") {
		t.Error("report should contain gemini")
	}
	if !contains(report, "claude-opus") {
		t.Error("report should contain claude-opus")
	}
	if !contains(report, "$") {
		t.Error("report should contain cost symbol")
	}
}

func TestNewBillingManager(t *testing.T) {
	bm := NewBillingManager()

	if bm == nil {
		t.Fatal("NewBillingManager should return non-nil")
	}
	if bm.Session == nil {
		t.Error("Session map should be initialized")
	}
}

func TestBillingManager_AllTimeReportEmpty(t *testing.T) {
	bm := &BillingManager{
		Session: make(map[string]*SessionUsage),
	}
	bm.Config = bm.defaultConfig()

	report := bm.GetAllTimeReport()
	if report == "" {
		t.Error("should return non-empty report even when empty")
	}
}

func TestBillingManager_AllTimeReportWithSessionData(t *testing.T) {
	bm := &BillingManager{
		Session: make(map[string]*SessionUsage),
	}
	bm.Config = bm.defaultConfig()

	// Track session usage
	bm.TrackTurn("xai", "grok-3", 10000, 5000)

	// GetAllTimeReport should reflect current usage
	report := bm.GetAllTimeReport()

	if report == "" {
		t.Error("report should be non-empty with session data")
	}
}

func TestBillingManager_GetTotalSessionCostZero(t *testing.T) {
	bm := &BillingManager{
		Session:      make(map[string]*SessionUsage),
		TotalSession: 0,
	}

	cost := bm.GetTotalSessionCost()
	if cost != 0 {
		t.Errorf("empty session should have zero cost, got %f", cost)
	}
}

func TestBillingManager_DefaultConfig_Comprehensive(t *testing.T) {
	bm := &BillingManager{
		Session: make(map[string]*SessionUsage),
	}
	config := bm.defaultConfig()

	if config.Providers == nil {
		t.Error("providers should be configured")
	}

	// Verify major providers are present
	requiredProviders := []string{"anthropic", "google", "xai", "openai"}
	for _, provider := range requiredProviders {
		if _, exists := config.Providers[provider]; !exists {
			t.Errorf("missing provider: %s", provider)
		}
	}
}

func TestBillingManager_MixedProviderCosts(t *testing.T) {
	bm := &BillingManager{
		Session: make(map[string]*SessionUsage),
	}
	bm.Config = bm.defaultConfig()

	// Track same input/output tokens for different providers
	inputs, outputs := 5000, 2500
	bm.TrackTurn("xai", "grok-3", inputs, outputs)
	bm.TrackTurn("google", "gemini", inputs, outputs)
	bm.TrackTurn("anthropic", "claude-opus", inputs, outputs)

	_ = bm.GetCostString()  // Verify cost string generation works

	// All should have tracked without error
	if bm.GetTotalSessionCost() <= 0 {
		t.Error("should have non-zero total cost with multiple providers")
	}
}

func TestBillingManager_CalculateCostMultipleModels(t *testing.T) {
	bm := &BillingManager{
		Session: make(map[string]*SessionUsage),
	}
	bm.Config = bm.defaultConfig()

	// Calculate costs for same provider, different models
	grok3Cost := bm.CalculateCost("xai", "grok-3", 10000, 5000)
	grok2Cost := bm.CalculateCost("xai", "grok-2", 10000, 5000)

	// Different models may have different costs
	if grok3Cost < 0 || grok2Cost < 0 {
		t.Error("costs should be non-negative")
	}
}

func TestSessionUsage_Fields(t *testing.T) {
	su := &SessionUsage{
		InputTokens:  2500,
		OutputTokens: 1250,
		TotalCost:    0.025,
	}

	if su.InputTokens != 2500 || su.OutputTokens != 1250 || su.TotalCost != 0.025 {
		t.Error("SessionUsage fields not properly set")
	}
}

func TestBillingManager_TrackTurnTwice(t *testing.T) {
	bm := &BillingManager{
		Session: make(map[string]*SessionUsage),
	}
	bm.Config = bm.defaultConfig()

	// Track same model twice
	bm.TrackTurn("xai", "grok-3", 1000, 500)
	cost1 := bm.GetTotalSessionCost()

	bm.TrackTurn("xai", "grok-3", 1000, 500)
	cost2 := bm.GetTotalSessionCost()

	// Cost should approximately double
	if cost2 <= cost1 {
		t.Error("second tracking should increase cost")
	}
}

func TestBillingManager_EmptyProviderReport(t *testing.T) {
	bm := &BillingManager{
		Session: make(map[string]*SessionUsage),
	}
	bm.Config = bm.defaultConfig()

	// No usage tracked
	report := bm.GetSessionReport()

	// Should still return a string (even if empty/minimal)
	if report == "" {
		t.Error("report should not be empty string")
	}
}

func TestBillingManager_CostStringVariations(t *testing.T) {
	tests := []struct {
		cost     float64
		minChars int
	}{
		{0.001, 4},      // $0.0010
		{0.1, 4},        // $0.1000
		{1.0, 4},        // $1.0000
		{10.5, 4},       // $10.5000
		{100.25, 5},     // $100.2500
	}

	for _, test := range tests {
		bm := &BillingManager{
			Session:      make(map[string]*SessionUsage),
			TotalSession: test.cost,
		}

		s := bm.GetCostString()
		if !contains(s, "$") {
			t.Errorf("cost string missing $: %s", s)
		}
		if len(s) < test.minChars {
			t.Errorf("cost string too short for %.4f: got %s", test.cost, s)
		}
	}
}

func TestBillingManager_SessionUsageUpdate(t *testing.T) {
	bm := &BillingManager{
		Session: make(map[string]*SessionUsage),
	}
	bm.Config = bm.defaultConfig()

	// First track
	bm.TrackTurn("google", "gemini", 2000, 1000)

	// Verify it exists
	usage := bm.Session["gemini"]
	if usage == nil {
		t.Fatal("usage should exist for gemini")
	}

	firstInputTokens := usage.InputTokens

	// Second track
	bm.TrackTurn("google", "gemini", 3000, 1500)

	// Verify accumulation
	updatedUsage := bm.Session["gemini"]
	if updatedUsage.InputTokens != firstInputTokens+3000 {
		t.Errorf("input tokens should accumulate: %d vs %d", updatedUsage.InputTokens, firstInputTokens+3000)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
