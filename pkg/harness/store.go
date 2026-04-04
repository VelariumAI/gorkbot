package harness

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Store provides thread-safe JSON persistence for harness state.
type Store struct {
	projectRoot string
	mu          sync.RWMutex
}

// NewStore creates a new Store for the given project root.
func NewStore(projectRoot string) *Store {
	return &Store{projectRoot: projectRoot}
}

// HarnessDir returns the path to the harness state directory.
func (s *Store) HarnessDir() string {
	return filepath.Join(s.projectRoot, ".gorkbot", "harness")
}

// EnsureDir creates the harness directory tree if it doesn't exist.
func (s *Store) EnsureDir() error {
	if err := os.MkdirAll(s.HarnessDir(), 0700); err != nil {
		return fmt.Errorf("create harness dir: %w", err)
	}
	return os.MkdirAll(filepath.Join(s.HarnessDir(), "reports"), 0700)
}

// ── Feature List ─────────────────────────────────────────────────────────────

func (s *Store) featureListPath() string {
	return filepath.Join(s.HarnessDir(), "feature_list.json")
}

// LoadFeatureList reads the feature list from disk.
func (s *Store) LoadFeatureList() (*FeatureList, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.featureListPath())
	if err != nil {
		return nil, fmt.Errorf("read feature list: %w", err)
	}
	var fl FeatureList
	if err := json.Unmarshal(data, &fl); err != nil {
		return nil, fmt.Errorf("parse feature list: %w", err)
	}
	return &fl, nil
}

// SaveFeatureList atomically writes the feature list to disk.
func (s *Store) SaveFeatureList(fl *FeatureList) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.atomicWrite(s.featureListPath(), fl)
}

// ── Harness State ────────────────────────────────────────────────────────────

func (s *Store) statePath() string {
	return filepath.Join(s.HarnessDir(), "harness_state.json")
}

// LoadState reads harness state from disk.
func (s *Store) LoadState() (*HarnessState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.statePath())
	if err != nil {
		return nil, fmt.Errorf("read harness state: %w", err)
	}
	var hs HarnessState
	if err := json.Unmarshal(data, &hs); err != nil {
		return nil, fmt.Errorf("parse harness state: %w", err)
	}
	return &hs, nil
}

// SaveState atomically writes harness state to disk.
func (s *Store) SaveState(hs *HarnessState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.atomicWrite(s.statePath(), hs)
}

// ── Progress Log ─────────────────────────────────────────────────────────────

func (s *Store) progressPath() string {
	return filepath.Join(s.HarnessDir(), "claude-progress.txt")
}

// AppendProgress appends a progress entry as a JSONL line.
func (s *Store) AppendProgress(entry ProgressEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal progress: %w", err)
	}

	f, err := os.OpenFile(s.progressPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open progress file: %w", err)
	}
	defer f.Close()

	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}

// LoadProgress reads all progress entries from the JSONL file.
func (s *Store) LoadProgress() ([]ProgressEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	f, err := os.Open(s.progressPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open progress file: %w", err)
	}
	defer f.Close()

	var entries []ProgressEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry ProgressEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue // skip malformed lines
		}
		entries = append(entries, entry)
	}
	return entries, scanner.Err()
}

// ── Verification Reports ─────────────────────────────────────────────────────

func (s *Store) reportPath(featureID string) string {
	return filepath.Join(s.HarnessDir(), "reports", featureID+".json")
}

// LoadVerificationReport reads a verification report for a feature.
func (s *Store) LoadVerificationReport(featureID string) (*VerificationReport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.reportPath(featureID))
	if err != nil {
		return nil, fmt.Errorf("read report: %w", err)
	}
	var vr VerificationReport
	if err := json.Unmarshal(data, &vr); err != nil {
		return nil, fmt.Errorf("parse report: %w", err)
	}
	return &vr, nil
}

// SaveVerificationReport atomically writes a verification report.
func (s *Store) SaveVerificationReport(vr *VerificationReport) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.atomicWrite(s.reportPath(vr.FeatureID), vr)
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// atomicWrite writes data to a temp file then renames it into place.
// Caller must hold s.mu write lock.
func (s *Store) atomicWrite(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write temp: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}
