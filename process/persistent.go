package process

import (
	"bytes"
	"context"
	stderrors "errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	goerrors "github.com/kbukum/gokit/errors"
)

// PersistentReadiness selects how StartPersistent decides a long-lived process is ready.
type PersistentReadiness int

const (
	// ReadyImmediate treats the process as ready as soon as it is spawned.
	ReadyImmediate PersistentReadiness = iota
	// ReadyOnOutput waits until either output stream contains OutputMarker.
	ReadyOnOutput
	// ReadyAfterDelay waits ReadyDelay after spawn before declaring readiness.
	ReadyAfterDelay
)

// PersistentConfig configures a persistent (long-lived) subprocess.
type PersistentConfig struct {
	// Readiness selects the readiness strategy. Defaults to ReadyImmediate.
	Readiness PersistentReadiness
	// OutputMarker is the substring awaited when Readiness is ReadyOnOutput.
	OutputMarker string
	// ReadyDelay is the wait applied when Readiness is ReadyAfterDelay.
	ReadyDelay time.Duration
	// ReadinessTimeout bounds how long StartPersistent waits for readiness. Defaults to 30s.
	ReadinessTimeout time.Duration
	// ShutdownGracePeriod is the wait after graceful termination before a force kill. Defaults to 5s.
	ShutdownGracePeriod time.Duration
	// MaxCaptureBytes bounds retained startup output per stream. Zero or negative means unbounded.
	MaxCaptureBytes int
	// Lifecycle configures process-group isolation and shutdown escalation.
	Lifecycle LifecyclePolicy
}

// DefaultPersistentConfig returns a config that is ready immediately with a 30s readiness
// timeout, a 5s shutdown grace period, and the default lifecycle policy.
func DefaultPersistentConfig() PersistentConfig {
	return PersistentConfig{
		Readiness:           ReadyImmediate,
		ReadinessTimeout:    30 * time.Second,
		ShutdownGracePeriod: DefaultGracePeriod,
		Lifecycle:           DefaultLifecyclePolicy(),
	}
}

func (c PersistentConfig) normalized() PersistentConfig {
	if c.ReadinessTimeout <= 0 {
		c.ReadinessTimeout = 30 * time.Second
	}
	if c.ShutdownGracePeriod <= 0 {
		c.ShutdownGracePeriod = DefaultGracePeriod
	}
	if c.Lifecycle == (LifecyclePolicy{}) {
		c.Lifecycle = DefaultLifecyclePolicy()
	}
	if c.Lifecycle.GracePeriod <= 0 {
		c.Lifecycle.GracePeriod = c.ShutdownGracePeriod
	}
	return c
}

// PersistentStartup holds the output captured while waiting for readiness.
type PersistentStartup struct {
	// Stdout is the stdout captured at the moment readiness completed.
	Stdout []byte
	// StdoutTruncated reports whether startup stdout exceeded MaxCaptureBytes.
	StdoutTruncated bool
	// Stderr is the stderr captured at the moment readiness completed.
	Stderr []byte
	// StderrTruncated reports whether startup stderr exceeded MaxCaptureBytes.
	StderrTruncated bool
	// Duration is the time from spawn until readiness completed.
	Duration time.Duration
}

// PersistentRun is the result of starting a persistent process: the startup output snapshot
// and the running process handle.
type PersistentRun struct {
	// Startup is the output captured up to the moment readiness completed.
	Startup PersistentStartup
	// Process is the running persistent process handle.
	Process *PersistentProcess
}

// ShutdownOutcome describes how a persistent process ended.
type ShutdownOutcome struct {
	// AlreadyExited reports whether the process had already exited before shutdown was requested.
	AlreadyExited bool
	// Result is the completed process result.
	Result *Result
}

// guardedBuffer is a limitedBuffer safe for concurrent writes and snapshots.
type guardedBuffer struct {
	mu  sync.Mutex
	buf *limitedBuffer
}

func newGuardedBuffer(limit int) *guardedBuffer {
	return &guardedBuffer{buf: newLimitedBuffer(limit)}
}

func (g *guardedBuffer) Write(p []byte) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.buf.Write(p)
}

func (g *guardedBuffer) snapshot() ([]byte, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	b := append([]byte(nil), g.buf.Bytes()...)
	return b, g.buf.Truncated()
}

// PersistentProcess is a running long-lived subprocess with graceful shutdown.
type PersistentProcess struct {
	cmd    *exec.Cmd
	policy LifecyclePolicy
	grace  time.Duration
	start  time.Time

	stdout *guardedBuffer
	stderr *guardedBuffer

	waitCh chan struct{} // closed when cmd.Wait returns

	mu      sync.Mutex
	stopped bool
}

