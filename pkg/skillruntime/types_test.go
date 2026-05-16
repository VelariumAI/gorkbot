package skillruntime

import (
	"strings"
	"testing"

	"github.com/velariumai/gorkbot/pkg/evidence"
	"github.com/velariumai/gorkbot/pkg/profile"
	"github.com/velariumai/gorkbot/pkg/trace"
)

func TestOperationNormalizationAndValidation(t *testing.T) {
	cases := []struct {
		raw  string
		want Operation
	}{
		{"retrieve", OperationRetrieve},
		{" ReTrIeVe ", OperationRetrieve},
		{"propose", OperationPropose},
		{"validate", OperationValidate},
		{"stage", OperationStage},
		{"promote", OperationPromote},
		{"disable", OperationDisable},
		{"", OperationUnknown},
		{"bogus", OperationUnknown},
	}
	for _, tc := range cases {
		got := NormalizeOperation(tc.raw)
		if got != tc.want {
			t.Fatalf("raw=%q got=%q want=%q", tc.raw, got, tc.want)
		}
	}

	if err := OperationUnknown.Validate(); err == nil {
		t.Fatalf("expected unknown operation validation error")
	}
	if err := OperationRetrieve.Validate(); err != nil {
		t.Fatalf("unexpected operation validation error: %v", err)
	}
}

func TestCandidateNormalizedStableIDBoundsAndRedaction(t *testing.T) {
	raw := Candidate{
		Name:           strings.Repeat("n", 300),
		Version:        strings.Repeat("v", 300),
		Source:         "source-label\nraw-body-should-not-survive",
		Summary:        strings.Repeat("s", 800),
		Risk:           evidence.Risk("SENSITIVE"),
		OperationClass: evidence.SensitiveOperation("SHELL_EXECUTION"),
		Profile: func() profile.Profile {
			cfg := profile.DefaultConfig(profile.ProfileCustom)
			cfg.CustomProfileConfigured = true
			return cfg.Profile
		}(),
		ArtifactRefs: []trace.Ref{
			trace.NewRef("artifact", strings.Repeat("a", 600), strings.Repeat("h", 600), -10),
		},
		EvidenceRefs: []trace.Ref{
			trace.NewRef("evidence", "ev:1", "h1", 4),
		},
		Metadata: map[string]string{
			"prompt":       "do not keep raw prompt",
			"model_output": "do not keep raw output",
			"token":        "secret-token",
			"ok":           "value",
		},
	}

	n := raw.Normalized()
	if n.ID == "" {
		t.Fatalf("expected stable candidate id")
	}
	if len(n.Name) > 128 {
		t.Fatalf("name not bounded: %d", len(n.Name))
	}
	if len(n.Version) > 64 {
		t.Fatalf("version not bounded: %d", len(n.Version))
	}
	if strings.Contains(n.Source, "\n") {
		t.Fatalf("source should not preserve raw multiline content: %q", n.Source)
	}
	if len(n.Summary) > 256 {
		t.Fatalf("summary not bounded: %d", len(n.Summary))
	}
	if n.Risk != evidence.RiskSensitive {
		t.Fatalf("risk not normalized: %q", n.Risk)
	}
	if n.OperationClass != evidence.SensitiveShellExecution {
		t.Fatalf("operation class not normalized: %q", n.OperationClass)
	}
	if n.Metadata["token"] != "[REDACTED]" {
		t.Fatalf("sensitive metadata expected redaction")
	}
	if _, ok := n.Metadata["prompt"]; ok {
		t.Fatalf("prompt metadata should be stripped")
	}
	if _, ok := n.Metadata["model_output"]; ok {
		t.Fatalf("model_output metadata should be stripped")
	}
	if n.Metadata["ok"] == "" {
		t.Fatalf("non-sensitive metadata should remain")
	}

	n2 := raw.Normalized()
	if n.ID != n2.ID {
		t.Fatalf("candidate id should be stable across normalization")
	}
	if err := n.Validate(); err != nil {
		t.Fatalf("expected valid candidate, got error: %v", err)
	}
}

func TestCandidateValidateRejectsMissingIdentity(t *testing.T) {
	if err := (Candidate{}).Validate(); err == nil {
		t.Fatalf("expected validation error for empty candidate")
	}
}

func TestStatusNormalization(t *testing.T) {
	if got := NormalizeStatus(" APPROVAL_REQUIRED "); got != StatusApprovalRequired {
		t.Fatalf("unexpected status normalize: %q", got)
	}
	if got := NormalizeStatus("bogus"); got != StatusInvalid {
		t.Fatalf("unexpected status for bogus: %q", got)
	}
}
