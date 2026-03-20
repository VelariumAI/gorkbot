package spark

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig("/tmp/test")
	if !cfg.Enabled {
		t.Error("Enabled should default to true")
	}
	if cfg.ConfigDir != "/tmp/test" {
		t.Errorf("ConfigDir mismatch: got %q", cfg.ConfigDir)
	}
	if cfg.MaxIDLEntries != 50 {
		t.Errorf("MaxIDLEntries should be 50, got %d", cfg.MaxIDLEntries)
	}
	if cfg.TIIAlpha != 0.1 {
		t.Errorf("TIIAlpha should be 0.1, got %f", cfg.TIIAlpha)
	}
	if cfg.DriveAlpha != 0.1 {
		t.Errorf("DriveAlpha should be 0.1, got %f", cfg.DriveAlpha)
	}
	if cfg.MinCycleInterval != 30*time.Second {
		t.Errorf("MinCycleInterval should be 30s, got %v", cfg.MinCycleInterval)
	}
	if cfg.ResearchObjectiveMax != 20 {
		t.Errorf("ResearchObjectiveMax should be 20, got %d", cfg.ResearchObjectiveMax)
	}
	if cfg.LLMObjectiveEnabled {
		t.Error("LLMObjectiveEnabled should default to false")
	}
}

func TestDirectiveKindConstants(t *testing.T) {
	if DirectiveRetry != 0 {
		t.Errorf("DirectiveRetry should be 0, got %d", DirectiveRetry)
	}
	if DirectiveFallback != 1 {
		t.Errorf("DirectiveFallback should be 1, got %d", DirectiveFallback)
	}
	if DirectivePromptFix != 2 {
		t.Errorf("DirectivePromptFix should be 2, got %d", DirectivePromptFix)
	}
	if DirectiveToolBan != 3 {
		t.Errorf("DirectiveToolBan should be 3, got %d", DirectiveToolBan)
	}
	if DirectiveResearch != 4 {
		t.Errorf("DirectiveResearch should be 4, got %d", DirectiveResearch)
	}
}
