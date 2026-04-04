package harness

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// Worker implements the boot/resume/select/complete protocol.
type Worker struct {
	store     *Store
	verifier  *Verifier
	logger    *slog.Logger
	sessionID string
}

// NewWorker creates a new Worker.
func NewWorker(store *Store, verifier *Verifier, sessionID string, logger *slog.Logger) *Worker {
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{
		store:     store,
		verifier:  verifier,
		logger:    logger,
		sessionID: sessionID,
	}
}

// Boot reads the current project state and returns a summary report.
func (w *Worker) Boot(ctx context.Context) (*BootReport, error) {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = w.store.projectRoot
	}

	report := &BootReport{ProjectRoot: cwd}

	// Get recent git commits
	cmd := exec.CommandContext(ctx, "git", "log", "--oneline", "-20")
	cmd.Dir = w.store.projectRoot
	var gitOut bytes.Buffer
	cmd.Stdout = &gitOut
	cmd.Stderr = &gitOut
	if err := cmd.Run(); err == nil {
		report.RecentCommits = strings.TrimSpace(gitOut.String())
	}

	// Load progress
	progress, _ := w.store.LoadProgress()
	if len(progress) > 10 {
		progress = progress[len(progress)-10:]
	}
	report.RecentProgress = progress

	// Load feature list
	fl, err := w.store.LoadFeatureList()
	if err != nil {
		return nil, fmt.Errorf("load feature list: %w", err)
	}

	// Count statuses
	report.TotalFeatures = len(fl.Features)
	for i := range fl.Features {
		switch fl.Features[i].Status {
		case StatusPassing:
			report.PassingCount++
		case StatusFailing, StatusIncomplete:
			report.FailingCount++
		case StatusInProgress:
			report.InProgressCount++
		}
	}

	// Find active feature
	state, _ := w.store.LoadState()
	if state != nil && state.ActiveFeatureID != "" {
		for i := range fl.Features {
			if fl.Features[i].ID == state.ActiveFeatureID {
				f := fl.Features[i]
				report.ActiveFeature = &f
				break
			}
		}
	}

	// Suggest next feature
	next, _ := w.selectFrom(fl)
	if next != nil {
		report.NextSuggested = next
	}

	// Update state
	now := time.Now()
	if state == nil {
		state = &HarnessState{
			ProjectRoot: w.store.projectRoot,
			Initialized: true,
		}
	}
	state.SessionID = w.sessionID
	state.LastBootAt = now
	state.TotalSessions++
	if err := w.store.SaveState(state); err != nil {
		w.logger.Warn("failed to save state on boot", "error", err)
	}

	// Record boot
	_ = w.store.AppendProgress(ProgressEntry{
		Timestamp: now,
		SessionID: w.sessionID,
		Action:    "boot",
		Details:   fmt.Sprintf("Session booted. %d/%d features passing.", report.PassingCount, report.TotalFeatures),
	})

	return report, nil
}

// SelectFeature finds the next eligible feature to work on.
func (w *Worker) SelectFeature() (*Feature, error) {
	fl, err := w.store.LoadFeatureList()
	if err != nil {
		return nil, fmt.Errorf("load feature list: %w", err)
	}
	return w.selectFrom(fl)
}

// selectFrom picks the highest-priority non-blocked feature.
func (w *Worker) selectFrom(fl *FeatureList) (*Feature, error) {
	// Build set of passing feature IDs for dependency checks
	passing := make(map[string]bool)
	for i := range fl.Features {
		if fl.Features[i].Status == StatusPassing {
			passing[fl.Features[i].ID] = true
		}
	}

	// Filter eligible features
	var eligible []Feature
	for _, f := range fl.Features {
		if f.Status == StatusPassing || f.Status == StatusSkipped {
			continue
		}
		// Check all dependencies are passing
		allDepsMet := true
		for _, dep := range f.Dependencies {
			if !passing[dep] {
				allDepsMet = false
				break
			}
		}
		if allDepsMet {
			eligible = append(eligible, f)
		}
	}

	if len(eligible) == 0 {
		return nil, fmt.Errorf("no eligible features found (all passing, skipped, or blocked)")
	}

	// Sort by priority (lowest number = highest priority)
	sort.Slice(eligible, func(i, j int) bool {
		return eligible[i].Priority < eligible[j].Priority
	})

	result := eligible[0]
	return &result, nil
}

