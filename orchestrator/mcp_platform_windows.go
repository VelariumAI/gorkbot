//go:build windows

package orchestrator

import (
	"os/exec"
)

// setupCmdPlatformAgnostic is a no-op on Windows; child processes inherit the
// parent's console group by default which is sufficient for our use-case.
func setupCmdPlatformAgnostic(cmd *exec.Cmd) {}

// killProcessGroup terminates the process on Windows. Windows does not have
// Unix-style process groups, so we simply kill the leader process.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
