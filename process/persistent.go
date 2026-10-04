package process

import (
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
	// MaxCaptureBytes bounds retained lifetime output per stream. Nonpositive values use Command.MaxOutputBytes, or 64 KiB if neither limit is positive.
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
		MaxCaptureBytes:     64 * 1024,
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
	// Complete confirms owned group release and completed Wait/output collection. Signal-only registrations cannot establish this.
	Complete bool
}

// PersistentProcess is a running long-lived subprocess with graceful shutdown.
type PersistentProcess struct {
	cmd    *exec.Cmd
	owned  *ownedChild
	policy LifecyclePolicy
	grace  time.Duration
	start  time.Time

	stdout *guardedBuffer
	stderr *guardedBuffer

	waitCh chan struct{} // closed when cmd.Wait returns

	outcome     ShutdownOutcome
	shutdownErr error
	shutdownMu  sync.Mutex
}

// StartPersistent spawns a long-lived subprocess and waits for it to become ready per cfg.
// On success it returns the startup output snapshot and a handle for waiting or shutting
// the process down. On failure it attempts cleanup and returns a classified AppError
// whose kind is retrievable via StartErrorKind. Incomplete cleanup also returns a non-nil
// owning run; callers must retain it and retry Shutdown until its outcome is Complete.
func StartPersistent(ctx context.Context, cmd Command, cfg PersistentConfig) (*PersistentRun, error) {
	return startPersistent(ctx, cmd, cfg, newOwnedChild)
}

