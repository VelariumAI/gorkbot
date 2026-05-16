package profile

import "testing"

func TestConfigRefStableAndBounded(t *testing.T) {
	cfg := DefaultConfig(ProfileExpert)
	cfg.CustomProfileConfigured = true
	cfg.Metadata = map[string]string{"k": "v"}

	ref1 := ConfigRef(cfg)
	ref2 := ConfigRef(cfg)
	if ref1.Ref != ref2.Ref || ref1.Hash != ref2.Hash {
		t.Fatalf("config refs must be stable, ref1=%+v ref2=%+v", ref1, ref2)
	}
	if len(ref1.Ref) > 256 {
		t.Fatalf("config ref must be bounded, len=%d", len(ref1.Ref))
	}
}

func TestConfigRefChangesWithConfig(t *testing.T) {
	cfg1 := DefaultConfig(ProfileStandard)
	cfg2 := DefaultConfig(ProfilePowerUser)
	if ConfigRef(cfg1).Hash == ConfigRef(cfg2).Hash {
		t.Fatal("different configs should not share same config ref hash")
	}
}
