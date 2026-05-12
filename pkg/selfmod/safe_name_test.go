package selfmod

import "testing"

func TestValidateSafeArtifactNameAccepts(t *testing.T) {
	for _, name := range []string{"safe_tool", "tool42", "my-tool", "abc_def_42"} {
		if err := ValidateSafeArtifactName(name); err != nil {
			t.Fatalf("expected %q to be accepted, got %v", name, err)
		}
	}
}

func TestValidateSafeArtifactNameRejects(t *testing.T) {
	cases := []string{
		"",
		"   ",
		"../escape",
		"..\\escape",
		"foo/bar",
		"foo\\bar",
		"foo\x00bar",
		"foo\nbar",
		".hidden",
		"-flag",
		"abc..def",
	}
	for _, name := range cases {
		if err := ValidateSafeArtifactName(name); err == nil {
			t.Fatalf("expected %q to be rejected", name)
		}
	}
}

func TestValidateStagedTargetPathRejectsConstructedTraversal(t *testing.T) {
	_, blocked, reason, _ := ValidateStagedTargetPath(".gorkbot/staging/tools/../../../pkg/governance/policy.go")
	if !blocked || (reason != REASON_DYNAMIC_PATH_TRAVERSAL && reason != REASON_DYNAMIC_PROTECTED_TARGET) {
		t.Fatalf("expected traversal block, got blocked=%v reason=%s", blocked, reason)
	}

	_, blocked, reason, _ = ValidateStagedTargetPath("pkg/governance/policy.go")
	if !blocked || reason != REASON_DYNAMIC_PROTECTED_TARGET {
		t.Fatalf("expected protected block, got blocked=%v reason=%s", blocked, reason)
	}
	_, blocked, reason, _ = ValidateStagedTargetPath(".gorkbot/staging/tools/ok.go")
	if blocked || reason != "" {
		t.Fatalf("expected staged path allowed, got blocked=%v reason=%s", blocked, reason)
	}
}
