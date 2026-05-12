package puteradapter

import "testing"

func TestValidatePuterWorkspacePath_AllowsExpectedAreas(t *testing.T) {
	manifest := DefaultWorkspaceManifest()
	cases := []string{
		"/Gorkbot/scratch/run-1.txt",
		"/Gorkbot/experiments/exp-a/notes.md",
		"/Gorkbot/apps/demo/index.html",
	}
	for _, raw := range cases {
		safe, decision := ValidatePuterWorkspacePath(raw, manifest)
		if !decision.Allowed {
			t.Fatalf("expected path %q to be allowed, reason=%s", raw, decision.ReasonCode)
		}
		if safe.String() == "" {
			t.Fatalf("expected normalized path for %q", raw)
		}
	}
}

func TestValidatePuterWorkspacePath_BlocksTraversalOutsideAndEmpty(t *testing.T) {
	manifest := DefaultWorkspaceManifest()
	cases := []struct {
		raw        string
		reasonCode string
	}{
		{"", ReasonInvalidPath},
		{"../escape.txt", ReasonOutsideWorkspaceRoot},
		{"/tmp/outside.txt", ReasonOutsideWorkspaceRoot},
	}
	for _, tc := range cases {
		_, decision := ValidatePuterWorkspacePath(tc.raw, manifest)
		if decision.Allowed {
			t.Fatalf("expected %q blocked", tc.raw)
		}
		if decision.ReasonCode != tc.reasonCode {
			t.Fatalf("%q reason=%s want=%s", tc.raw, decision.ReasonCode, tc.reasonCode)
		}
	}
}

func TestValidatePuterWorkspacePath_NormalizesDuplicateSlash(t *testing.T) {
	manifest := DefaultWorkspaceManifest()
	safe, decision := ValidatePuterWorkspacePath("/Gorkbot//scratch///a.txt", manifest)
	if !decision.Allowed {
		t.Fatalf("expected duplicate slash path allowed, got %s", decision.ReasonCode)
	}
	if got, want := safe.String(), "/Gorkbot/scratch/a.txt"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestValidatePuterWorkspacePath_BlocksControlCharacters(t *testing.T) {
	manifest := DefaultWorkspaceManifest()
	_, decision := ValidatePuterWorkspacePath("/Gorkbot/scratch/bad\x00name", manifest)
	if decision.Allowed {
		t.Fatalf("expected control-char path blocked")
	}
	if decision.ReasonCode != ReasonControlCharacterBlocked {
		t.Fatalf("got reason %s", decision.ReasonCode)
	}
}
