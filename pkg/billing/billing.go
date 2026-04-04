package billing

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type PricingEntry struct {
	Prefix     string  `json:"prefix"`
	InputPerM  float64 `json:"input_per_m"`
	OutputPerM float64 `json:"output_per_m"`
}

type ProviderPricing struct {
	Entries []PricingEntry `json:"entries"`
}

type PricingConfig struct {
	Providers map[string]ProviderPricing `json:"providers"`
}

type SessionUsage struct {
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalCost    float64 `json:"total_cost"`
}

type BillingManager struct {
	Mu           sync.RWMutex
	Config       PricingConfig
	Session      map[string]*SessionUsage // modelID -> usage
	TotalSession float64                  // overall cost this session
	configDir    string
}

// NewBillingManager loads pricing from ~/.gorkbot/usage_pricing.json or creates a default.
func NewBillingManager() *BillingManager {
	home, _ := os.UserHomeDir()
	return NewBillingManagerWithDir(filepath.Join(home, ".gorkbot"))
}

// NewBillingManagerWithDir uses the supplied configDir instead of ~/.gorkbot.
func NewBillingManagerWithDir(configDir string) *BillingManager {
	bm := &BillingManager{
		Session:   make(map[string]*SessionUsage),
		configDir: configDir,
	}
	bm.loadConfig()
	return bm
}

func (bm *BillingManager) loadConfig() {
	configPath := filepath.Join(bm.configDir, "usage_pricing.json")

	file, err := os.Open(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			bm.Config = bm.defaultConfig()
			bm.saveConfig(configPath)
		} else {
			bm.Config = bm.defaultConfig()
		}
		return
	}
	defer file.Close()

	body, _ := io.ReadAll(file)
	var config PricingConfig
	if err := json.Unmarshal(body, &config); err != nil {
		bm.Config = bm.defaultConfig()
	} else {
		bm.Config = config
	}
}

func (bm *BillingManager) saveConfig(path string) {
	os.MkdirAll(filepath.Dir(path), 0755)
	data, _ := json.MarshalIndent(bm.Config, "", "  ")
	os.WriteFile(path, data, 0644)
}

func (bm *BillingManager) defaultConfig() PricingConfig {
	return PricingConfig{
		Providers: map[string]ProviderPricing{
			"anthropic": {
				Entries: []PricingEntry{
					{Prefix: "claude-3-5-sonnet", InputPerM: 3.0, OutputPerM: 15.0},
					{Prefix: "claude-3-7-sonnet", InputPerM: 3.0, OutputPerM: 15.0},
					{Prefix: "claude-3-haiku", InputPerM: 0.25, OutputPerM: 1.25},
				},
			},
			"google": {
				Entries: []PricingEntry{
					{Prefix: "gemini-2.0-flash", InputPerM: 0.10, OutputPerM: 0.40},
					{Prefix: "gemini-1.5-pro", InputPerM: 1.25, OutputPerM: 5.00},
				},
			},
			"xai": {
				Entries: []PricingEntry{
					{Prefix: "grok-2", InputPerM: 2.0, OutputPerM: 10.0},
					{Prefix: "grok-3", InputPerM: 2.0, OutputPerM: 10.0},
				},
			},
			"openai": {
				Entries: []PricingEntry{
					{Prefix: "gpt-4o-mini", InputPerM: 0.15, OutputPerM: 0.60},
					{Prefix: "gpt-4o", InputPerM: 2.50, OutputPerM: 10.0},
					{Prefix: "o1", InputPerM: 15.0, OutputPerM: 60.0},
					{Prefix: "o3-mini", InputPerM: 1.10, OutputPerM: 4.40},
				},
			},
		},
	}
}

// TrackTurn adds tokens to the session and calculates cost
func (bm *BillingManager) TrackTurn(providerID, modelID string, inputTokens, outputTokens int) {
	bm.Mu.Lock()
	defer bm.Mu.Unlock()

	var inputPrice, outputPrice float64

	// Find pricing
	providerID = strings.ToLower(providerID)
	if prov, ok := bm.Config.Providers[providerID]; ok {
		lowerModel := strings.ToLower(modelID)
		for _, entry := range prov.Entries {
			if strings.Contains(lowerModel, strings.ToLower(entry.Prefix)) {
				inputPrice = entry.InputPerM
				outputPrice = entry.OutputPerM
				break // first match wins
			}
		}
	}

	cost := (float64(inputTokens)/1_000_000.0)*inputPrice + (float64(outputTokens)/1_000_000.0)*outputPrice

	if _, ok := bm.Session[modelID]; !ok {
		bm.Session[modelID] = &SessionUsage{}
	}

	bm.Session[modelID].InputTokens += inputTokens
	bm.Session[modelID].OutputTokens += outputTokens
	bm.Session[modelID].TotalCost += cost
	bm.TotalSession += cost
}