// StartPersistent spawns a long-lived subprocess and waits for it to become ready per cfg.
// On success it returns the startup output snapshot and a handle for waiting or shutting
// the process down. On failure it tears the process down and returns a classified AppError
// whose kind is retrievable via StartErrorKind.
func StartPersistent(ctx context.Context, cmd Command, cfg PersistentConfig) (*PersistentRun, error) {
	if cmd.Binary == "" {
		return nil, goerrors.MissingField("binary")
	}
	cfg = cfg.normalized()
	if cfg.Readiness == ReadyOnOutput && cfg.OutputMarker == "" {
		return nil, goerrors.InvalidInput("readiness.output_marker", "output readiness marker must not be empty")
	}
	if err := persistentStartupContextError(ctx); err != nil {
		return nil, err
	}

	// The persistent lifecycle is owned by the returned handle (Wait/Shutdown), so the
	// spawn context is detached from cancellation; readiness still honors ctx below.
	c := exec.CommandContext(context.WithoutCancel(ctx), cmd.Binary, cmd.Args...) //nolint:gosec // dynamic args are the purpose of this package
	c.Dir = cmd.Dir
	c.Env = mergeEnv(cmd.Env, cmd.ScrubEnv)
	applyInput(c, cmd)
	if cfg.Lifecycle.IsolateProcessGroup {
		configureSysProcAttr(c)
	}

	stdoutPipe, err := c.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("process: stdout pipe: %w", err)
	}
	stderrPipe, err := c.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("process: stderr pipe: %w", err)
	}

	p := &PersistentProcess{
		cmd:    c,
		policy: cfg.Lifecycle,
		grace:  cfg.ShutdownGracePeriod,
		stdout: newGuardedBuffer(cfg.MaxCaptureBytes),
		stderr: newGuardedBuffer(cfg.MaxCaptureBytes),
		waitCh: make(chan struct{}),
	}

	if err := c.Start(); err != nil {
		return nil, withStartErrorKind(
			goerrors.Wrap(SpawnError(fmt.Sprintf("process: start %s", cmd.Binary), err)),
			PersistentStartSpawnFailed,
		)
	}
	p.start = time.Now()

	readyCh := make(chan struct{})
	var readyOnce sync.Once
	marker := []byte(cfg.OutputMarker)
	signalReady := func() { readyOnce.Do(func() { close(readyCh) }) }

	var readersWG sync.WaitGroup
	readersWG.Add(2)
	go p.readInto(stdoutPipe, p.stdout, marker, signalReady, &readersWG)
	go p.readInto(stderrPipe, p.stderr, marker, signalReady, &readersWG)

	readersDone := make(chan struct{})
	go func() {
		readersWG.Wait()
		close(readersDone)
		_ = c.Wait()
		close(p.waitCh)
	}()

	if err := p.awaitReady(ctx, cfg, readyCh, readersDone); err != nil {
		_ = KillGroup(c)
		<-p.waitCh
		return nil, err
	}

	stdout, stdoutTrunc := p.stdout.snapshot()
	stderr, stderrTrunc := p.stderr.snapshot()
	return &PersistentRun{
		Startup: PersistentStartup{
			Stdout:          stdout,
			StdoutTruncated: stdoutTrunc,
			Stderr:          stderr,
			StderrTruncated: stderrTrunc,
			Duration:        time.Since(p.start),
		},
		Process: p,
	}, nil
}

// readInto copies a pipe into a guarded buffer, signaling readiness when the marker appears.
// Marker detection scans only each freshly read chunk plus a small carry of the previous
// chunk's trailing bytes (so a marker split across reads is still found), keeping the scan
// linear in total output rather than rescanning the whole accumulated buffer per read.
func (p *PersistentProcess) readInto(r io.Reader, dst *guardedBuffer, marker []byte, signalReady func(), wg *sync.WaitGroup) {
	defer wg.Done()
	buf := make([]byte, 32*1024)
	found := len(marker) == 0
	var carry []byte
	for {
		n, err := r.Read(buf)
		if n > 0 {
			_, _ = dst.Write(buf[:n])
			if !found {
				window := make([]byte, 0, len(carry)+n)
				window = append(window, carry...)
				window = append(window, buf[:n]...)
				if bytes.Contains(window, marker) {
					found = true
					carry = nil
					signalReady()
				} else {
					keep := min(len(marker)-1, len(window))
					carry = append(carry[:0], window[len(window)-keep:]...)
				}
			}
		}
		if err != nil {
			return
		}
	}
}

