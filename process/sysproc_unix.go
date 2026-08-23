//go:build !windows

package process

import (
	stderrors "errors"
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
	return TerminateGroup(c)
}

// signalGroup sends sig to the child's process group.
func signalGroup(c *exec.Cmd, sig syscall.Signal) error {
	return syscall.Kill(-c.Process.Pid, sig)
}

func interruptGroup(c *exec.Cmd) error { return signalGroup(c, syscall.SIGINT) }

func terminateGroup(c *exec.Cmd) error { return signalGroup(c, syscall.SIGTERM) }

func killGroup(c *exec.Cmd) error { return signalGroup(c, syscall.SIGKILL) }

// terminatePIDGroup sends SIGTERM to a tracked pid (or its group when group is true).
func terminatePIDGroup(pid int, group bool) error {
	return signalPID(pid, syscall.SIGTERM, group)
}

// killPIDGroup sends SIGKILL to a tracked pid (or its group when group is true).
func killPIDGroup(pid int, group bool) error {
	return signalPID(pid, syscall.SIGKILL, group)
}

// pidGroupAlive reports whether a tracked pid (or its group) still exists.
func pidGroupAlive(pid int, group bool) bool {
	err := signalPID(pid, 0, group)
	return err == nil || stderrors.Is(err, syscall.EPERM)
}

func signalPID(pid int, sig syscall.Signal, group bool) error {
	target := pid
	if group {
		target = -pid
	}
	return syscall.Kill(target, sig)
}

func configureSysProcAttr(c *exec.Cmd) { ConfigureSysProcAttr(c) }

func terminateGracefully(c *exec.Cmd) error { return TerminateGracefully(c) }

// signalErrIsGone reports whether a signal error means the target is already gone.
// On Unix a signal to a reaped process (or its group) fails with ESRCH.
func signalErrIsGone(err error) bool {
	return stderrors.Is(err, syscall.ESRCH)
}
