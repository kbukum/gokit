package process

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// Run executes a subprocess and waits for it to complete. If the context is canceled,
// the process group receives SIGTERM on Unix (or the process is killed on Windows),
// then the runtime escalates to SIGKILL after GracePeriod via WaitDelay.
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
	c.Env = mergeEnv(cmd.Env, cmd.ScrubEnv)

	stdout := newLimitedBuffer(cmd.MaxOutputBytes)
	stderr := newLimitedBuffer(cmd.MaxOutputBytes)
	c.Stdout = stdout
	c.Stderr = stderr

	if cmd.Stdin != nil {
		c.Stdin = cmd.Stdin
	}

	configureSysProcAttr(c)

	// Don't let exec.CommandContext kill with SIGKILL immediately;
	// graceful-terminate via platform-specific helper.
	c.Cancel = func() error {
		return terminateGracefully(c)
	}
	c.WaitDelay = gracePeriod

	start := time.Now()
	err := c.Run()
	duration := time.Since(start)

	exitCode := -1
	if c.ProcessState != nil {
		exitCode = c.ProcessState.ExitCode()
	}

	result := &Result{
		Stdout:          stdout.Bytes(),
		StdoutTruncated: stdout.Truncated(),
		Stderr:          stderr.Bytes(),
		StderrTruncated: stderr.Truncated(),
		ExitCode:        exitCode,
		Duration:        duration,
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

// mergeEnv prepares the process environment.
func mergeEnv(extra []string, scrub bool) []string {
	if scrub {
		return append([]string{}, extra...)
	}
	if len(extra) == 0 {
		return nil // inherit parent env
	}
	env := os.Environ()
	return append(env, extra...)
}
