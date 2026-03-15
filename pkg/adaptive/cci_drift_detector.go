package adaptive

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// DriftDetector implements the CCI Truth Sentry.
//
// At session start (right after the git workspace checkpoint is created),
// DriftDetector parses recent commits and cross-references modified source files
// against the subsystem-to-spec mapping. If a source file was modified but its
// Tier 3 spec was not updated in the same window, a DriftWarning is emitted.
// These warnings are injected into the system prompt as automated pre-flight alerts.
type DriftDetector struct {
	// MaxCommits is how many recent commits to scan. Default: 10.
	MaxCommits int
	// fileMap maps subsystem name → source file patterns (provided by ColdMemoryStore).
	fileMap map[string][]string
	// docsDir is the path to the Tier 3 spec directory.
	docsDir string
}

// NewDriftDetector creates a detector using the subsystem file map and docs directory.
func NewDriftDetector(fileMap map[string][]string, docsDir string) *DriftDetector {
	return &DriftDetector{
		MaxCommits: 10,
		fileMap:    fileMap,
		docsDir:    docsDir,
	}
}

// Check runs the drift analysis for the given working directory.
// Returns a slice of DriftWarnings (empty if everything is up-to-date).
func (d *DriftDetector) Check(cwd string) []DriftWarning {
	if cwd == "" {
		return nil
	}

	// Get recently modified files from git log.
	recentFiles := d.recentlyModifiedFiles(cwd)
	if len(recentFiles) == 0 {
		return nil
	}

	// Get recently modified spec files.
	recentSpecs := d.recentlyModifiedSpecs(cwd)

	var warnings []DriftWarning

	// For each subsystem, check if any source file was modified but the spec was not.
	for subsystem, sourcePaths := range d.fileMap {
		specFile := subsystem + ".md"

		// Check if any source file for this subsystem was recently changed.
		sourceChanged := false
		changedSource := ""
		for _, src := range sourcePaths {
			for _, recent := range recentFiles {
				if matchesPath(recent, src) {
					sourceChanged = true
					changedSource = recent
					break
				}
			}
			if sourceChanged {
				break
			}
		}
		if !sourceChanged {
			continue
		}

		// Source was changed — check if the spec was also updated.
		specUpdated := false
		for _, spec := range recentSpecs {
			if strings.Contains(spec, specFile) {
				specUpdated = true
				break
			}
		}

		if !specUpdated {
			warnings = append(warnings, DriftWarning{
				SourceFile:  changedSource,
				SpecFile:    specFile,
				Subsystem:   subsystem,
				LastModSpec: d.specModTime(specFile),
			})
		}
	}

	return warnings
}

// FormatWarnings converts drift warnings into an injected system prompt block.
func FormatDriftWarnings(warnings []DriftWarning) string {
	if len(warnings) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("<!-- CCI TRUTH SENTRY: DRIFT WARNINGS (pre-flight check) -->\n")
	sb.WriteString(fmt.Sprintf("## ⚠ Context Drift Detected (%d subsystem(s))\n\n", len(warnings)))
	sb.WriteString("The following source files were recently modified but their Tier 3 specs were NOT updated.\n")
	sb.WriteString("**Trust these specs with caution — verify against source before acting.**\n\n")

	for _, w := range warnings {
		sb.WriteString(fmt.Sprintf("- **%s**: `%s` was changed but `cci/docs/%s` was not updated",
			w.Subsystem, w.SourceFile, w.SpecFile))
		if !w.LastModSpec.IsZero() {
			sb.WriteString(fmt.Sprintf(" (spec last updated: %s)", w.LastModSpec.Format("2006-01-02")))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\n**Before modifying these subsystems, run:** `mcp_context_get_subsystem {\"name\": \"<subsystem>\"}` and verify accuracy.\n")
	sb.WriteString("<!-- END TRUTH SENTRY -->\n")

	return sb.String()
}

// recentlyModifiedFiles returns file paths changed in the last MaxCommits commits.
// Filters out non-source-code files (go.sum, vendor/, etc.).
func (d *DriftDetector) recentlyModifiedFiles(cwd string) []string {
	n := d.MaxCommits
	if n <= 0 {
		n = 10
	}
	cmd := exec.Command("git", "log", fmt.Sprintf("--max-count=%d", n),
		"--name-only", "--pretty=format:")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	seen := make(map[string]bool)
	var files []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || seen[line] {
			continue
		}
		// Filter noise: go.sum, vendor/, .git/, docs that are themselves the specs
		if isNoisePath(line) {
			continue
		}
		seen[line] = true
		files = append(files, line)
	}
	return files
}

// recentlyModifiedSpecs returns spec file paths (relative) changed in recent commits.
func (d *DriftDetector) recentlyModifiedSpecs(cwd string) []string {
	n := d.MaxCommits
	if n <= 0 {
		n = 10
	}
	cmd := exec.Command("git", "log", fmt.Sprintf("--max-count=%d", n),
		"--name-only", "--pretty=format:", "--", "*.md")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var specs []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			specs = append(specs, line)
		}
	}
	return specs
}

func (d *DriftDetector) specModTime(specFile string) time.Time {
	// Try to get the last git commit date for the spec file.
	// If not tracked, stat the file.
	cmd := exec.Command("git", "log", "-1", "--format=%ci", "--", d.docsDir+"/"+specFile)
	out, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return time.Time{}
	}
	t, _ := time.Parse("2006-01-02 15:04:05 -0700", strings.TrimSpace(string(out)))
	return t
}

func matchesPath(actual, pattern string) bool {
	// Normalize separators and check suffix or substring.
	actual = strings.ReplaceAll(actual, "\\", "/")
	pattern = strings.ReplaceAll(pattern, "\\", "/")
	return strings.HasSuffix(actual, pattern) || strings.Contains(actual, pattern)
}

func isNoisePath(p string) bool {
	noisy := []string{"go.sum", "go.mod", "vendor/", ".git/", "bin/", "testdata/"}
	for _, n := range noisy {
		if strings.Contains(p, n) || strings.HasPrefix(p, n) {
			return true
		}
	}
	return false
}
