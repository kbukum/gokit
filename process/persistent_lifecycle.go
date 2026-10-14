package process

import (
	"context"
	"errors"
	"os"

	goerrors "github.com/kbukum/gokit/errors"
)

// Pid returns the owned process ID. It remains available after exit for diagnostics, not as permission to signal that PID again.
func (p *PersistentProcess) Pid() int { return p.owned.pid }

// Signal targets only this retained, unreaped child, never a historical diagnostic PID.
// It does not signal descendants; Shutdown owns process-group teardown.
func (p *PersistentProcess) Signal(signal os.Signal) error {
	p.shutdownMu.Lock()
	defer p.shutdownMu.Unlock()
	if signal == nil {
		return goerrors.InvalidInput("signal", "A signal is required")
	}
	if p.outcome.Complete || p.owned.waitStarted {
		return goerrors.InvalidInput("process", "Process ownership has been released")
	}
	exited, err := p.owned.retryObservation()
	if err != nil {
		return goerrors.Internal(err)
	}
	if exited {
		return goerrors.InvalidInput("process", "Process has exited")
	}
	if err := p.cmd.Process.Signal(signal); err != nil {
		return goerrors.Internal(err)
	}
	return nil
}

// Wait observes a persistent process without taking away Shutdown ownership. Canceling this wait does not stop the process; its owner must still call Shutdown.
func (p *PersistentProcess) Wait(ctx context.Context) (*Result, error) {
	result, err := p.WaitExit(ctx)
	if result != nil {
		err = errors.Join(err, result.Check())
	}
	return result, err
}

// WaitExit observes and reaps an exited child, returning its status without
// interpreting a nonzero exit or deliberate crash as an assertion failure.
// Ownership, capture and unexpected forced-cleanup errors remain failures.
// Canceling the wait never stops a live child.
func (p *PersistentProcess) WaitExit(ctx context.Context) (*Result, error) {
	select {
	case <-p.waitCh:
		outcome, err := p.shutdown(ctx)
		return completedExit(outcome, err)
	default:
	}
	select {
	case <-p.owned.observed:
		if err := p.owned.observationError(); err != nil {
			return nil, err
		}
		outcome, err := p.shutdown(ctx)
		return completedExit(outcome, err)
	case <-ctx.Done():
		return nil, persistentStartupContextError(ctx)
	}
}

func completedExit(outcome ShutdownOutcome, err error) (*Result, error) {
	if !outcome.Complete {
		return nil, errors.Join(err, goerrors.Internal(errors.New("owned process cleanup is incomplete")))
	}
	return outcome.Result, err
}

// Shutdown gracefully stops and reaps the persistent process. Deadline/cancellation or grace-period escalation returns a forced result and an error. Repeated calls return the recorded outcome.
func (p *PersistentProcess) Shutdown(ctx context.Context) (ShutdownOutcome, error) {
	outcome, err := p.shutdown(ctx)
	return outcome, errors.Join(err, shutdownStatusError(outcome))
}

func (p *PersistentProcess) shutdown(ctx context.Context) (ShutdownOutcome, error) {
	p.shutdownMu.Lock()
	defer p.shutdownMu.Unlock()
	if p.outcome.Complete {
		return p.outcome, p.shutdownErr
	}
	previous, previousErr := p.outcome, p.shutdownErr
	p.outcome, p.shutdownErr = settleOwned(ctx, p.owned, p.policy, p.grace)
	preserveShutdownFlags(previous.Result, p.outcome.Result)
	p.shutdownErr = errors.Join(previousErr, p.shutdownErr)
	select {
	case <-p.waitCh:
		result := p.result()
		result.Forced = p.outcome.Result.Forced
		result.Canceled = p.outcome.Result.Canceled
		result.TimedOut = p.outcome.Result.TimedOut
		p.outcome.Result = result
		p.shutdownErr = errors.Join(p.shutdownErr, unexpectedWaitError(p.owned.waitErr), p.stdout.error(), p.stderr.error())
	default:
	}
	return p.outcome, p.shutdownErr
}
