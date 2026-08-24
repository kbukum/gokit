package process

import (
	"context"
	stderrors "errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	goerrors "github.com/kbukum/gokit/errors"
)

// Run executes a subprocess and waits for it to complete. If the context is canceled,
// the process group receives SIGTERM on Unix (or the process is killed on Windows),
// then the runtime escalates to SIGKILL after the grace period via WaitDelay. When
// cmd.IO is IOInherited the child's stdout/stderr are wired to the parent terminal and
// nothing is captured; otherwise stdout/stderr are captured into Result.
func Run(ctx context.Context, cmd Command) (*Result, error) {
	if cmd.Binary == "" {
		return nil, goerrors.MissingField("binary")
	}

	policy := resolveLifecycle(cmd)

	c := exec.CommandContext(ctx, cmd.Binary, cmd.Args...) //nolint:gosec // dynamic args are the purpose of this package
	c.Dir = cmd.Dir
	c.Env = mergeEnv(cmd.Env, cmd.ScrubEnv)
	applyInput(c, cmd)

	var stdout, stderr *limitedBuffer
	if cmd.IO == IOInherited {
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
	} else {
		stdout = newLimitedBuffer(cmd.MaxOutputBytes)
		stderr = newLimitedBuffer(cmd.MaxOutputBytes)
		c.Stdout = stdout
		c.Stderr = stderr
	}

	applyLifecycle(c, policy)

	start := time.Now()
	// c.Run() is Start()+Wait(); split so the start races on ETXTBSY are retried
	// while a just-written executable's writable descriptor closes.
	err := startWithETXTBSYRetry(c)
	if err == nil {
		err = c.Wait()
	}
	duration := time.Since(start)

	exitCode := -1
	if c.ProcessState != nil {
		exitCode = c.ProcessState.ExitCode()
	}

	result := &Result{
		ExitCode: exitCode,
		Duration: duration,
	}
	if stdout != nil {
		result.Stdout = stdout.Bytes()
		result.StdoutTruncated = stdout.Truncated()
		result.Stderr = stderr.Bytes()
		result.StderrTruncated = stderr.Truncated()
	}

	if err != nil {
		return result, classifyRunError(ctx, cmd, result, err)
	}

	return result, nil
}

// classifyRunError maps a failed subprocess execution to a typed error and annotates the
// result with timeout/cancellation state. Context cancellation is the expected way to kill
// a process; a failure to start is classified via SpawnError.
func classifyRunError(ctx context.Context, cmd Command, result *Result, err error) error {
	switch {
	case stderrors.Is(ctx.Err(), context.DeadlineExceeded):
		result.TimedOut = true
		return goerrors.Timeout("process").WithCause(err)
	case stderrors.Is(ctx.Err(), context.Canceled):
		result.Canceled = true
		return goerrors.Canceled("process").WithCause(err)
	}

	var exitErr *exec.ExitError
	if !stderrors.As(err, &exitErr) {
		return SpawnError(fmt.Sprintf("process: start %s", cmd.Binary), err)
	}
	return fmt.Errorf("process: exit code %d: %w", result.ExitCode, err)
}

// applyLifecycle configures process-group isolation and the graceful cancel path on c.
func applyLifecycle(c *exec.Cmd, policy LifecyclePolicy) {
	if policy.IsolateProcessGroup {
		configureSysProcAttr(c)
	}
	c.Cancel = func() error {
		if policy.targetsGroup() {
			return terminateGracefully(c)
		}
		return c.Process.Kill()
	}
	if policy.KillAfterGrace {
		c.WaitDelay = policy.grace()
	}
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
