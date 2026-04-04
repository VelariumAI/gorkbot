package skills

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// MockRuleRegistrar is a test implementation of RuleRegistrar
type MockRuleRegistrar struct {
	addedRules []struct {
		decision string
		pattern  string
		comment  string
	}
	addRuleErr error
}

func (mrr *MockRuleRegistrar) AddRule(decision, pattern, comment string) error {
	if mrr.addRuleErr != nil {
		return mrr.addRuleErr
	}
	mrr.addedRules = append(mrr.addedRules, struct {
		decision string
		pattern  string
		comment  string
	}{decision, pattern, comment})
	return nil
}

// TestNewPersistentRegistry_CreatesRegistry tests persistent registry initialization
func TestNewPersistentRegistry_CreatesRegistry(t *testing.T) {
	tmpDir := t.TempDir()

	reg := NewPersistentRegistry(tmpDir, slog.Default())
	if reg == nil {
		t.Fatal("expected non-nil registry")
	}
	if reg.dir != tmpDir {
		t.Errorf("dir mismatch: got %s, want %s", reg.dir, tmpDir)
	}
}

// TestPersistentRegistry_Persist writes and reads enabled/disabled state
func TestPersistentRegistry_Persist(t *testing.T) {
	tmpDir := t.TempDir()
	reg := NewPersistentRegistry(tmpDir, slog.Default())

	// Register two skills
	skill1 := &SkillManifest{
		Name:    "skill_1",
		Version: "1.0.0",
		Enabled: true,
	}
	skill2 := &SkillManifest{
		Name:    "skill_2",
		Version: "1.0.0",
		Enabled: false,
	}

	reg.Register(skill1, "")
	reg.Register(skill2, "")

	// Persist index
	err := reg.persistIndex()
	if err != nil {
		t.Fatalf("persistIndex failed: %v", err)
	}

	// Verify index.json exists
	indexPath := filepath.Join(tmpDir, "index.json")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Fatal("index.json was not created")
	}

	// Create new registry and load index
	reg2 := NewPersistentRegistry(tmpDir, slog.Default())
	reg2.Register(skill1, "")
	reg2.Register(skill2, "")
	err = reg2.loadIndex()
	if err != nil {
		t.Fatalf("loadIndex failed: %v", err)
	}

	// Verify state was restored
	s1 := reg2.Get("skill_1")
	if s1 == nil || !s1.Manifest.Enabled {
		t.Error("skill_1 should be enabled")
	}
	s2 := reg2.Get("skill_2")
	if s2 == nil || s2.Manifest.Enabled {
		t.Error("skill_2 should be disabled")
	}
}

// TestPersistentRegistry_EnableRegistersPermissions tests permission registration on Enable
func TestPersistentRegistry_EnableRegistersPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	reg := NewPersistentRegistry(tmpDir, slog.Default())

	// Inject mock rule registrar
	mockRR := &MockRuleRegistrar{}
	reg.SetRuleRegistrar(mockRR)

	// Register skill with permissions
	skill := &SkillManifest{
		Name:    "secure_skill",
		Version: "1.0.0",
		Enabled: false,
		Permissions: []SkillPermission{
			{Tool: "write_file", Level: "once", Pattern: "path:/tmp/*"},
			{Tool: "bash", Level: "ask"},
		},
	}

	reg.Register(skill, "")

	// Enable the skill
	err := reg.Enable("secure_skill")
	if err != nil {
		t.Fatalf("Enable failed: %v", err)
	}

	// Verify permissions were registered
	if len(mockRR.addedRules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(mockRR.addedRules))
	}

	// Check first rule (with pattern)
	rule1 := mockRR.addedRules[0]
	if rule1.decision != "once" {
		t.Errorf("rule1 decision: got %s, want once", rule1.decision)
	}
	if rule1.pattern != "write_file(path:/tmp/*)" {
		t.Errorf("rule1 pattern: got %s, want write_file(path:/tmp/*)", rule1.pattern)
	}

	// Check second rule (without pattern)
	rule2 := mockRR.addedRules[1]
	if rule2.decision != "ask" {
		t.Errorf("rule2 decision: got %s, want ask", rule2.decision)
	}
	if rule2.pattern != "bash" {
		t.Errorf("rule2 pattern: got %s, want bash", rule2.pattern)
	}
}

