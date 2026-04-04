package safety

import (
	"fmt"
	"log/slog"
)

// EditValidationConfig contains configuration for edit validation
type EditValidationConfig struct {
	Enabled          bool
	HashDepth        int
	StrictMode       bool
	RequireContext   bool
	AutoFixAllowed   bool
}

// EditValidator validates edits before applying them
type EditValidator struct {
	logger       *slog.Logger
	fileValidator *FileValidator
	config       *EditValidationConfig
	snapshots    map[string]*FileSnapshot
}

// NewEditValidator creates a new edit validator
func NewEditValidator(logger *slog.Logger, config *EditValidationConfig) *EditValidator {
	if logger == nil {
		logger = slog.Default()
	}
	if config == nil {
		config = &EditValidationConfig{
			Enabled:        true,
			HashDepth:      5,
			StrictMode:     true,
			RequireContext: false,
			AutoFixAllowed: false,
		}
	}

	return &EditValidator{
		logger:        logger,
		fileValidator: NewFileValidator(logger, config.HashDepth),
		config:        config,
		snapshots:     make(map[string]*FileSnapshot),
	}
}

// CaptureSnapshot captures current file state before editing
func (ev *EditValidator) CaptureSnapshot(filePath string) (*FileSnapshot, error) {
	if !ev.config.Enabled {
		return nil, nil
	}

	snapshot, err := ev.fileValidator.CreateSnapshot(filePath)
	if err != nil {
		return nil, err
	}

	ev.snapshots[filePath] = snapshot
	ev.logger.Debug("captured snapshot for editing",
		slog.String("path", filePath),
		slog.Int("lines", snapshot.TotalLines),
	)

	return snapshot, nil
}

// ValidateLine validates a single line before editing
func (ev *EditValidator) ValidateLine(filePath string, lineNum int, expectedHash string) error {
	if !ev.config.Enabled {
		return nil
	}

	snapshot, ok := ev.snapshots[filePath]
	if !ok {
		return fmt.Errorf("no snapshot captured for %s", filePath)
	}

	valid, msg := ev.fileValidator.ValidateEdit(snapshot, lineNum, expectedHash)
	if !valid {
		if ev.config.StrictMode {
			return fmt.Errorf("edit validation failed: %s", msg)
		}
		ev.logger.Warn("edit validation failed but continuing",
			slog.String("path", filePath),
			slog.String("message", msg),
		)
	}

	return nil
}

// ValidateEditOperation validates a complete edit operation
func (ev *EditValidator) ValidateEditOperation(req *EditValidationRequest) *EditValidationResult {
	if !ev.config.Enabled {
		return &EditValidationResult{IsValid: true}
	}

	snapshot, ok := ev.snapshots[req.FilePath]
	if !ok {
		return &EditValidationResult{
			IsValid:      false,
			ErrorMessage: fmt.Sprintf("no snapshot for %s", req.FilePath),
		}
	}

	result := ev.fileValidator.ValidateEditRequest(req, snapshot)

	if !result.IsValid && ev.config.StrictMode {
		ev.logger.Error("edit validation failed",
			slog.String("path", req.FilePath),
			slog.String("error", result.ErrorMessage),
		)
		return result
	}

	if !result.IsValid && !ev.config.StrictMode {
		ev.logger.Warn("edit validation failed but continuing",
			slog.String("path", req.FilePath),
			slog.String("error", result.ErrorMessage),
		)
		result.IsValid = true // Allow in lenient mode
	}

	return result
}

// DetectFileChanges detects if file has been modified since snapshot
func (ev *EditValidator) DetectFileChanges(filePath string) ([]string, error) {
	snapshot, ok := ev.snapshots[filePath]
	if !ok {
		return nil, fmt.Errorf("no snapshot for %s", filePath)
	}

	return ev.fileValidator.DetectChanges(snapshot)
}

// ClearSnapshot removes a snapshot for a file
func (ev *EditValidator) ClearSnapshot(filePath string) {
	delete(ev.snapshots, filePath)
	ev.logger.Debug("cleared snapshot", slog.String("path", filePath))
}

// ClearAllSnapshots removes all snapshots
func (ev *EditValidator) ClearAllSnapshots() {
	ev.snapshots = make(map[string]*FileSnapshot)
	ev.logger.Debug("cleared all snapshots")
}

// GetSnapshotCount returns number of active snapshots
func (ev *EditValidator) GetSnapshotCount() int {
	return len(ev.snapshots)
}

// SafeEdit represents a validated edit that can be safely applied
type SafeEdit struct {
	FilePath       string
	LineNum        int
	OldContent     string
	NewContent     string
	ValidationHash string
	Timestamp      int64
}

// PrepareSafeEdit prepares an edit with validation
func (ev *EditValidator) PrepareSafeEdit(filePath string, lineNum int, newContent string) (*SafeEdit, error) {
	snapshot, ok := ev.snapshots[filePath]
	if !ok {
		return nil, fmt.Errorf("no snapshot for %s", filePath)
	}

	if lineNum < 1 || lineNum > snapshot.TotalLines {
		return nil, fmt.Errorf("invalid line number %d", lineNum)
	}

	lineData := snapshot.LineHashes[lineNum-1]

	// Create context hashes
	contextHashes := ev.fileValidator.GetLineContext(snapshot, lineNum)

	// Validate
	req := &EditValidationRequest{
		FilePath:       filePath,
		LineNum:        lineNum,
		OldContent:     lineData.Content,
		OldHash:        lineData.Hash,
		NewContent:     newContent,
		ContextHashes:  contextHashes,
		AllowedToAutoFix: ev.config.AutoFixAllowed,
	}

	result := ev.ValidateEditOperation(req)
	if !result.IsValid {
		return nil, fmt.Errorf("validation failed: %s", result.ErrorMessage)
	}

	safeEdit := &SafeEdit{
		FilePath:       filePath,
		LineNum:        lineNum,
		OldContent:     lineData.Content,
		NewContent:     newContent,
		ValidationHash: lineData.Hash,
	}

	return safeEdit, nil
}

// ValidationStats tracks validation metrics
type ValidationStats struct {
	TotalValidations    int
	SuccessfulEdits     int
	FailedValidations   int
	AutoFixesApplied    int
	FilesCovered        int
	AverageValidateTime float64
}

// Stats returns validation statistics
func (ev *EditValidator) Stats() *ValidationStats {
	return &ValidationStats{
		FilesCovered: len(ev.snapshots),
	}
}
