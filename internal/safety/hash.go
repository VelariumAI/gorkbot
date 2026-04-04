package safety

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log/slog"
	"strings"
)

// LineHash represents the hash of a specific line
type LineHash struct {
	LineNum int
	Content string
	Hash    string
}

// FileSnapshot captures file content and line hashes
type FileSnapshot struct {
	Path           string
	TotalLines     int
	FullHash       string      // SHA256 of entire file
	LineHashes     []LineHash  // Per-line hashes
	Timestamp      int64       // When snapshot was taken
	Metadata       map[string]interface{}
}

// FileValidator validates file integrity
type FileValidator struct {
	logger *slog.Logger
	depth  int // Number of lines to hash around an edit point
}

// NewFileValidator creates a new file validator
func NewFileValidator(logger *slog.Logger, depth int) *FileValidator {
	if logger == nil {
		logger = slog.Default()
	}
	if depth == 0 {
		depth = 5 // Default: hash 5 lines before/after
	}
	return &FileValidator{
		logger: logger,
		depth:  depth,
	}
}

// CreateSnapshot creates a snapshot of a file
func (fv *FileValidator) CreateSnapshot(filePath string) (*FileSnapshot, error) {
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	lines := strings.Split(string(content), "\n")
	snapshot := &FileSnapshot{
		Path:       filePath,
		TotalLines: len(lines),
		Metadata:   make(map[string]interface{}),
	}

	// Compute full file hash
	fullHash := sha256.Sum256(content)
	snapshot.FullHash = hex.EncodeToString(fullHash[:])

	// Compute per-line hashes
	snapshot.LineHashes = make([]LineHash, len(lines))
	for i, line := range lines {
		lineHash := sha256.Sum256([]byte(line))
		snapshot.LineHashes[i] = LineHash{
			LineNum: i + 1,
			Content: line,
			Hash:    hex.EncodeToString(lineHash[:]),
		}
	}

	fv.logger.Debug("created file snapshot",
		slog.String("path", filePath),
		slog.Int("lines", snapshot.TotalLines),
		slog.String("hash", snapshot.FullHash[:16]),
	)

	return snapshot, nil
}

// ValidateEdit checks if a line is still valid for editing
func (fv *FileValidator) ValidateEdit(snapshot *FileSnapshot, lineNum int, expectedHash string) (bool, string) {
	if lineNum < 1 || lineNum > snapshot.TotalLines {
		return false, fmt.Sprintf("line number %d out of range (1-%d)", lineNum, snapshot.TotalLines)
	}

	lineHash := snapshot.LineHashes[lineNum-1]
	if lineHash.Hash != expectedHash {
		expShort := expectedHash
		if len(expShort) > 16 {
			expShort = expShort[:16]
		}
		gotShort := lineHash.Hash
		if len(gotShort) > 16 {
			gotShort = gotShort[:16]
		}
		return false, fmt.Sprintf("line %d hash mismatch: expected %s, got %s",
			lineNum, expShort, gotShort)
	}

	return true, ""
}

// ValidateContext checks if lines around an edit point are unchanged
func (fv *FileValidator) ValidateContext(snapshot *FileSnapshot, lineNum int, contextHashes map[int]string) (bool, []string) {
	errors := []string{}

	for lineOffset, expectedHash := range contextHashes {
		checkLineNum := lineNum + lineOffset
		if checkLineNum < 1 || checkLineNum > snapshot.TotalLines {
			errors = append(errors, fmt.Sprintf("context line %d out of range", checkLineNum))
			continue
		}

		lineHash := snapshot.LineHashes[checkLineNum-1]
		if lineHash.Hash != expectedHash {
			content := snapshot.LineHashes[checkLineNum-1].Content
			if len(content) > 40 {
				content = content[:40]
			}
			errors = append(errors,
				fmt.Sprintf("context line %d changed: %s → %s",
					checkLineNum,
					content,
					"[file changed]",
				),
			)
		}
	}

	return len(errors) == 0, errors
}

// DetectChanges compares current file state with snapshot
func (fv *FileValidator) DetectChanges(snapshot *FileSnapshot) ([]string, error) {
	currentSnapshot, err := fv.CreateSnapshot(snapshot.Path)
	if err != nil {
		return nil, err
	}

	changes := []string{}

	// Check if file hashes match
	if snapshot.FullHash != currentSnapshot.FullHash {
		// File changed - find which lines
		for i := 0; i < len(snapshot.LineHashes) && i < len(currentSnapshot.LineHashes); i++ {
			if snapshot.LineHashes[i].Hash != currentSnapshot.LineHashes[i].Hash {
				changes = append(changes, fmt.Sprintf("line %d changed", i+1))
			}
		}

		if len(snapshot.LineHashes) != len(currentSnapshot.LineHashes) {
			changes = append(changes, fmt.Sprintf("line count changed: %d → %d",
				len(snapshot.LineHashes), len(currentSnapshot.LineHashes)))
		}
	}

	return changes, nil
}

