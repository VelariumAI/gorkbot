package orchestrator

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ProcessManager owns a single external MCP subprocess and its I/O pipes.
// It is intentionally single-use: create a new instance for each (re)start.
type ProcessManager struct {
	command string
	args    []string
	env     []string
	logger  *slog.Logger

	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	pipeW  *io.PipeWriter  // write-end of the stdout relay pipe; closed in reap()
	cancel context.CancelFunc

	// dead is closed exactly once when the subprocess exits. It is the primary
	// signal for the watchdog goroutine to detect unexpected termination.
	dead chan struct{}
}

// NewProcessManager constructs a ProcessManager ready to Start().
func NewProcessManager(command string, args []string, env []string, logger *slog.Logger) *ProcessManager {
	return &ProcessManager{
		command: command,
		args:    args,
		env:     env,
		logger:  logger,
		dead:    make(chan struct{}),
	}
}

// Dead returns a channel that is closed once when the subprocess exits.
func (pm *ProcessManager) Dead() <-chan struct{} { return pm.dead }

// IsAlive reports whether the subprocess is still running.
func (pm *ProcessManager) IsAlive() bool {
	select {
	case <-pm.dead:
		return false
	default:
		return true
	}
}

// Wait blocks until the subprocess exits.
func (pm *ProcessManager) Wait() { <-pm.dead }

// Start resolves the binary, launches the subprocess, and starts background
// goroutines for stdout relay, stderr draining, and process reaping.
// It returns (stdin, pipeR, error) where pipeR is the read-end of an io.Pipe
// that receives all subprocess stdout — safe to hand to the MCP transport.
func (pm *ProcessManager) Start(ctx context.Context) (io.WriteCloser, io.Reader, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	executable, err := resolveBinary(pm.command)
	if err != nil {
		return nil, nil, fmt.Errorf("binary resolution failed for %q: %w", pm.command, err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	pm.cancel = cancel

	pm.cmd = exec.CommandContext(runCtx, executable, pm.args...)
	pm.cmd.Env = append(os.Environ(), pm.env...)
	setupCmdPlatformAgnostic(pm.cmd)

	pm.stdin, err = pm.cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("stdin pipe: %w", err)
	}

	stdoutPipe, err := pm.cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("stdout pipe: %w", err)
	}

	stderrPipe, err := pm.cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := pm.cmd.Start(); err != nil {
		cancel()
		return nil, nil, fmt.Errorf("subprocess start failed: %w", err)
	}

	pipeR, pipeW := io.Pipe()
	pm.pipeW = pipeW

	go pm.relayStdout(pipeW, stdoutPipe)
	go pm.drainStderr(stderrPipe)
	go pm.reap()

	pm.logger.Info("MCP subprocess started", "pid", pm.cmd.Process.Pid, "cmd", executable)
	return pm.stdin, pipeR, nil
}

// relayStdout copies subprocess stdout into pipeW.
//
// Critical: if pipeW.Write() fails (because the MCP client closed pipeR), we
// must drain the remaining subprocess stdout with io.Copy(io.Discard, stdout).
// Without this drain, the OS pipe buffer fills up, the subprocess blocks on
// its next write, and cmd.Wait() in reap() deadlocks.
func (pm *ProcessManager) relayStdout(pw *io.PipeWriter, stdout io.ReadCloser) {
	buf := make([]byte, 32*1024)
	for {
		n, readErr := stdout.Read(buf)
		if n > 0 {
			if _, writeErr := pw.Write(buf[:n]); writeErr != nil {
				// Transport closed the read-end. Drain remaining output to
				// prevent the subprocess from blocking on a full pipe buffer,
				// which would deadlock cmd.Wait() in reap().
				_, _ = io.Copy(io.Discard, stdout)
				return
			}
		}
		if readErr != nil {
			pw.CloseWithError(readErr)
			return
		}
	}
}

// drainStderr logs all stderr lines from the subprocess, then discards any
// remaining bytes to prevent cmd.Wait() from blocking on a full stderr buffer.
func (pm *ProcessManager) drainStderr(r io.ReadCloser) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		pm.logger.Debug("MCP stderr", "server", pm.command, "line", scanner.Text())
	}
	// Drain anything the scanner left behind.
	_, _ = io.Copy(io.Discard, r)
}

// reap waits for the subprocess to exit, then closes pm.dead to signal
// the watchdog, and closes pipeW so the MCP transport sees io.EOF.
func (pm *ProcessManager) reap() {
	_ = pm.cmd.Wait()
	close(pm.dead)
	if pm.pipeW != nil {
		pm.pipeW.CloseWithError(io.EOF)
	}
	pm.logger.Debug("MCP subprocess reaped", "cmd", pm.command)
}

// Stop terminates the subprocess with a tiered shutdown sequence:
//  1. Cancel the run context (causes exec.CommandContext to send SIGKILL after
//     Go 1.20+, or SIGKILL immediately before — depends on version).
//  2. Send os.Interrupt (SIGINT on Unix) for a graceful shutdown attempt.
//  3. Wait up to 2 s for the process to exit via pm.dead.
//  4. Force-kill the entire process group if still alive.
func (pm *ProcessManager) Stop() error {
	pm.mu.Lock()
	cancel := pm.cancel
	cmd := pm.cmd
	pm.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	if cmd == nil || cmd.Process == nil {
		return nil
	}

	pm.logger.Info("Stopping MCP subprocess", "pid", cmd.Process.Pid)
	_ = cmd.Process.Signal(os.Interrupt)

	select {
	case <-pm.dead:
		pm.logger.Debug("MCP subprocess exited cleanly after interrupt")
	case <-time.After(2 * time.Second):
		pm.logger.Warn("MCP subprocess did not exit gracefully, killing process group")
		killProcessGroup(cmd)
		// Give the OS a moment to reap the processes.
		select {
		case <-pm.dead:
		case <-time.After(500 * time.Millisecond):
		}
	}
	return nil
}

// ── Binary resolution ──────────────────────────────────────────────────────────

// binaryCandidates returns an ordered list of binary names to try for the
// given command. This enables platform-agnostic invocation of Python and Node
// tools regardless of the exact binary name in the user's PATH.
func binaryCandidates(name string) []string {
	switch strings.ToLower(name) {
	case "python", "python3":
		return []string{"python3", "python", "uvx"}
	case "npx", "node":
		return []string{"npx", "node", "npm"}
	default:
		return []string{name}
	}
}

// resolveBinary finds the absolute path of the first candidate that exists on
// PATH. It is a package-level function so both ProcessManager and the shaper
// runner in mcp_client.go can use it.
func resolveBinary(name string) (string, error) {
	candidates := binaryCandidates(name)
	for _, c := range candidates {
		if path, err := exec.LookPath(c); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("none of %v found on PATH", candidates)
}
