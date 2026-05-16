package profile

import (
	"strings"
	"testing"
)

func TestCustomRequiresExplicitMarker(t *testing.T) {
	cfg := DefaultConfig(ProfileCustom)
	if err := cfg.Validate(); err == nil {
		t.Fatal("custom profile must require explicit marker")
	}
	cfg.CustomProfileConfigured = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("custom profile with explicit marker should validate: %v", err)
	}
}

func TestMetadataBoundsAndRedaction(t *testing.T) {
	cfg := DefaultConfig(ProfileStandard)
	cfg.Metadata = map[string]string{}
	for i := 0; i < 30; i++ {
		cfg.Metadata[strings.Repeat("k", 70)+string(rune('a'+(i%26)))+string(rune('a'+((i+1)%26)))] = strings.Repeat("v", 400)
	}
	cfg.Metadata["api_key"] = "secret-value"

	norm := cfg.Normalized()
	if len(norm.Metadata) > 16 {
		t.Fatalf("metadata must be bounded, got %d", len(norm.Metadata))
	}
	if got := norm.Metadata["api_key"]; got != "[REDACTED]" {
		t.Fatalf("sensitive metadata should be redacted, got %q", got)
	}
}

func TestMalformedInputNoPanic(t *testing.T) {
	cfg := Config{
		Profile:   Profile("BAD"),
		TraceMode: "not-a-mode",
		Authority: AuthorityConfig{ToolAuthority: AuthorityMode("???")},
	}
	norm := cfg.Normalized()
	if norm.Profile != ProfileBeginner {
		t.Fatalf("malformed profile should normalize conservatively, got %q", norm.Profile)
	}
}