// GetLineContext returns hashes around a specific line
func (fv *FileValidator) GetLineContext(snapshot *FileSnapshot, lineNum int) map[int]string {
	context := make(map[int]string)

	start := lineNum - fv.depth
	if start < 1 {
		start = 1
	}

	end := lineNum + fv.depth
	if end > snapshot.TotalLines {
		end = snapshot.TotalLines
	}

	for i := start; i <= end; i++ {
		offset := i - lineNum
		context[offset] = snapshot.LineHashes[i-1].Hash
	}

	return context
}

// ExtractEditHints extracts changeable lines from snapshot
func (fv *FileValidator) ExtractEditHints(snapshot *FileSnapshot) []string {
	hints := []string{}
	for i, lh := range snapshot.LineHashes {
		if i%100 == 0 || len(lh.Content) < 80 {
			hints = append(hints, fmt.Sprintf("Line %d (hash: %s): %s",
				lh.LineNum, lh.Hash[:12], truncate(lh.Content, 60)))
		}
	}
	return hints
}

// truncate truncates string to max length
func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}

// EditValidationRequest represents a proposed edit
type EditValidationRequest struct {
	FilePath         string
	LineNum          int
	OldContent       string
	OldHash          string
	NewContent       string
	ContextHashes    map[int]string
	AllowedToAutoFix bool
}

// EditValidationResult represents the result of edit validation
type EditValidationResult struct {
	IsValid              bool
	LineStillValid       bool
	ContextStillValid    bool
	Changes              []string
	SuggestedLineHash    string
	AutoFixAvailable     bool
	ErrorMessage         string
}

// ValidateEditRequest validates an edit request against current file state
func (fv *FileValidator) ValidateEditRequest(req *EditValidationRequest, snapshot *FileSnapshot) *EditValidationResult {
	result := &EditValidationResult{
		IsValid: true,
	}

	// Check if line still exists and has expected hash
	if req.LineNum < 1 || req.LineNum > snapshot.TotalLines {
		result.IsValid = false
		result.LineStillValid = false
		result.ErrorMessage = fmt.Sprintf("line %d no longer exists (file has %d lines)",
			req.LineNum, snapshot.TotalLines)
		return result
	}

	lineHash := snapshot.LineHashes[req.LineNum-1]
	result.SuggestedLineHash = lineHash.Hash

	if lineHash.Hash != req.OldHash {
		result.IsValid = false
		result.LineStillValid = false
		result.ErrorMessage = fmt.Sprintf("line %d has changed (old hash doesn't match)",
			req.LineNum)
		return result
	}

	// Check context
	if len(req.ContextHashes) > 0 {
		contextValid, errors := fv.ValidateContext(snapshot, req.LineNum, req.ContextHashes)
		result.ContextStillValid = contextValid
		result.Changes = errors

		if !contextValid && !req.AllowedToAutoFix {
			result.IsValid = false
			result.ErrorMessage = fmt.Sprintf("context changed at lines around %d", req.LineNum)
			return result
		}

		if !contextValid && req.AllowedToAutoFix {
			result.AutoFixAvailable = true
		}
	}

	return result
}

// DiffStats represents file diff statistics
type DiffStats struct {
	LinesAdded   int
	LinesRemoved int
	LinesChanged int
	TotalLines   int
}

// ComputeDiff computes differences between two snapshots
func (fv *FileValidator) ComputeDiff(old, current *FileSnapshot) *DiffStats {
	stats := &DiffStats{
		TotalLines: current.TotalLines,
	}

	minLines := len(old.LineHashes)
	if len(current.LineHashes) < minLines {
		minLines = len(current.LineHashes)
	}

	// Count changed lines
	for i := 0; i < minLines; i++ {
		if old.LineHashes[i].Hash != current.LineHashes[i].Hash {
			stats.LinesChanged++
		}
	}

	// Count added/removed lines
	if len(current.LineHashes) > len(old.LineHashes) {
		stats.LinesAdded = len(current.LineHashes) - len(old.LineHashes)
	} else if len(old.LineHashes) > len(current.LineHashes) {
		stats.LinesRemoved = len(old.LineHashes) - len(current.LineHashes)
	}

	return stats
}
