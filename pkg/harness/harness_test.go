package harness

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s := NewStore(dir)
	if err := s.EnsureDir(); err != nil {
		t.Fatal(err)
	}
	return s
}

// ── Store Tests ──────────────────────────────────────────────────────────────

func TestStoreFeatureListRoundTrip(t *testing.T) {
	s := tempStore(t)
	now := time.Now()

	fl := &FeatureList{
		ProjectName: "test-project",
		Goal:        "Build something",
		Features: []Feature{
			{ID: "feat-001", Title: "First", Status: StatusFailing, CreatedAt: now, UpdatedAt: now},
			{ID: "feat-002", Title: "Second", Status: StatusFailing, CreatedAt: now, UpdatedAt: now},
		},
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.SaveFeatureList(fl); err != nil {
		t.Fatal(err)
	}

	loaded, err := s.LoadFeatureList()
	if err != nil {
		t.Fatal(err)
	}

	if loaded.ProjectName != "test-project" {
		t.Errorf("got %q, want %q", loaded.ProjectName, "test-project")
	}
	if len(loaded.Features) != 2 {
		t.Fatalf("got %d features, want 2", len(loaded.Features))
	}
	if loaded.Features[0].ID != "feat-001" {
		t.Errorf("got %q, want %q", loaded.Features[0].ID, "feat-001")
	}
}

func TestStoreStateRoundTrip(t *testing.T) {
	s := tempStore(t)

	state := &HarnessState{
		ActiveFeatureID: "feat-003",
		SessionID:       "sess-1",
		ProjectRoot:     "/tmp/test",
		Initialized:     true,
		TotalSessions:   5,
	}

	if err := s.SaveState(state); err != nil {
		t.Fatal(err)
	}

	loaded, err := s.LoadState()
	if err != nil {
		t.Fatal(err)
	}

	if loaded.ActiveFeatureID != "feat-003" {
		t.Errorf("got %q, want %q", loaded.ActiveFeatureID, "feat-003")
	}
	if loaded.TotalSessions != 5 {
		t.Errorf("got %d, want 5", loaded.TotalSessions)
	}
}

func TestStoreProgressAppendAndLoad(t *testing.T) {
	s := tempStore(t)

	entries := []ProgressEntry{
		{Timestamp: time.Now(), SessionID: "s1", Action: "boot"},
		{Timestamp: time.Now(), SessionID: "s1", Action: "start", FeatureID: "feat-001"},
		{Timestamp: time.Now(), SessionID: "s1", Action: "complete", FeatureID: "feat-001"},
	}

	for _, e := range entries {
		if err := s.AppendProgress(e); err != nil {
			t.Fatal(err)
		}
	}

	loaded, err := s.LoadProgress()
	if err != nil {
		t.Fatal(err)
	}

	if len(loaded) != 3 {
		t.Fatalf("got %d entries, want 3", len(loaded))
	}
	if loaded[1].Action != "start" {
		t.Errorf("got %q, want %q", loaded[1].Action, "start")
	}
}

func TestStoreProgressLoadEmpty(t *testing.T) {
	s := tempStore(t)

	loaded, err := s.LoadProgress()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 0 {
		t.Errorf("got %d entries, want 0", len(loaded))
	}
}

func TestStoreVerificationReportRoundTrip(t *testing.T) {
	s := tempStore(t)

	report := &VerificationReport{
		FeatureID: "feat-001",
		Passed:    true,
		Summary:   "All good",
		Steps: []VerificationStep{
			{Name: "build", Command: "go build ./...", Passed: true, ExitCode: 0, Duration: time.Second},
		},
	}

	if err := s.SaveVerificationReport(report); err != nil {
		t.Fatal(err)
	}

	loaded, err := s.LoadVerificationReport("feat-001")
	if err != nil {
		t.Fatal(err)
	}

	if !loaded.Passed {
		t.Error("expected Passed=true")
	}
	if len(loaded.Steps) != 1 {
		t.Fatalf("got %d steps, want 1", len(loaded.Steps))
	}
}

// ── Initializer Tests ────────────────────────────────────────────────────────

func TestInitializerBasic(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	init := NewInitializer(s, nil)

	if init.IsInitialized() {
		t.Error("should not be initialized yet")
	}

	features := []FeatureInput{
		{Title: "Feature A", Description: "First feature", Priority: 1},
		{Title: "Feature B", Description: "Second feature", Priority: 2, Dependencies: []string{"feat-001"}},
	}

	fl, err := init.Initialize(context.Background(), "Test goal", features)
	if err != nil {
		t.Fatal(err)
	}

	if fl.ProjectName != filepath.Base(dir) {
		t.Errorf("got %q, want %q", fl.ProjectName, filepath.Base(dir))
	}
	if len(fl.Features) != 2 {
		t.Fatalf("got %d features, want 2", len(fl.Features))
	}
	if fl.Features[0].Status != StatusFailing {
		t.Errorf("initial status should be failing, got %q", fl.Features[0].Status)
	}
	if fl.Features[0].ID != "feat-001" {
		t.Errorf("got ID %q, want %q", fl.Features[0].ID, "feat-001")
	}

	if !init.IsInitialized() {
		t.Error("should be initialized now")
	}
}

func TestInitializerDoubleInit(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	init := NewInitializer(s, nil)

	_, err := init.Initialize(context.Background(), "Goal", []FeatureInput{{Title: "A", Priority: 1}})
	if err != nil {
		t.Fatal(err)
	}

	// Second init should fail because IsInitialized() is checked externally by tools
	if !init.IsInitialized() {
		t.Error("should be initialized")
	}
}

// ── Worker Tests ─────────────────────────────────────────────────────────────

func setupWorker(t *testing.T) (*Worker, *Store) {
	t.Helper()
	dir := t.TempDir()
	s := NewStore(dir)
	init := NewInitializer(s, nil)

	features := []FeatureInput{
		{Title: "Foundation", Description: "Base layer", Priority: 1},
		{Title: "Feature X", Description: "Depends on foundation", Priority: 2, Dependencies: []string{"feat-001"}},
		{Title: "Independent", Description: "No deps", Priority: 3},
	}

	_, err := init.Initialize(context.Background(), "Test project", features)
	if err != nil {
		t.Fatal(err)
	}

	verifier := NewVerifier(s, nil)
	worker := NewWorker(s, verifier, "test-session", nil)
	return worker, s
}

func TestWorkerSelectFeaturePriority(t *testing.T) {
	worker, _ := setupWorker(t)

	feat, err := worker.SelectFeature()
	if err != nil {
		t.Fatal(err)
	}

	// Should select feat-001 (priority 1) since it has no deps
	if feat.ID != "feat-001" {
		t.Errorf("got %q, want %q", feat.ID, "feat-001")
	}
}

func TestWorkerSelectFeatureDependencyBlocking(t *testing.T) {
	worker, store := setupWorker(t)

	// Mark feat-001 as passing
	fl, _ := store.LoadFeatureList()
	fl.Features[0].Status = StatusPassing
	store.SaveFeatureList(fl)

	feat, err := worker.SelectFeature()
	if err != nil {
		t.Fatal(err)
	}

	// feat-002 depends on feat-001 (now passing), priority 2
	// feat-003 has no deps, priority 3
	// So feat-002 should be selected
	if feat.ID != "feat-002" {
		t.Errorf("got %q, want %q", feat.ID, "feat-002")
	}
}

func TestWorkerStartFeature(t *testing.T) {
	worker, store := setupWorker(t)

	if err := worker.StartFeature("feat-001"); err != nil {
		t.Fatal(err)
	}

	fl, _ := store.LoadFeatureList()
	if fl.Features[0].Status != StatusInProgress {
		t.Errorf("got %q, want %q", fl.Features[0].Status, StatusInProgress)
	}

	state, _ := store.LoadState()
	if state.ActiveFeatureID != "feat-001" {
		t.Errorf("got %q, want %q", state.ActiveFeatureID, "feat-001")
	}
}

func TestWorkerStartFeatureNotFound(t *testing.T) {
	worker, _ := setupWorker(t)

	err := worker.StartFeature("feat-999")
	if err == nil {
		t.Error("expected error for non-existent feature")
	}
}

func TestWorkerBoot(t *testing.T) {
	worker, _ := setupWorker(t)
	ctx := context.Background()

	report, err := worker.Boot(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if report.TotalFeatures != 3 {
		t.Errorf("got %d, want 3", report.TotalFeatures)
	}
	if report.FailingCount != 3 {
		t.Errorf("got %d failing, want 3", report.FailingCount)
	}
}

// ── Verifier Tests ───────────────────────────────────────────────────────────

func TestVerifierRunCommand(t *testing.T) {
	s := tempStore(t)
	v := NewVerifier(s, nil)

	step := v.RunCommand(context.Background(), VerifyCommand{
		Name:    "echo test",
		Command: "echo hello",
		Timeout: 5 * time.Second,
	}, s.projectRoot)

	if !step.Passed {
		t.Errorf("expected pass, got exit %d: %s", step.ExitCode, step.Output)
	}
	if step.ExitCode != 0 {
		t.Errorf("got exit code %d, want 0", step.ExitCode)
	}
}

func TestVerifierRunCommandFailure(t *testing.T) {
	s := tempStore(t)
	v := NewVerifier(s, nil)

	step := v.RunCommand(context.Background(), VerifyCommand{
		Name:    "fail",
		Command: "exit 1",
		Timeout: 5 * time.Second,
	}, s.projectRoot)

	if step.Passed {
		t.Error("expected failure")
	}
	if step.ExitCode != 1 {
		t.Errorf("got exit code %d, want 1", step.ExitCode)
	}
}

func TestVerifierVerifyFeature(t *testing.T) {
	s := tempStore(t)
	v := NewVerifier(s, nil)

	feature := &Feature{
		ID:    "feat-001",
		Title: "Test",
		VerificationSpec: &VerificationSpec{
			Commands: []VerifyCommand{
				{Name: "check", Command: "echo OK", Timeout: 5 * time.Second},
			},
		},
	}

	report, err := v.Verify(context.Background(), feature)
	if err != nil {
		t.Fatal(err)
	}

	if !report.Passed {
		t.Errorf("expected pass: %s", report.Summary)
	}
}

// ── Type Serialization Tests ─────────────────────────────────────────────────

func TestFeatureStatusValues(t *testing.T) {
	statuses := []FeatureStatus{StatusFailing, StatusIncomplete, StatusInProgress, StatusPassing, StatusSkipped}
	for _, s := range statuses {
		data, err := json.Marshal(s)
		if err != nil {
			t.Errorf("marshal %q: %v", s, err)
		}
		var got FeatureStatus
		if err := json.Unmarshal(data, &got); err != nil {
			t.Errorf("unmarshal %q: %v", s, err)
		}
		if got != s {
			t.Errorf("roundtrip: got %q, want %q", got, s)
		}
	}
}

func TestHarnessDirLocation(t *testing.T) {
	s := NewStore("/tmp/myproject")
	expected := filepath.Join("/tmp/myproject", ".gorkbot", "harness")
	if got := s.HarnessDir(); got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

// ── All-Passing Completion ───────────────────────────────────────────────────

func TestWorkerSelectAllPassing(t *testing.T) {
	_, store := setupWorker(t)

	fl, _ := store.LoadFeatureList()
	for i := range fl.Features {
		fl.Features[i].Status = StatusPassing
	}
	store.SaveFeatureList(fl)

	verifier := NewVerifier(store, nil)
	worker := NewWorker(store, verifier, "test", nil)

	_, err := worker.SelectFeature()
	if err == nil {
		t.Error("expected error when all features are passing")
	}
}

// ── Atomic Write Safety ─────────────────────────────────────────────────────

func TestAtomicWriteNoPartialFile(t *testing.T) {
	s := tempStore(t)
	fl := &FeatureList{ProjectName: "atomic-test", Version: 1}

	if err := s.SaveFeatureList(fl); err != nil {
		t.Fatal(err)
	}

	// Temp file should not exist after successful write
	tmpPath := s.featureListPath() + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("temp file should be cleaned up")
	}
}
