package provider

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// LocalSandboxProvider implements SandboxProvider by executing commands on the host OS.
// It enforces a working directory boundary to prevent path escapes.
type LocalSandboxProvider struct {
	workDir string
}

// newLocalSandboxProvider creates a new local sandbox provider.
// It reads params["work_dir"] (defaults to current working directory).
func newLocalSandboxProvider(params map[string]interface{}) (SandboxProvider, error) {
	workDir := ""
	if wd, ok := params["work_dir"]; ok {
		if s, ok := wd.(string); ok {
			workDir = s
		}
	}

	// Default to current working directory if not specified
	if workDir == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get working directory: %w", err)
		}
	}

	// Ensure work_dir is absolute
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return nil, fmt.Errorf("invalid work_dir: %w", err)
	}

	return &LocalSandboxProvider{
		workDir: absWorkDir,
	}, nil
}

// RunCommand executes a command in the sandbox environment.
func (l *LocalSandboxProvider) RunCommand(ctx context.Context, cmd string, args []string) (output string, exitCode int, err error) {
	command := exec.CommandContext(ctx, cmd, args...)
	command.Dir = l.workDir

	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	err = command.Run()
	output = stdout.String()
	if stderr.Len() > 0 {
		output = output + "\n" + stderr.String()
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			// Command failed to start or other error
			exitCode = -1
		}
		return output, exitCode, err
	}

	return output, 0, nil
}

// ReadFile reads a file from the sandbox, ensuring it's within the work directory.
func (l *LocalSandboxProvider) ReadFile(ctx context.Context, path string) ([]byte, error) {
	// Resolve the path relative to work directory
	if !filepath.IsAbs(path) {
		path = filepath.Join(l.workDir, path)
	}

	// Ensure the file is within workDir (prevent path escape)
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}

	if !isWithinDir(absPath, l.workDir) {
		return nil, fmt.Errorf("path escape attempt: %s is outside work directory", path)
	}

	return os.ReadFile(absPath)
}

// WriteFile writes content to a file in the sandbox, ensuring it's within the work directory.
func (l *LocalSandboxProvider) WriteFile(ctx context.Context, path string, content []byte) error {
	// Resolve the path relative to work directory
	if !filepath.IsAbs(path) {
		path = filepath.Join(l.workDir, path)
	}

	// Ensure the file is within workDir (prevent path escape)
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	if !isWithinDir(absPath, l.workDir) {
		return fmt.Errorf("path escape attempt: %s is outside work directory", path)
	}

	// Create parent directory if needed
	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	return os.WriteFile(absPath, content, 0644)
}

// Name returns the name of this provider.
func (l *LocalSandboxProvider) Name() string {
	return "LocalSandboxProvider"
}

// Close releases any resources (no-op for local provider).
func (l *LocalSandboxProvider) Close() error {
	return nil
}

// isWithinDir checks if path is within the given directory, preventing path escape.
func isWithinDir(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	// Check if the relative path starts with ".." (would escape the directory)
	return !filepath.IsAbs(rel) && (rel == "." || !bytes.HasPrefix([]byte(rel), []byte("..")))
}
