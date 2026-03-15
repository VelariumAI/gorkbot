package dag

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Executor abstracts platform-specific command execution and file I/O.
// The DAG engine never calls os/exec directly; it always goes through this
// interface so behaviour can be swapped transparently between Termux, standard
// Linux/macOS, and Windows without conditional logic in task ActionFuncs.
type Executor interface {
	// Exec runs an arbitrary command and returns its combined stdout+stderr.
	Exec(ctx context.Context, name string, args ...string) (string, error)

	// Shell runs a shell one-liner (sh -c on Unix, cmd /C on Windows).
	Shell(ctx context.Context, cmdLine string) (string, error)

	// ReadFile returns the full text content of a file.
	ReadFile(ctx context.Context, path string) (string, error)

	// WriteFile atomically writes content to path (creates parent directories).
	WriteFile(ctx context.Context, path, content string) error

	// AppendFile appends content to path (creates the file if absent).
	AppendFile(ctx context.Context, path, content string) error

	// EditFile replaces the first occurrence of oldText with newText in path.
	// Returns an error if oldText is not found.
	EditFile(ctx context.Context, path, oldText, newText string) error

	// Capabilities returns a list of features available on this platform
	// (e.g. "sed", "grep", "bash", "powershell").
	Capabilities() []string

	// Platform returns a string like "termux", "linux", "darwin", "windows".
	Platform() string
}

// ─── Unix Executor ────────────────────────────────────────────────────────────

// UnixExecutor implements Executor for Linux, macOS, and Android/Termux.
// It prefers native Go I/O primitives for file operations and falls back to
// shell utilities only when they add genuine value (e.g. piped grep).
type UnixExecutor struct {
	IsTermux bool
	// Shell is the shell binary used for Shell(). Defaults to "sh".
	ShellBin string
	// Timeout is the default deadline added to Exec/Shell calls with no
	// deadline already set. Zero = 60s.
	Timeout time.Duration
}

func (e *UnixExecutor) shell() string {
	if e.ShellBin != "" {
		return e.ShellBin
	}
	if e.IsTermux {
		// Termux ships bash and is the preferred shell there.
		if _, err := exec.LookPath("bash"); err == nil {
			return "bash"
		}
	}
	return "sh"
}

func (e *UnixExecutor) deadline(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	timeout := e.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return context.WithTimeout(ctx, timeout)
}

func (e *UnixExecutor) Exec(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := e.deadline(ctx)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("exec %q: %w\n%s", name, err, string(out))
	}
	return string(out), nil
}

func (e *UnixExecutor) Shell(ctx context.Context, cmdLine string) (string, error) {
	return e.Exec(ctx, e.shell(), "-c", cmdLine)
}

func (e *UnixExecutor) ReadFile(_ context.Context, path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %q: %w", path, err)
	}
	return string(b), nil
}

func (e *UnixExecutor) WriteFile(_ context.Context, path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %q: %w", filepath.Dir(path), err)
	}
	// Write to a temp file then rename for atomicity.
	tmp := path + ".dag.tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	return os.Rename(tmp, path)
}

func (e *UnixExecutor) AppendFile(_ context.Context, path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("append open %q: %w", path, err)
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}

// EditFile uses a pure-Go line scanner for reliable cross-OS behaviour.
// Unlike sed, it handles files with Windows line-endings, binary-safe paths,
// and multi-line search patterns without shell injection risk.
func (e *UnixExecutor) EditFile(_ context.Context, path, oldText, newText string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("edit read %q: %w", path, err)
	}
	if !bytes.Contains(data, []byte(oldText)) {
		return fmt.Errorf("edit %q: search text not found", path)
	}
	replaced := bytes.Replace(data, []byte(oldText), []byte(newText), 1)
	tmp := path + ".dag.tmp"
	if err := os.WriteFile(tmp, replaced, 0o644); err != nil {
		return fmt.Errorf("edit write %q: %w", path, err)
	}
	return os.Rename(tmp, path)
}

func (e *UnixExecutor) Capabilities() []string {
	caps := []string{"go-io", "exec"}
	for _, bin := range []string{"bash", "sh", "grep", "sed", "awk", "git", "curl", "jq"} {
		if _, err := exec.LookPath(bin); err == nil {
			caps = append(caps, bin)
		}
	}
	if e.IsTermux {
		caps = append(caps, "termux")
	}
	return caps
}

func (e *UnixExecutor) Platform() string {
	if e.IsTermux {
		return "termux"
	}
	return runtime.GOOS
}

// ─── Windows Executor ─────────────────────────────────────────────────────────

// WindowsExecutor implements Executor for Windows using cmd.exe / PowerShell
// and native Go file I/O. It never uses Unix-only utilities like sed/grep.
type WindowsExecutor struct {
	// UsePS prefers PowerShell over cmd.exe when both are available.
	UsePS   bool
	Timeout time.Duration
}

func (e *WindowsExecutor) deadline(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	timeout := e.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return context.WithTimeout(ctx, timeout)
}

func (e *WindowsExecutor) Exec(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := e.deadline(ctx)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("exec %q: %w\n%s", name, err, string(out))
	}
	return string(out), nil
}

func (e *WindowsExecutor) Shell(ctx context.Context, cmdLine string) (string, error) {
	if e.UsePS {
		return e.Exec(ctx, "powershell", "-NoProfile", "-Command", cmdLine)
	}
	return e.Exec(ctx, "cmd", "/C", cmdLine)
}

func (e *WindowsExecutor) ReadFile(_ context.Context, path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %q: %w", path, err)
	}
	return string(b), nil
}

func (e *WindowsExecutor) WriteFile(_ context.Context, path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".dag.tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (e *WindowsExecutor) AppendFile(_ context.Context, path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}

// EditFile uses a bufio.Scanner-based replacement — no sed required.
func (e *WindowsExecutor) EditFile(_ context.Context, path, oldText, newText string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("edit open %q: %w", path, err)
	}

	var buf bytes.Buffer
	scanner := bufio.NewScanner(f)
	found := false
	for scanner.Scan() {
		line := scanner.Text()
		if !found && strings.Contains(line, oldText) {
			line = strings.Replace(line, oldText, newText, 1)
			found = true
		}
		buf.WriteString(line + "\r\n")
	}
	f.Close()

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("edit scan %q: %w", path, err)
	}
	if !found {
		return fmt.Errorf("edit %q: search text not found", path)
	}

	tmp := path + ".dag.tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (e *WindowsExecutor) Capabilities() []string {
	caps := []string{"go-io", "exec", "cmd"}
	if _, err := exec.LookPath("powershell"); err == nil {
		caps = append(caps, "powershell")
	}
	for _, bin := range []string{"git", "curl"} {
		if _, err := exec.LookPath(bin); err == nil {
			caps = append(caps, bin)
		}
	}
	return caps
}

func (e *WindowsExecutor) Platform() string { return "windows" }

// ─── Runtime factory ─────────────────────────────────────────────────────────

// NewExecutor returns the correct Executor implementation for the current
// runtime. Safe to call from init() or package-level var declarations.
func NewExecutor() Executor {
	if runtime.GOOS == "windows" {
		return &WindowsExecutor{}
	}
	_, isTermux := os.LookupEnv("TERMUX_VERSION")
	return &UnixExecutor{IsTermux: isTermux}
}
