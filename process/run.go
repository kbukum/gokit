package process

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// Run executes a subprocess and waits for it to complete.
// If the context is canceled, SIGTERM is sent first, then SIGKILL after GracePeriod.
func Run(ctx context.Context, cmd Command) (*Result, error) {
	if cmd.Binary == "" {
		return nil, fmt.Errorf("process: binary is required")
	}

	gracePeriod := cmd.GracePeriod
	if gracePeriod == 0 {
		gracePeriod = 5 * time.Second
	}

	c := exec.CommandContext(ctx, cmd.Binary, cmd.Args...) //nolint:gosec // dynamic args are the purpose of this package
	c.Dir = cmd.Dir
	c.Env = mergeEnv(cmd.Env)

	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr

	if cmd.Stdin != nil {
		c.Stdin = cmd.Stdin
	}

	// Use process group so we can kill the entire tree
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Don't let exec.CommandContext kill with SIGKILL immediately
	c.Cancel = func() error {
		if c.Process == nil {
			return nil
		}
		return syscall.Kill(-c.Process.Pid, syscall.SIGTERM)
	}
	c.WaitDelay = gracePeriod

	start := time.Now()
	err := c.Run()
	duration := time.Since(start)

	result := &Result{
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
		ExitCode: c.ProcessState.ExitCode(),
		Duration: duration,
	}

	if err != nil {
		// Context cancellation is the expected way to kill a process
		if ctx.Err() != nil {
			return result, fmt.Errorf("process: killed by context: %w", ctx.Err())
		}
		return result, fmt.Errorf("process: exit code %d: %w", result.ExitCode, err)
	}

	return result, nil
}

// mergeEnv merges additional env vars with the current environment.
func mergeEnv(extra []string) []string {
	if len(extra) == 0 {
		return nil // inherit parent env
	}
	env := os.Environ()
	return append(env, extra...)
}
