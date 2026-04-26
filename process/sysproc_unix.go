//go:build !windows

package process

import (
	"os/exec"
	"syscall"
)

// configureSysProcAttr places the child in its own process group so we
// can signal the entire tree on cancellation.
func configureSysProcAttr(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// terminateGracefully sends SIGTERM to the child's process group so any
// grandchildren are also signalled. WaitDelay handles SIGKILL escalation.
func terminateGracefully(c *exec.Cmd) error {
	if c.Process == nil {
		return nil
	}
	return syscall.Kill(-c.Process.Pid, syscall.SIGTERM)
}
