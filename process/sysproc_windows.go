//go:build windows

package process

import "os/exec"

// ConfigureSysProcAttr is a no-op on Windows:
// there is no Setpgid equivalent in the standard library
// and exec.CommandContext handles child cleanup adequately for the cases this package targets.
func ConfigureSysProcAttr(c *exec.Cmd) {}

// TerminateGracefully signals the child to stop. Windows lacks a direct analogue of SIGTERM,
// so we ask the OS to kill the process;
// WaitDelay still bounds shutdown if the call returns immediately.
func TerminateGracefully(c *exec.Cmd) error {
	if c.Process == nil {
		return nil
	}
	return c.Process.Kill()
}

func configureSysProcAttr(c *exec.Cmd) { ConfigureSysProcAttr(c) }

func terminateGracefully(c *exec.Cmd) error { return TerminateGracefully(c) }