func (bm *BillingManager) GetTotalSessionCost() float64 {
	bm.Mu.RLock()
	defer bm.Mu.RUnlock()
	return bm.TotalSession
}

// CalculateCost computes the cost for a turn without modifying state
func (bm *BillingManager) CalculateCost(providerID, modelID string, inputTokens, outputTokens int) float64 {
	bm.Mu.RLock()
	defer bm.Mu.RUnlock()

	var inputPrice, outputPrice float64

	// Find pricing
	providerID = strings.ToLower(providerID)
	if prov, ok := bm.Config.Providers[providerID]; ok {
		lowerModel := strings.ToLower(modelID)
		for _, entry := range prov.Entries {
			if strings.Contains(lowerModel, strings.ToLower(entry.Prefix)) {
				inputPrice = entry.InputPerM
				outputPrice = entry.OutputPerM
				break
			}
		}
	}

	return (float64(inputTokens)/1_000_000.0)*inputPrice + (float64(outputTokens)/1_000_000.0)*outputPrice
}

func (bm *BillingManager) GetCostString() string {
	cost := bm.GetTotalSessionCost()
	if cost == 0 {
		return "$0.00"
	}
	if cost < 0.01 {
		return fmt.Sprintf("$%.4f", cost)
	}
	return fmt.Sprintf("$%.2f", cost)
}

// GetSessionReport returns a formatted markdown report of current session usage.
func (bm *BillingManager) GetSessionReport() string {
	bm.Mu.RLock()
	defer bm.Mu.RUnlock()

	if len(bm.Session) == 0 {
		return "No usage recorded this session."
	}

	var sb strings.Builder
	sb.WriteString("## Session Usage\n\n")
	sb.WriteString("| Model | Input Tokens | Output Tokens | Cost |\n")
	sb.WriteString("|-------|-------------|---------------|------|\n")
	for modelID, u := range bm.Session {
		sb.WriteString(fmt.Sprintf("| %s | %d | %d | $%.4f |\n",
			modelID, u.InputTokens, u.OutputTokens, u.TotalCost))
	}
	sb.WriteString(fmt.Sprintf("\n**Session Total: $%.4f**\n", bm.TotalSession))
	return sb.String()
}

// GetAllTimeReport reads usage_history.jsonl and returns a formatted all-time summary.
func (bm *BillingManager) GetAllTimeReport() string {
	logPath := filepath.Join(bm.configDir, "usage_history.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "No usage history found. History is saved at session end."
		}
		return fmt.Sprintf("Error reading usage history: %v", err)
	}

	type entry struct {
		Date      string                   `json:"date"`
		TotalCost float64                  `json:"total_cost"`
		Models    map[string]*SessionUsage `json:"models"`
	}

	var total float64
	modelTotals := make(map[string]SessionUsage)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		var e entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		total += e.TotalCost
		for modelID, u := range e.Models {
			existing := modelTotals[modelID]
			existing.InputTokens += u.InputTokens
			existing.OutputTokens += u.OutputTokens
			existing.TotalCost += u.TotalCost
			modelTotals[modelID] = existing
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## All-Time Usage (%d sessions)\n\n", len(lines)))
	sb.WriteString("| Model | Total Input | Total Output | Total Cost |\n")
	sb.WriteString("|-------|------------|--------------|------------|\n")
	for modelID, u := range modelTotals {
		sb.WriteString(fmt.Sprintf("| %s | %d | %d | $%.4f |\n",
			modelID, u.InputTokens, u.OutputTokens, u.TotalCost))
	}
	sb.WriteString(fmt.Sprintf("\n**All-Time Total: $%.4f**\n", total))
	return sb.String()
}

func (bm *BillingManager) SaveDailyLog() {
	bm.Mu.RLock()
	defer bm.Mu.RUnlock()

	if bm.TotalSession <= 0 {
		return
	}

	logPath := filepath.Join(bm.configDir, "usage_history.jsonl")

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	entry := map[string]interface{}{
		"date":       time.Now().Format(time.RFC3339),
		"total_cost": bm.TotalSession,
		"models":     bm.Session,
	}

	b, _ := json.Marshal(entry)
	f.Write(append(b, '\n'))
}
