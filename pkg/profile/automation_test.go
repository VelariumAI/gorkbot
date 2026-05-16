package profile

import "testing"

func TestAutoPromotionDisabledByDefault(t *testing.T) {
	profiles := []Profile{ProfileBeginner, ProfileStandard, ProfileEnterprise}
	for _, p := range profiles {
		cfg := DefaultConfig(p)
		if cfg.Automation.AutoPromotionMode != AutomationDisabled {
			t.Fatalf("%s auto promotion must be disabled by default, got %q", p, cfg.Automation.AutoPromotionMode)
		}
	}
}

func TestAutoPromotionConfigurableFutureUse(t *testing.T) {
	cfg := DefaultConfig(ProfileExpert)
	if cfg.Automation.AutoPromotionMode != AutomationAllowConfigured {
		t.Fatalf("expert should allow configured auto promotion, got %q", cfg.Automation.AutoPromotionMode)
	}
}

func TestPlannerMutationDefaultsConservative(t *testing.T) {
	std := DefaultConfig(ProfileStandard)
	if std.Automation.PlannerMutation != AutomationDisabled {
		t.Fatalf("standard planner mutation should be disabled, got %q", std.Automation.PlannerMutation)
	}
	power := DefaultConfig(ProfilePowerUser)
	if power.Automation.PlannerMutation != AutomationSessionLocal {
		t.Fatalf("power user planner mutation should be session-local, got %q", power.Automation.PlannerMutation)
	}
}
