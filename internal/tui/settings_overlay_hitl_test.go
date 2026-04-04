package tui

import "testing"

func TestCurrentWhitelistProfileIndex(t *testing.T) {
	if idx := currentWhitelistProfileIndex(nil); idx != 0 {
		t.Fatalf("expected strict profile index 0, got %d", idx)
	}
	if idx := currentWhitelistProfileIndex([]string{"list_directory", "read_file", "grep_content"}); idx != 1 {
		t.Fatalf("expected readonly profile index 1, got %d", idx)
	}
	if idx := currentWhitelistProfileIndex([]string{"unknown_tool"}); idx != 0 {
		t.Fatalf("expected unknown profile to fall back to strict index 0, got %d", idx)
	}
}

func TestCycleHITLWhitelistProfile(t *testing.T) {
	s := &SettingsOverlay{}

	if profile := s.cycleHITLWhitelistProfile(); profile != "readonly" {
		t.Fatalf("expected readonly profile, got %q", profile)
	}
	if len(s.hitlWhitelistedTools) != 3 {
		t.Fatalf("expected readonly tools length 3, got %d", len(s.hitlWhitelistedTools))
	}

	if profile := s.cycleHITLWhitelistProfile(); profile != "editor" {
		t.Fatalf("expected editor profile, got %q", profile)
	}
	if profile := s.cycleHITLWhitelistProfile(); profile != "power" {
		t.Fatalf("expected power profile, got %q", profile)
	}
	if profile := s.cycleHITLWhitelistProfile(); profile != "strict (none)" {
		t.Fatalf("expected strict profile, got %q", profile)
	}
	if len(s.hitlWhitelistedTools) != 0 {
		t.Fatalf("expected strict profile to clear whitelist, got %#v", s.hitlWhitelistedTools)
	}
}
