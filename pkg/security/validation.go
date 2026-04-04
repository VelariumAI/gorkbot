package security

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateInput hardens string inputs by rejecting control characters.
// Does NOT reject punctuation like ?, #, % since user input (queries, messages)
// naturally contains these characters. Resource-specific validation is done elsewhere.
func ValidateInput(input string) error {
	for _, r := range input {
		// Reject control characters (but allow \t, \n, \r)
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
			return fmt.Errorf("invalid character: control character detected (0x%02X)", r)
		}
	}
	return nil
}

// ValidatePath canonicalizes the given path and sandboxes it to the current working directory.
// Uses platform-agnostic filepath functions.
func ValidatePath(path string) (string, error) {
	if err := ValidateInput(path); err != nil {
		return "", fmt.Errorf("path validation failed: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("could not get current working directory: %w", err)
	}

	// Canonicalize the path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("could not get absolute path: %w", err)
	}

	// Resolve symlinks
	cleanPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		// If the file doesn't exist yet, EvalSymlinks fails. We can use filepath.Clean instead.
		cleanPath = filepath.Clean(absPath)
	}

	// Sandbox to CWD (ensure cleanPath starts with cwd)
	if !strings.HasPrefix(cleanPath, cwd) {
		return "", errors.New("path escapes the current working directory sandbox")
	}

	// Ensure the boundary is precise (e.g. /home/user/dir vs /home/user/dir2)
	// Add a path separator to cwd if it doesn't have one, to avoid prefix attacks.
	cwdWithSep := cwd
	if !strings.HasSuffix(cwdWithSep, string(os.PathSeparator)) {
		cwdWithSep += string(os.PathSeparator)
	}
	cleanPathWithSep := cleanPath
	if !strings.HasSuffix(cleanPathWithSep, string(os.PathSeparator)) {
		cleanPathWithSep += string(os.PathSeparator)
	}

	if !strings.HasPrefix(cleanPathWithSep, cwdWithSep) {
		return "", errors.New("path escapes the current working directory sandbox")
	}

	return cleanPath, nil
}
