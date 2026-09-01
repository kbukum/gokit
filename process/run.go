package process

import (
	"context"
	stderrors "errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
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

	var stdout, stderr *limitedBuffer
	if cmd.IO != IOInherited {
		stdout = newLimitedBuffer(cmd.MaxOutputBytes)
		stderr = newLimitedBuffer(cmd.MaxOutputBytes)
	}

	// A fresh command is built on every start attempt: an ETXTBSY retry cannot reuse a
	// Cmd whose Start already failed, because Cmd refuses a second Start. The capture
	// buffers are stable across attempts since a failed start writes nothing to them.
	newCmd := func() *exec.Cmd {
		c := exec.CommandContext(ctx, cmd.Binary, cmd.Args...) //nolint:gosec // dynamic args are the purpose of this package
		c.Dir = cmd.Dir
		c.Env = mergeEnv(cmd.Env, cmd.EnvPolicy)
		applyInput(c, cmd)
		if cmd.IO == IOInherited {
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
		} else {
			c.Stdout = stdout
			c.Stderr = stderr
		}
		applyLifecycle(c, policy)
		return c
	}

	start := time.Now()
	// c.Run() is Start()+Wait(); split so the start races on ETXTBSY are retried
	// while a just-written executable's writable descriptor closes.
	var c *exec.Cmd
	err := startWithETXTBSYRetry(func() error {
		c = newCmd()
		return c.Start()
	})
	if err == nil {
		err = c.Wait()
	}
	duration := time.Since(start)

	result := &Result{
		ExitCode: exitCodeOf(c.ProcessState),
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
	return goerrors.Internal(
		fmt.Errorf("process: exit code %s: %w", exitCodeLabel(result.ExitCode), err),
	).WithDetail("exit_code", result.ExitCodeOr(-1))
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

// mergeEnv prepares the process environment from the command's Env map and EnvPolicy.
// A nil result inherits the parent environment unchanged; a non-nil (possibly empty) result
// is passed to exec verbatim, so EnvEmpty always yields a non-nil slice. When inheriting,
// Env entries are merged onto the parent by key so each variable appears exactly once and
// the explicit override always wins — appending duplicates would let a child select the
// inherited value instead, contrary to the documented override semantics.
func mergeEnv(extra map[string]string, policy EnvPolicy) []string {
	if policy == EnvEmpty {
		return envSlice(extra) // non-nil, even when empty: start from an empty environment
	}
	if len(extra) == 0 {
		return nil // inherit parent env unchanged
	}
	merged := make(map[string]string, len(extra))
	for _, kv := range os.Environ() {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			merged[kv[:i]] = kv[i+1:]
		}
	}
	for k, v := range extra {
		merged[k] = v
	}
	return envSlice(merged)
}

// envSlice renders an environment map as a non-nil sorted key=value slice.
func envSlice(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

// exitCodeOf reads a completed process's exit code, returning nil when the process was
// killed by a signal or never produced a status (an explicit unknown-exit representation
// rather than a -1 sentinel).
func exitCodeOf(state *os.ProcessState) *int {
	if state == nil {
		return nil
	}
	code := state.ExitCode()
	if code < 0 {
		return nil
	}
	return &code
}
