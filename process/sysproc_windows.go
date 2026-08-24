//go:build windows

package process

import (
	stderrors "errors"
	"os/exec"
)

// ConfigureSysProcAttr is a no-op on Windows:
// there is no Setpgid equivalent in the standard library
// and exec.CommandContext handles child cleanup adequately for the cases this package targets.
func ConfigureSysProcAttr(c *exec.Cmd) {}

// TerminateGracefully signals the child to stop. Windows lacks a direct analogue of SIGTERM,
// so we ask the OS to kill the process;
// WaitDelay still bounds shutdown if the call returns immediately.
func TerminateGracefully(c *exec.Cmd) error {
	return TerminateGroup(c)
}

// interruptGroup falls back to killing the process on Windows,
// which has no process-group interrupt analogue in the standard library.
func interruptGroup(c *exec.Cmd) error { return c.Process.Kill() }

func terminateGroup(c *exec.Cmd) error { return c.Process.Kill() }

func killGroup(c *exec.Cmd) error { return c.Process.Kill() }

// terminatePIDGroup cannot signal a foreign pid on Windows without an open handle,
// so it returns an honest error.
func terminatePIDGroup(_ int, _ bool) error {
	return errWindowsPIDSignal
}

// killPIDGroup cannot signal a foreign pid on Windows without an open handle.
func killPIDGroup(_ int, _ bool) error {
	return errWindowsPIDSignal
}

// pidGroupAlive cannot inspect a foreign pid on Windows and conservatively reports false.
func pidGroupAlive(_ int, _ bool) bool { return false }

var errWindowsPIDSignal = stderrors.New("process: cannot signal a tracked pid on Windows without an owned handle")

func configureSysProcAttr(c *exec.Cmd) { ConfigureSysProcAttr(c) }

func terminateGracefully(c *exec.Cmd) error { return TerminateGracefully(c) }

// signalErrIsGone reports whether a signal error means the target is already gone.
// Windows teardown goes through os.Process.Kill, which reports os.ErrProcessDone (handled
// by the shared caller), so there is no additional platform errno to recognize here.
func signalErrIsGone(error) bool { return false }

// isTextFileBusy reports whether err is a transient ETXTBSY. Windows has no ETXTBSY
// analogue for exec, so a start failure there is never treated as transiently busy.
func isTextFileBusy(error) bool { return false }
