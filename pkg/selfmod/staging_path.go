package selfmod

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	toolStagingPrefix    = ".gorkbot/staging/tools"
	commandStagingPrefix = ".gorkbot/staging/commands"
	pythonStagingPrefix  = ".gorkbot/staging/python_tools"
)

// SafeStagingPath is a validated staging path artifact for filesystem sinks.
type SafeStagingPath struct {
	path string
}

func NewToolStagingPath(name string) (SafeStagingPath, error) {
	safeName, err := normalizeSafeName(name)
	if err != nil {
		return SafeStagingPath{}, err
	}
	return newSafeStagingPath(toolStagingPrefix, safeName+".go")
}

func NewCommandStagingPath(name string) (SafeStagingPath, error) {
	safeName, err := normalizeSafeName(name)
	if err != nil {
		return SafeStagingPath{}, err
	}
	return newSafeStagingPath(commandStagingPrefix, safeName+".json")
}

func NewPythonToolStagingPath(name string, filename string) (SafeStagingPath, error) {
	safeName, err := normalizeSafeName(name)
	if err != nil {
		return SafeStagingPath{}, err
	}
	safeFile, err := normalizeSafeName(filename)
	if err != nil {
		return SafeStagingPath{}, err
	}
	return newSafeStagingPath(pythonStagingPrefix, safeName, safeFile)
}

func (p SafeStagingPath) Dir() string {
	return filepath.Dir(p.path)
}

func (p SafeStagingPath) String() string {
	return p.path
}

func WriteStagedFile(p SafeStagingPath, content []byte, perm fs.FileMode) error {
	dir := filepath.Dir(p.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(p.path, content, perm)
}

func newSafeStagingPath(parts ...string) (SafeStagingPath, error) {
	joined := filepath.Join(parts...)
	clean := filepath.ToSlash(filepath.Clean(joined))
	if clean == "." || clean == "" {
		return SafeStagingPath{}, fmt.Errorf("%s: empty staging path", REASON_DYNAMIC_PATH_OUTSIDE_STAGING)
	}
	requiresApproval, hardBlock, reason, issue := ValidateStagedTargetPath(clean)
	if hardBlock {
		return SafeStagingPath{}, fmt.Errorf("%s: %s", reason, issue)
	}
	if requiresApproval {
		return SafeStagingPath{}, fmt.Errorf("%s: %s", reason, issue)
	}
	return SafeStagingPath{path: clean}, nil
}

func normalizeSafeName(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if err := ValidateSafeArtifactName(v); err != nil {
		return "", err
	}
	return v, nil
}
