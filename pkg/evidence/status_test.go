package evidence

import "testing"

func TestNormalizeStatus(t *testing.T) {
	if got := NormalizeStatus("PASS"); got != StatusPass {
		t.Fatalf("expected pass, got %q", got)
	}
	if got := NormalizeStatus("   "); got != StatusUnknown {
		t.Fatalf("expected unknown for empty input, got %q", got)
	}
	if got := NormalizeStatus("bogus"); got != StatusInvalid {
		t.Fatalf("expected invalid for unknown input, got %q", got)
	}
}

func TestStatusValidate(t *testing.T) {
	if err := StatusPass.Validate(); err != nil {
		t.Fatalf("pass should validate: %v", err)
	}
	if err := StatusUnknown.Validate(); err == nil {
		t.Fatal("unknown should fail validation")
	}
	if err := StatusInvalid.Validate(); err == nil {
		t.Fatal("invalid should fail validation")
	}
}