func startPersistent(ctx context.Context, cmd Command, cfg PersistentConfig, acquire func(*exec.Cmd) *ownedChild) (*PersistentRun, error) {
	if cmd.Binary == "" {
		return nil, goerrors.MissingField("binary")
	}
	cfg = cfg.normalized()
	if !childObservationSupported {
		return nil, goerrors.InvalidInput("platform", "owned persistent processes require Linux, Darwin, or Windows")
	}
	if cfg.MaxCaptureBytes <= 0 {
		cfg.MaxCaptureBytes = cmd.MaxOutputBytes
		if cfg.MaxCaptureBytes <= 0 {
			cfg.MaxCaptureBytes = 64 * 1024
		}
	}
	if cfg.Readiness == ReadyOnOutput && cfg.OutputMarker == "" {
		return nil, goerrors.InvalidInput("readiness.output_marker", "output readiness marker must not be empty")
	}
	if err := persistentStartupContextError(ctx); err != nil {
		return nil, err
	}

	// The persistent lifecycle is owned by the returned handle (Wait/Shutdown), so the
	// spawn context is detached from cancellation; readiness still honors ctx below.
	// A fresh command is built on every start attempt: an ETXTBSY retry cannot reuse a
	// Cmd whose Start already failed, so the pipes are recreated per attempt and captured
	// once a start succeeds.
	spawnCtx := context.WithoutCancel(ctx)
	var (
		c                      *exec.Cmd
		stdoutPipe, stderrPipe io.ReadCloser
		pipeErr                error
	)
	startErr := startWithETXTBSYRetry(func() error {
		c = exec.CommandContext(spawnCtx, cmd.Binary, cmd.Args...) //nolint:gosec // dynamic args are the purpose of this package
		c.Dir = cmd.Dir
		c.Env = mergeEnv(cmd.Env, cmd.EnvPolicy)
		applyInput(c, cmd)
		if cfg.Lifecycle.IsolateProcessGroup {
			configureSysProcAttr(c)
		}
		if stdoutPipe, pipeErr = c.StdoutPipe(); pipeErr != nil {
			return pipeErr
		}
		if stderrPipe, pipeErr = c.StderrPipe(); pipeErr != nil {
			return pipeErr
		}
		return c.Start()
	})
	if pipeErr != nil {
		return nil, fmt.Errorf("process: pipe: %w", pipeErr)
	}
	if startErr != nil {
		return nil, withStartErrorKind(
			goerrors.Wrap(SpawnError(fmt.Sprintf("process: start %s", cmd.Binary), startErr)),
			PersistentStartSpawnFailed,
		)
	}

	p := &PersistentProcess{
		cmd:    c,
		policy: cfg.Lifecycle,
		grace:  cfg.ShutdownGracePeriod,
		stdout: newGuardedBuffer(cfg.MaxCaptureBytes),
		stderr: newGuardedBuffer(cfg.MaxCaptureBytes),
	}
	p.owned = acquire(c)
	p.waitCh = p.owned.done
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
	}()
	p.owned.readersDone = readersDone
	p.owned.closeReaders = func() error {
		return stderrors.Join(stdoutPipe.Close(), stderrPipe.Close())
	}

	if err := p.awaitReady(ctx, cfg, readyCh, readersDone); err != nil {
		p.outcome, p.shutdownErr = shutdownOwned(context.WithoutCancel(ctx), p.owned, cfg.Lifecycle, cfg.ShutdownGracePeriod)
		shutdownErr := p.shutdownErr
		if p.outcome.Complete {
			shutdownErr = stderrors.Join(shutdownErr, unexpectedWaitError(p.owned.waitErr), p.stdout.error(), p.stderr.error())
		} else {
			return &PersistentRun{Process: p}, stderrors.Join(err, shutdownErr)
		}
		return nil, stderrors.Join(err, shutdownErr)
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

// awaitReady blocks until the configured readiness condition, a terminal state, or timeout.
func (p *PersistentProcess) awaitReady(ctx context.Context, cfg PersistentConfig, readyCh, readersDone chan struct{}) error {
	if err := p.owned.observationError(); err != nil {
		return withStartErrorKind(goerrors.Internal(err), PersistentStartObservationFailed)
	}
	timeout := time.NewTimer(cfg.ReadinessTimeout)
	defer timeout.Stop()

	switch cfg.Readiness {
	case ReadyImmediate:
		// Readiness is defined as a successful spawn: the process started, so it is
		// ready. Peeking at waitCh here to catch an early exit would be
		// scheduler-dependent — the waiter closes waitCh asynchronously, so the same
		// short-lived child could observe either the exit or success on this path.
		// Observing an early exit deterministically is exactly what ReadyAfterDelay
		// (a stabilization window) and ReadyOnOutput (a startup handshake) provide;
		// ReadyImmediate promises only that the spawn succeeded.
		return nil
	case ReadyAfterDelay:
		delay := time.NewTimer(cfg.ReadyDelay)
		defer delay.Stop()
		select {
		case <-delay.C:
			return nil
		case <-p.owned.observed:
			return p.observedBeforeReadyErr()
		case <-timeout.C:
			return p.readinessTimedOutErr()
		case <-ctx.Done():
			return persistentStartupContextError(ctx)
		}
	default: // ReadyOnOutput
		select {
		case <-readyCh:
			return nil
		case <-p.owned.observed:
			if err := p.owned.observationError(); err != nil {
				return p.observedBeforeReadyErr()
			}
			select {
			case <-readyCh:
				return nil
			case <-readersDone:
				select {
				case <-readyCh:
					return nil
				default:
					return p.exitedBeforeReadyErr()
				}
			case <-timeout.C:
				return p.readinessTimedOutErr()
			case <-ctx.Done():
				return persistentStartupContextError(ctx)
			}
		case <-readersDone:
			// Output ended without the marker; give the exit path a brief moment to win.
			select {
			case <-readyCh:
				return nil
			default:
			}
			select {
			case <-readyCh:
				return nil
			case <-p.owned.observed:
				select {
				case <-readyCh:
					return nil
				default:
				}
				return p.observedBeforeReadyErr()
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
		goerrors.Internal(fmt.Errorf("persistent process exited before readiness (exit code %s)", exitCodeLabel(p.exitCode()))),
		PersistentStartExitedBeforeReadiness,
	)
}

func (p *PersistentProcess) observedBeforeReadyErr() error {
	if err := p.owned.observationError(); err != nil {
		return withStartErrorKind(goerrors.Internal(err), PersistentStartObservationFailed)
	}
	return p.exitedBeforeReadyErr()
}

func (p *PersistentProcess) readinessTimedOutErr() error {
	return withStartErrorKind(
		goerrors.Timeout("persistent process readiness"),
		PersistentStartReadinessTimedOut,
	)
}
