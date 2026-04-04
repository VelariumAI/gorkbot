package harness

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Verifier runs verification checks against features.
type Verifier struct {
	store  *Store
	logger *slog.Logger
}

// NewVerifier creates a new Verifier.
func NewVerifier(store *Store, logger *slog.Logger) *Verifier {
	if logger == nil {
		logger = slog.Default()
	}
	return &Verifier{store: store, logger: logger}
}

// Verify runs all verification steps for a feature and returns a report.
func (v *Verifier) Verify(ctx context.Context, feature *Feature) (*VerificationReport, error) {
	report := &VerificationReport{
		FeatureID: feature.ID,
		Passed:    true,
	}

	workdir := v.store.projectRoot

	// Run default project-type checks
	defaults := v.DefaultChecks(ctx, workdir)
	report.Steps = append(report.Steps, defaults...)

	// Run feature-specific verification commands
	if feature.VerificationSpec != nil {
		for _, vc := range feature.VerificationSpec.Commands {
			step := v.RunCommand(ctx, vc, workdir)
			report.Steps = append(report.Steps, *step)
		}
	}

	// Aggregate pass/fail
	var failed []string
	for i := range report.Steps {
		if !report.Steps[i].Passed {
			report.Passed = false
			failed = append(failed, report.Steps[i].Name)
		}
	}

	if report.Passed {
		report.Summary = fmt.Sprintf("All %d verification steps passed", len(report.Steps))
	} else {
		report.Summary = fmt.Sprintf("%d/%d steps failed: %s",
			len(failed), len(report.Steps), strings.Join(failed, ", "))
	}

	// Persist report
	if err := v.store.SaveVerificationReport(report); err != nil {
		v.logger.Warn("failed to save verification report", "error", err)
	}

	return report, nil
}

// RunCommand executes a single verification command and returns the step result.
func (v *Verifier) RunCommand(ctx context.Context, vc VerifyCommand, workdir string) *VerificationStep {
	timeout := vc.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", vc.Command)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", vc.Command)
	}
	cmd.Dir = workdir

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	duration := time.Since(start)

	step := &VerificationStep{
		Name:     vc.Name,
		Command:  vc.Command,
		Output:   truncateOutput(out.String(), 4000),
		Duration: duration,
	}

	if err != nil {
		step.Passed = false
		if exitErr, ok := err.(*exec.ExitError); ok {
			step.ExitCode = exitErr.ExitCode()
		} else {
			step.ExitCode = -1
			step.Output += "\n" + err.Error()
		}
	} else {
		step.Passed = true
		step.ExitCode = 0
	}

	return step
}

// DefaultChecks detects the project type and returns appropriate build/test steps.
func (v *Verifier) DefaultChecks(ctx context.Context, workdir string) []VerificationStep {
	var steps []VerificationStep

	// Go project
	if fileExists(filepath.Join(workdir, "go.mod")) {
		steps = append(steps,
			*v.RunCommand(ctx, VerifyCommand{Name: "go build", Command: "go build ./..."}, workdir),
			*v.RunCommand(ctx, VerifyCommand{Name: "go vet", Command: "go vet ./..."}, workdir),
		)
	}

	// Node project
	if fileExists(filepath.Join(workdir, "package.json")) {
		steps = append(steps,
			*v.RunCommand(ctx, VerifyCommand{Name: "npm test", Command: "npm test"}, workdir),
		)
	}

	// Python project
	if fileExists(filepath.Join(workdir, "pyproject.toml")) || fileExists(filepath.Join(workdir, "setup.py")) {
		steps = append(steps,
			*v.RunCommand(ctx, VerifyCommand{Name: "pytest", Command: "python -m pytest"}, workdir),
		)
	}

	return steps
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func truncateOutput(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n... (truncated)"
}
