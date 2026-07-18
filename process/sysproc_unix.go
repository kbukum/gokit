//go:build !windows

package process

import (
	"os/exec"
	"syscall"
)

// ConfigureSysProcAttr places the child in its own process group
// so we can signal the entire tree on cancellation.
// No-op on platforms (such as Windows) that do not support process groups.
func ConfigureSysProcAttr(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// TerminateGracefully sends SIGTERM to the child's process group
// so any grandchildren are also signaled. Callers should set cmd.WaitDelay
// so the runtime escalates to SIGKILL if the child does not exit in time.
// On Windows this falls back to os.Process.Kill.
func TerminateGracefully(c *exec.Cmd) error {
	if c.Process == nil {
		return nil
	}
	return syscall.Kill(-c.Process.Pid, syscall.SIGTERM)
}

func configureSysProcAttr(c *exec.Cmd) { ConfigureSysProcAttr(c) }

func terminateGracefully(c *exec.Cmd) error { return TerminateGracefully(c) }