// TestPersistentRegistry_EnablePersists tests that Enable() persists state
func TestPersistentRegistry_EnablePersists(t *testing.T) {
	tmpDir := t.TempDir()
	reg := NewPersistentRegistry(tmpDir, slog.Default())

	skill := &SkillManifest{
		Name:    "test_skill",
		Version: "1.0.0",
		Enabled: false,
	}
	reg.Register(skill, "")

	// Enable and verify persistence
	err := reg.Enable("test_skill")
	if err != nil {
		t.Fatalf("Enable failed: %v", err)
	}

	// Load index and verify
	reg2 := NewPersistentRegistry(tmpDir, slog.Default())
	reg2.Register(skill, "")
	if err := reg2.loadIndex(); err != nil {
		t.Fatalf("loadIndex failed: %v", err)
	}

	s := reg2.Get("test_skill")
	if s == nil || !s.Manifest.Enabled {
		t.Error("skill should be enabled after reload")
	}
}

// TestSetRuleRegistrar_Injects tests SetRuleRegistrar injection
func TestSetRuleRegistrar_Injects(t *testing.T) {
	reg := NewInMemoryRegistry(slog.Default())
	if reg.ruleRegistrar != nil {
		t.Error("initial ruleRegistrar should be nil")
	}

	mockRR := &MockRuleRegistrar{}
	reg.SetRuleRegistrar(mockRR)

	if reg.ruleRegistrar != mockRR {
		t.Error("ruleRegistrar not set correctly")
	}
}

// TestRecordUsage_MovingAverage tests skill usage recording
func TestRecordUsage_MovingAverage(t *testing.T) {
	reg := NewInMemoryRegistry(slog.Default())

	skill := &SkillManifest{
		Name:    "timed_skill",
		Version: "1.0.0",
	}
	reg.Register(skill, "")

	// Record usage with different times
	reg.RecordUsage("timed_skill", 100)
	s1 := reg.Get("timed_skill")
	if s1.UsageCount != 1 || s1.AvgExecutionTime != 100 {
		t.Error("first usage not recorded correctly")
	}

	reg.RecordUsage("timed_skill", 200)
	s2 := reg.Get("timed_skill")
	if s2.UsageCount != 2 {
		t.Errorf("usage count: got %d, want 2", s2.UsageCount)
	}
	expectedAvg := (100 + 200) / 2
	if s2.AvgExecutionTime != int64(expectedAvg) {
		t.Errorf("avg execution time: got %d, want %d", s2.AvgExecutionTime, expectedAvg)
	}
	if s2.LastUsed.IsZero() {
		t.Error("LastUsed should be set")
	}
}

// TestDisable_DisablesSkill tests disabling a skill
func TestDisable_DisablesSkill(t *testing.T) {
	reg := NewInMemoryRegistry(slog.Default())

	skill := &SkillManifest{
		Name:    "disableable",
		Version: "1.0.0",
		Enabled: true,
	}
	reg.Register(skill, "")

	err := reg.Disable("disableable")
	if err != nil {
		t.Fatalf("Disable failed: %v", err)
	}

	s := reg.Get("disableable")
	if s.Manifest.Enabled {
		t.Error("skill should be disabled")
	}
}

// BenchmarkPersistIndex benchmarks persistence performance
func BenchmarkPersistIndex(b *testing.B) {
	tmpDir := b.TempDir()
	reg := NewPersistentRegistry(tmpDir, slog.Default())

	// Register many skills
	for i := 0; i < 100; i++ {
		name := "skill_" + string(rune('0'+(i%10)))
		if i%2 == 0 {
			name += "_a"
		} else {
			name += "_b"
		}
		skill := &SkillManifest{
			Name:    name,
			Version: "1.0.0",
			Enabled: i%3 == 0,
		}
		reg.Register(skill, "")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = reg.persistIndex()
	}
}
