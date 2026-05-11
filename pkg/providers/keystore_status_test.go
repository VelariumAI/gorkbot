package providers

import (
	"strings"
	"testing"
)

func TestKeyStatusString(t *testing.T) {
	if KeyStatusValid.String() != "valid" {
		t.Fatalf("expected valid")
	}
	if KeyStatusInvalid.String() != "invalid" {
		t.Fatalf("expected invalid")
	}
	if KeyStatusUnverified.String() != "unverified" {
		t.Fatalf("expected unverified")
	}
	if KeyStatusMissing.String() != "missing" {
		t.Fatalf("expected missing")
	}
}

func TestKeyStoreStatusAndGetKey(t *testing.T) {
	ks := NewKeyStore(t.TempDir())
	if err := ks.Set(ProviderOpenAI, "sk-test"); err != nil {
		t.Fatalf("set key: %v", err)
	}
	if got := ks.GetKey(ProviderOpenAI); got != "sk-test" {
		t.Fatalf("expected GetKey to return saved key")
	}

	line := ks.StatusLine()
	if len(line) != len(AllProviders()) {
		t.Fatalf("expected status line for all providers")
	}

	formatted := ks.FormatStatus()
	if !strings.Contains(formatted, ProviderOpenAI) {
		t.Fatalf("expected formatted status to include provider name")
	}
	if !strings.Contains(formatted, "unverified") {
		t.Fatalf("expected formatted status to include key status")
	}
}

func TestAllProvidersReturnsCopy_StatusSuite(t *testing.T) {
	a := AllProviders()
	if len(a) == 0 {
		t.Fatalf("expected providers list")
	}
	orig := a[0]
	a[0] = "mutated"
	b := AllProviders()
	if b[0] != orig {
		t.Fatalf("expected AllProviders to return a copy")
	}
}