// awaitReady blocks until the configured readiness condition, a terminal state, or timeout.
func (p *PersistentProcess) awaitReady(ctx context.Context, cfg PersistentConfig, readyCh, readersDone chan struct{}) error {
	timeout := time.NewTimer(cfg.ReadinessTimeout)
	defer timeout.Stop()

	switch cfg.Readiness {
	case ReadyImmediate:
		select {
		case <-p.waitCh:
			return p.exitedBeforeReadyErr()
		case <-ctx.Done():
			return persistentStartupContextError(ctx)
		default:
			return nil
		}
	case ReadyAfterDelay:
		delay := time.NewTimer(cfg.ReadyDelay)
		defer delay.Stop()
		select {
		case <-delay.C:
			return nil
		case <-p.waitCh:
			return p.exitedBeforeReadyErr()
		case <-timeout.C:
			return p.readinessTimedOutErr()
		case <-ctx.Done():
			return persistentStartupContextError(ctx)
		}
	default: // ReadyOnOutput
		select {
		case <-readyCh:
			return nil
		case <-p.waitCh:
			return p.exitedBeforeReadyErr()
		case <-readersDone:
			// Output ended without the marker; give the exit path a brief moment to win.
			select {
			case <-readyCh:
				return nil
			case <-p.waitCh:
				return p.exitedBeforeReadyErr()
			case <-time.After(200 * time.Millisecond):
				return withStartErrorKind(
					goerrors.Internal(nil),
					PersistentStartOutputEndedBeforeReadiness,
				)
			}
		case <-timeout.C:
			return p.readinessTimedOutErr()
		case <-ctx.Done():
			return persistentStartupContextError(ctx)
		}
	}
}

func persistentStartupContextError(ctx context.Context) error {
	switch {
	case stderrors.Is(ctx.Err(), context.DeadlineExceeded):
		return goerrors.Timeout("persistent process startup").WithCause(ctx.Err())
	case stderrors.Is(ctx.Err(), context.Canceled):
		return goerrors.Canceled("persistent process startup").WithCause(ctx.Err())
	default:
		return nil
	}
}

func (p *PersistentProcess) exitedBeforeReadyErr() error {
	return withStartErrorKind(
		goerrors.Internal(fmt.Errorf("persistent process exited before readiness (exit code %d)", p.exitCode())),
		PersistentStartExitedBeforeReadiness,
	)
}

func (p *PersistentProcess) readinessTimedOutErr() error {
	return withStartErrorKind(
		goerrors.Timeout("persistent process readiness"),
		PersistentStartReadinessTimedOut,
	)
}

// Pid returns the process id, or -1 if the process is not running.
func (p *PersistentProcess) Pid() int {
	if p.cmd.Process == nil {
		return -1
	}
	return p.cmd.Process.Pid
}

// Wait blocks until the persistent process exits on its own and returns its result.
func (p *PersistentProcess) Wait() error {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return goerrors.Conflict("persistent process already stopped")
	}
	p.stopped = true
	p.mu.Unlock()

	<-p.waitCh
	return nil
}

// Shutdown gracefully stops the persistent process, escalating to a force kill after the
// grace period, and returns the completed result. It reports AlreadyExited when the process
// had already ended. The context bounds the graceful wait before escalation.
func (p *PersistentProcess) Shutdown(ctx context.Context) (ShutdownOutcome, error) {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return ShutdownOutcome{}, goerrors.Conflict("persistent process already stopped")
	}
	p.stopped = true
	p.mu.Unlock()

	select {
	case <-p.waitCh:
		return ShutdownOutcome{AlreadyExited: true, Result: p.result()}, nil
	default:
	}

	if p.policy.targetsGroup() {
		_ = TerminateGroup(p.cmd)
	} else {
		_ = p.cmd.Process.Kill()
	}

	timer := time.NewTimer(p.grace)
	defer timer.Stop()
	select {
	case <-p.waitCh:
		return ShutdownOutcome{Result: p.result()}, nil
	case <-timer.C:
	case <-ctx.Done():
	}

	if p.policy.KillAfterGrace || ctx.Err() != nil {
		_ = KillGroup(p.cmd)
	}
	<-p.waitCh
	return ShutdownOutcome{Result: p.result()}, nil
}

func (p *PersistentProcess) exitCode() int {
	if p.cmd.ProcessState == nil {
		return -1
	}
	return p.cmd.ProcessState.ExitCode()
}

func (p *PersistentProcess) result() *Result {
	stdout, stdoutTrunc := p.stdout.snapshot()
	stderr, stderrTrunc := p.stderr.snapshot()
	return &Result{
		Stdout:          stdout,
		StdoutTruncated: stdoutTrunc,
		Stderr:          stderr,
		StderrTruncated: stderrTrunc,
		ExitCode:        p.exitCode(),
		Duration:        time.Since(p.start),
	}
}