// StartFeature marks a feature as in-progress and updates the active feature.
func (w *Worker) StartFeature(featureID string) error {
	fl, err := w.store.LoadFeatureList()
	if err != nil {
		return fmt.Errorf("load feature list: %w", err)
	}

	now := time.Now()
	found := false
	for i := range fl.Features {
		if fl.Features[i].ID == featureID {
			fl.Features[i].Status = StatusInProgress
			fl.Features[i].UpdatedAt = now
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("feature %q not found", featureID)
	}

	fl.UpdatedAt = now
	if err := w.store.SaveFeatureList(fl); err != nil {
		return fmt.Errorf("save feature list: %w", err)
	}

	// Update active feature in state
	state, _ := w.store.LoadState()
	if state == nil {
		state = &HarnessState{ProjectRoot: w.store.projectRoot, Initialized: true}
	}
	state.ActiveFeatureID = featureID
	state.SessionID = w.sessionID
	if err := w.store.SaveState(state); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	_ = w.store.AppendProgress(ProgressEntry{
		Timestamp: now,
		SessionID: w.sessionID,
		Action:    "start_feature",
		FeatureID: featureID,
		Details:   "Feature work started",
	})

	return nil
}

// CompleteFeature runs verification, commits if passing, and updates status.
func (w *Worker) CompleteFeature(ctx context.Context, featureID, commitMsg string) (*VerificationReport, error) {
	fl, err := w.store.LoadFeatureList()
	if err != nil {
		return nil, fmt.Errorf("load feature list: %w", err)
	}

	// Find feature
	var feature *Feature
	var featureIdx int
	for i := range fl.Features {
		if fl.Features[i].ID == featureID {
			feature = &fl.Features[i]
			featureIdx = i
			break
		}
	}
	if feature == nil {
		return nil, fmt.Errorf("feature %q not found", featureID)
	}

	// Run verification
	report, err := w.verifier.Verify(ctx, feature)
	if err != nil {
		return nil, fmt.Errorf("verification failed: %w", err)
	}

	now := time.Now()

	if report.Passed {
		// Git commit
		commitHash, gitErr := w.gitCommit(ctx, commitMsg)
		if gitErr != nil {
			w.logger.Warn("git commit failed", "error", gitErr)
		}

		fl.Features[featureIdx].Status = StatusPassing
		fl.Features[featureIdx].CommitHash = commitHash
		fl.Features[featureIdx].ErrorLog = ""
	} else {
		fl.Features[featureIdx].Status = StatusIncomplete
		fl.Features[featureIdx].ErrorLog = report.Summary
	}

	fl.Features[featureIdx].UpdatedAt = now
	fl.UpdatedAt = now

	if err := w.store.SaveFeatureList(fl); err != nil {
		return nil, fmt.Errorf("save feature list: %w", err)
	}

	// Clear active feature if it passed
	if report.Passed {
		state, _ := w.store.LoadState()
		if state != nil && state.ActiveFeatureID == featureID {
			state.ActiveFeatureID = ""
			_ = w.store.SaveState(state)
		}
	}

	action := "complete_pass"
	if !report.Passed {
		action = "complete_fail"
	}
	_ = w.store.AppendProgress(ProgressEntry{
		Timestamp: now,
		SessionID: w.sessionID,
		Action:    action,
		FeatureID: featureID,
		Details:   report.Summary,
	})

	return report, nil
}

// gitCommit stages all changes and commits with the given message.
func (w *Worker) gitCommit(ctx context.Context, msg string) (string, error) {
	dir := w.store.projectRoot

	// git add -A
	addCmd := exec.CommandContext(ctx, "git", "add", "-A")
	addCmd.Dir = dir
	if out, err := addCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git add: %s: %w", string(out), err)
	}

	// git commit
	commitCmd := exec.CommandContext(ctx, "git", "commit", "-m", msg)
	commitCmd.Dir = dir
	if out, err := commitCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git commit: %s: %w", string(out), err)
	}

	// Get commit hash
	hashCmd := exec.CommandContext(ctx, "git", "rev-parse", "--short", "HEAD")
	hashCmd.Dir = dir
	var hashOut bytes.Buffer
	hashCmd.Stdout = &hashOut
	if err := hashCmd.Run(); err != nil {
		return "", nil // non-fatal
	}

	return strings.TrimSpace(hashOut.String()), nil
}
