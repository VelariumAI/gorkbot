package safety

import (
	"log/slog"
	"os"
	"testing"
)

// TestCreateSnapshot tests file snapshot creation
func TestCreateSnapshot(t *testing.T) {
	logger := slog.Default()
	fv := NewFileValidator(logger, 5)

	// Create temp file
	tmpFile, err := os.CreateTemp("", "test_*.go")
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := "line 1\nline 2\nline 3\nline 4\nline 5\n"
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	tmpFile.Close()

	// Create snapshot
	snapshot, err := fv.CreateSnapshot(tmpFile.Name())
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	if snapshot.TotalLines != 6 { // 5 lines + empty after final newline
		t.Errorf("Expected 6 lines, got %d", snapshot.TotalLines)
	}

	if snapshot.FullHash == "" {
		t.Error("FullHash should not be empty")
	}

	if len(snapshot.LineHashes) == 0 {
		t.Error("LineHashes should not be empty")
	}
}

// TestValidateEdit tests edit validation
func TestValidateEdit(t *testing.T) {
	logger := slog.Default()
	fv := NewFileValidator(logger, 5)

	// Create temp file
	tmpFile, err := os.CreateTemp("", "test_*.go")
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString("original line\n")
	tmpFile.Close()

	snapshot, _ := fv.CreateSnapshot(tmpFile.Name())

	// Test valid line
	lineHash := snapshot.LineHashes[0].Hash
	valid, msg := fv.ValidateEdit(snapshot, 1, lineHash)
	if !valid {
		t.Errorf("Valid edit should pass: %s", msg)
	}

	// Test invalid hash
	valid, msg = fv.ValidateEdit(snapshot, 1, "wrong_hash")
	if valid {
		t.Error("Invalid hash should fail")
	}

	// Test out of range
	valid, msg = fv.ValidateEdit(snapshot, 100, lineHash)
	if valid {
		t.Error("Out of range should fail")
	}
}

// TestDetectChanges tests change detection
func TestDetectChanges(t *testing.T) {
	logger := slog.Default()
	fv := NewFileValidator(logger, 5)

	// Create and modify temp file
	tmpFile, err := os.CreateTemp("", "test_*.go")
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString("line 1\nline 2\n")
	tmpFile.Close()

	snapshot1, _ := fv.CreateSnapshot(tmpFile.Name())

	// Modify file
	tmpFile2, _ := os.OpenFile(tmpFile.Name(), os.O_WRONLY|os.O_TRUNC, 0644)
	tmpFile2.WriteString("line 1\nmodified line 2\n")
	tmpFile2.Close()

	changes, err := fv.DetectChanges(snapshot1)
	if err != nil {
		t.Fatalf("DetectChanges failed: %v", err)
	}

	if len(changes) == 0 {
		t.Error("Should detect changes")
	}
}

// TestEditValidator tests the full edit validator
func TestEditValidator(t *testing.T) {
	logger := slog.Default()
	config := &EditValidationConfig{
		Enabled:      true,
		HashDepth:    3,
		StrictMode:   true,
		AutoFixAllowed: false,
	}
	validator := NewEditValidator(logger, config)

	// Create temp file
	tmpFile, _ := os.CreateTemp("", "test_*.go")
	tmpFile.WriteString("line 1\nline 2\nline 3\n")
	tmpFile.Close()

	// Capture snapshot
	snapshot, err := validator.CaptureSnapshot(tmpFile.Name())
	if err != nil {
		t.Fatalf("CaptureSnapshot failed: %v", err)
	}

	if snapshot == nil {
		t.Error("Snapshot should not be nil")
	}

	// Validate line
	lineHash := snapshot.LineHashes[0].Hash
	err = validator.ValidateLine(tmpFile.Name(), 1, lineHash)
	if err != nil {
		t.Errorf("ValidateLine failed: %v", err)
	}

	// Clean up
	os.Remove(tmpFile.Name())
}

// TestPrepareSafeEdit tests safe edit preparation
func TestPrepareSafeEdit(t *testing.T) {
	logger := slog.Default()
	validator := NewEditValidator(logger, nil)

	// Create temp file
	tmpFile, _ := os.CreateTemp("", "test_*.go")
	tmpFile.WriteString("original content\nline 2\n")
	tmpFile.Close()

	// Capture snapshot
	validator.CaptureSnapshot(tmpFile.Name())

	// Prepare safe edit
	safeEdit, err := validator.PrepareSafeEdit(tmpFile.Name(), 1, "modified content")
	if err != nil {
		t.Fatalf("PrepareSafeEdit failed: %v", err)
	}

	if safeEdit == nil {
		t.Error("SafeEdit should not be nil")
	}

	if safeEdit.OldContent != "original content" {
		t.Error("OldContent mismatch")
	}

	if safeEdit.NewContent != "modified content" {
		t.Error("NewContent mismatch")
	}

	os.Remove(tmpFile.Name())
}

// TestClearSnapshot tests snapshot cleanup
func TestClearSnapshot(t *testing.T) {
	logger := slog.Default()
	validator := NewEditValidator(logger, nil)

	tmpFile, _ := os.CreateTemp("", "test_*.go")
	tmpFile.WriteString("content\n")
	tmpFile.Close()

	validator.CaptureSnapshot(tmpFile.Name())

	if validator.GetSnapshotCount() != 1 {
		t.Error("Should have 1 snapshot")
	}

	validator.ClearSnapshot(tmpFile.Name())

	if validator.GetSnapshotCount() != 0 {
		t.Error("Should have 0 snapshots after clear")
	}

	os.Remove(tmpFile.Name())
}

// TestComputeDiff tests diff computation
func TestComputeDiff(t *testing.T) {
	logger := slog.Default()
	fv := NewFileValidator(logger, 5)

	// Create two versions
	tmpFile1, _ := os.CreateTemp("", "test_*.go")
	tmpFile1.WriteString("line 1\nline 2\nline 3\n")
	tmpFile1.Close()

	tmpFile2, _ := os.CreateTemp("", "test_*.go")
	tmpFile2.WriteString("line 1\nmodified 2\nline 3\nline 4\nline 5\n")
	tmpFile2.Close()

	snap1, _ := fv.CreateSnapshot(tmpFile1.Name())
	snap2, _ := fv.CreateSnapshot(tmpFile2.Name())

	diff := fv.ComputeDiff(snap1, snap2)

	if diff.LinesChanged == 0 {
		t.Error("Should detect changed lines")
	}

	if diff.LinesAdded == 0 {
		t.Error("Should detect added lines")
	}

	os.Remove(tmpFile1.Name())
	os.Remove(tmpFile2.Name())
}
