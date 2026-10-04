package process

import (
	"context"
	"errors"
)

// Pid returns the owned process ID. It remains available after exit for diagnostics, not as permission to signal that PID again.
func (p *PersistentProcess) Pid() int { return p.owned.pid }

// Wait observes a persistent process without taking away Shutdown ownership. Canceling this wait does not stop the process; its owner must still call Shutdown.
func (p *PersistentProcess) Wait(ctx context.Context) (*Result, error) {
	select {
	case <-p.waitCh:
		outcome, err := p.Shutdown(ctx)
		return outcome.Result, errors.Join(err, outcome.Result.Check())
	default:
	}
	select {
	case <-p.owned.observed:
		if err := p.owned.observationError(); err != nil {
			return nil, err
		}
		outcome, err := p.Shutdown(ctx)
		return outcome.Result, errors.Join(err, outcome.Result.Check())
	case <-ctx.Done():
		return nil, persistentStartupContextError(ctx)
	}
}

// Shutdown gracefully stops and reaps the persistent process. Deadline/cancellation or grace-period escalation returns a forced result and an error. Repeated calls return the recorded outcome.
func (p *PersistentProcess) Shutdown(ctx context.Context) (ShutdownOutcome, error) {
	p.shutdownMu.Lock()
	defer p.shutdownMu.Unlock()
	if p.outcome.Complete {
		return p.outcome, p.shutdownErr
	}
	previous, previousErr := p.outcome, p.shutdownErr
	p.outcome, p.shutdownErr = shutdownOwned(ctx, p.owned, p.policy, p.grace)
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
