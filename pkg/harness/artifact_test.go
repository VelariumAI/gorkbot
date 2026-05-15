package harness

import (
	"errors"
	"strings"
	"testing"
)

func TestArtifactValidation(t *testing.T) {
	a := Artifact{
		ID:      "artifact-1",
		Kind:    ArtifactKindText,
		Name:    "tool.shell",
		Content: "echo hello",
		Metadata: map[string]string{
			"safe_key": "safe_value",
		},
	}
	if err := a.Validate(); err != nil {
		t.Fatalf("expected valid artifact, got %v", err)
	}
}

func TestArtifactContentSizeBound(t *testing.T) {
	a := Artifact{
		ID:      "artifact-oversized",
		Kind:    ArtifactKindText,
		Content: strings.Repeat("x", maxArtifactContentSize+1),
	}
	err := a.Validate()
	if err == nil {
		t.Fatalf("expected oversized artifact error")
	}
	if !errors.Is(err, ErrArtifactTooLarge) {
		t.Fatalf("expected ErrArtifactTooLarge, got %v", err)
	}
}

func TestArtifactMetadataBoundAndRedaction(t *testing.T) {
	a := Artifact{
		ID:   "artifact-meta",
		Kind: ArtifactKindText,
		Metadata: map[string]string{
			"api_key":       "secret-value",
			"session_token": "session-secret",
			"ok":            "value",
		},
	}
	norm := a.Normalized()
	if got := norm.Metadata["api_key"]; got != "[REDACTED]" {
		t.Fatalf("expected api_key redacted, got %q", got)
	}
	if got := norm.Metadata["session_token"]; got != "[REDACTED]" {
		t.Fatalf("expected session_token redacted, got %q", got)
	}
	if got := norm.Metadata["ok"]; got != "value" {
		t.Fatalf("expected normal metadata preserved, got %q", got)
	}
}

func TestArtifactMalformedNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected panic: %v", r)
		}
	}()

	_ = (Artifact{}).Normalized()
	_ = (Artifact{ID: "x", Kind: ArtifactKind("bad-kind")}).Validate()
}
