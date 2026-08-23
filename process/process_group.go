package process

import "os/exec"

// InterruptGroup asks the command's process to stop gracefully. On Unix it sends SIGINT
// to the child's process group so descendants are signaled too; on platforms without
// process groups it falls back to signaling the immediate process. It is a no-op when the
// process has not started.
func InterruptGroup(c *exec.Cmd) error {
	if c == nil || c.Process == nil {
		return nil
	}
	return interruptGroup(c)
}

// TerminateGroup requests graceful termination. On Unix it sends SIGTERM to the child's
// process group; on other platforms it falls back to Process.Kill or returns an honest
// error. It is a no-op when the process has not started.
func TerminateGroup(c *exec.Cmd) error {
	if c == nil || c.Process == nil {
		return nil
	}
	return terminateGroup(c)
}

// KillGroup force-kills the process. On Unix it sends SIGKILL to the child's process group;
// on other platforms it falls back to Process.Kill. It is a no-op when the process has not
// started.
func KillGroup(c *exec.Cmd) error {
	if c == nil || c.Process == nil {
		return nil
	}
	return killGroup(c)
}
