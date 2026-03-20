//go:build !windows

package orchestrator

import (
	"os/exec"
	"syscall"
)

// setupCmdPlatformAgnostic isolates the subprocess in its own process group
// so that signals sent to Gorkbot are not propagated to the child.
func setupCmdPlatformAgnostic(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup sends SIGKILL to every process in the group rooted at
// cmd.Process.Pid (using the negative-PID convention on Unix), then also
// kills the leader directly as a belt-and-suspenders fallback.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	// Negative PID targets the entire process group.
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	// Belt-and-suspenders: kill the group leader directly.
	_ = cmd.Process.Kill()
}
