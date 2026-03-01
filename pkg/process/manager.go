package process

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// ProcessState represents the current state of a process.
type ProcessState string

const (
	StatePending   ProcessState = "pending"
	StateRunning   ProcessState = "running"
	StateCompleted ProcessState = "completed"
	StateFailed    ProcessState = "failed"
	StateStopped   ProcessState = "stopped"
)

const maxOutputBytes = 10 * 1024 * 1024 // 10MB per process

// Process represents a managed system process.
type Process struct {
	ID        string
	Command   string
	Args      []string
	State     ProcessState
	StartTime time.Time
	EndTime   time.Time
	ExitCode  int
	Output    string // Buffered output for recall

	cmd *exec.Cmd
	pty *os.File // Pseudo-terminal file descriptor (if applicable)
	mu  sync.RWMutex
	ctx context.Context
	cancel context.CancelFunc

	// Output streaming (optional channels/writers)
	StdoutStream io.Writer
	StderrStream io.Writer

	// OnComplete is called when the process exits (exitCode, isError)
	OnComplete func(exitCode int, isError bool)
}

// GetOutput safely returns the buffered output.
func (p *Process) GetOutput() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.Output
}

// Manager handles the lifecycle of multiple processes.
type Manager struct {
	processes map[string]*Process
	mu        sync.RWMutex
}

// NewManager creates a new process manager instance.
func NewManager() *Manager {
	return &Manager{
		processes: make(map[string]*Process),
	}
}

// Start initiates a new process, optionally using a PTY for interactive support.
func (m *Manager) Start(id string, command string, args []string, usePty bool) (*Process, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.processes[id]; exists {
		return nil, fmt.Errorf("process with ID %s already exists", id)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, command, args...)
	
	// Inherit environment
	cmd.Env = os.Environ()

	proc := &Process{
		ID:        id,
		Command:   command,
		Args:      args,
		State:     StatePending,
		StartTime: time.Now(),
		cmd:       cmd,
		ctx:       ctx,
		cancel:    cancel,
	}

	if usePty {
		if err := m.startWithPty(proc); err != nil {
			cancel()
			return nil, err
		}
	} else {
		if err := m.startStandard(proc); err != nil {
			cancel()
			return nil, err
		}
	}

	m.processes[id] = proc
	return proc, nil
}

// startWithPty launches the command attached to a pseudo-terminal.
func (m *Manager) startWithPty(proc *Process) error {
	ptmx, err := pty.Start(proc.cmd)
	if err != nil {
		return fmt.Errorf("failed to start pty: %w", err)
	}
	proc.pty = ptmx
	proc.State = StateRunning

	// Copy output asynchronously
	go func() {
		// Read from pty master
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				chunk := string(buf[:n])
				proc.mu.Lock()
				proc.Output += chunk
				if len(proc.Output) > maxOutputBytes {
					proc.Output = proc.Output[len(proc.Output)-maxOutputBytes:]
				}
				proc.mu.Unlock()

				// Stream if configured
				if proc.StdoutStream != nil {
					proc.StdoutStream.Write(buf[:n])
				}
			}
			if err != nil {
				if err != io.EOF && !strings.Contains(err.Error(), "input/output error") {
					// PTY read error (often EIO on Linux when slave closes)
				}
				break
			}
		}
		
		// Wait for command completion
		err := proc.cmd.Wait()
		proc.mu.Lock()
		proc.EndTime = time.Now()
		isError := false
		if err != nil {
			proc.State = StateFailed
			isError = true
			if exitError, ok := err.(*exec.ExitError); ok {
				proc.ExitCode = exitError.ExitCode()
			} else {
				proc.ExitCode = -1
			}
		} else {
			proc.State = StateCompleted
			proc.ExitCode = 0
		}
		// Call completion callback if set
		if proc.OnComplete != nil {
			proc.OnComplete(proc.ExitCode, isError)
		}
		proc.mu.Unlock()

		ptmx.Close()
	}()

	return nil
}

// startStandard launches the command with standard pipes.
func (m *Manager) startStandard(proc *Process) error {
	stdoutPipe, err := proc.cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderrPipe, err := proc.cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := proc.cmd.Start(); err != nil {
		return err
	}

	proc.State = StateRunning

	// Handle stdout
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdoutPipe.Read(buf)
			if n > 0 {
				chunk := string(buf[:n])
				proc.mu.Lock()
				proc.Output += chunk
				if len(proc.Output) > maxOutputBytes {
					proc.Output = proc.Output[len(proc.Output)-maxOutputBytes:]
				}
				proc.mu.Unlock()
				if proc.StdoutStream != nil {
					proc.StdoutStream.Write(buf[:n])
				}
			}
			if err != nil {
				break
			}
		}
	}()

	// Handle stderr
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stderrPipe.Read(buf)
			if n > 0 {
				chunk := string(buf[:n])
				proc.mu.Lock()
				proc.Output += chunk
				if len(proc.Output) > maxOutputBytes {
					proc.Output = proc.Output[len(proc.Output)-maxOutputBytes:]
				}
				proc.mu.Unlock()
				if proc.StderrStream != nil {
					proc.StderrStream.Write(buf[:n])
				}
			}
			if err != nil {
				break
			}
		}
	}()

	// Wait routine
	go func() {
		err := proc.cmd.Wait()
		proc.mu.Lock()
		proc.EndTime = time.Now()
		isError := false
		if err != nil {
			proc.State = StateFailed
			isError = true
			if exitError, ok := err.(*exec.ExitError); ok {
				proc.ExitCode = exitError.ExitCode()
			} else {
				proc.ExitCode = -1
			}
		} else {
			proc.State = StateCompleted
			proc.ExitCode = 0
		}
		// Call completion callback if set
		if proc.OnComplete != nil {
			proc.OnComplete(proc.ExitCode, isError)
		}
		proc.mu.Unlock()
	}()

	return nil
}

// Stop terminates a running process.
func (m *Manager) Stop(id string) error {
	m.mu.Lock()
	proc, exists := m.processes[id]
	m.mu.Unlock()

	if !exists {
		return fmt.Errorf("process %s not found", id)
	}

	proc.mu.Lock()
	defer proc.mu.Unlock()

	if proc.State != StateRunning {
		return nil // Already stopped
	}

	// Try graceful termination first if PTY
	if proc.pty != nil {
		// Send SIGINT or similar? 
		// For now, let's just use the context cancel or process kill
	}

	if proc.cmd != nil && proc.cmd.Process != nil {
		// Send SIGTERM
		if err := proc.cmd.Process.Signal(syscall.SIGTERM); err != nil {
			// Force kill if SIGTERM fails
			proc.cmd.Process.Kill()
		}
	}
	
	proc.cancel() // Cancel context
	proc.State = StateStopped
	proc.EndTime = time.Now()

	return nil
}

// GetProcess retrieves a process by ID.
func (m *Manager) GetProcess(id string) (*Process, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.processes[id]
	return p, ok
}

// ListProcesses returns a snapshot of all managed processes.
func (m *Manager) ListProcesses() []*Process {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]*Process, 0, len(m.processes))
	for _, p := range m.processes {
		list = append(list, p)
	}
	return list
}

// Cleanup removes completed/failed/stopped processes older than duration.
func (m *Manager) Cleanup(olderThan time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for id, p := range m.processes {
		p.mu.RLock()
		if p.State != StateRunning && now.Sub(p.EndTime) > olderThan {
			delete(m.processes, id)
		}
		p.mu.RUnlock()
	}
}
