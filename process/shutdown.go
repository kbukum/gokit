package process

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"

	goerrors "github.com/kbukum/gokit/errors"
)

// shutdownOwned retains ownership until both Wait/output collection and the isolated process group complete.
func shutdownOwned(ctx context.Context, child *ownedChild, policy LifecyclePolicy, grace time.Duration) (result ShutdownOutcome, resultErr error) {
	var probeErr error
	defer func() { resultErr = errors.Join(resultErr, probeErr, child.observationError()) }()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	outcome := ShutdownOutcome{Result: &Result{}}
	finish := func() error {
		select {
		case <-child.done:
			outcome.Result.ExitCode = exitCodeOf(child.cmd.ProcessState)
			outcome.Result.Forced = outcome.Result.Forced || wasForced(child.cmd.ProcessState)
		case <-child.exited:
			outcome.Result.ExitCode = child.exit.code
			outcome.Result.Forced = outcome.Result.Forced || child.exit.forced
		default:
		}
		if outcome.AlreadyExited || outcome.Result.Forced ||
			(outcome.Result.ExitCode != nil && *outcome.Result.ExitCode != 0) {
			return outcome.Result.Check()
		}
		return nil
	}
	select {
	case <-child.done:
		outcome.AlreadyExited = true
		outcome.Complete = true
		return outcome, finish()
	default:
	}
	select {
	case <-child.exited:
		outcome.AlreadyExited = true
	default:
	}
	confirmed, inspectionErr := child.retryObservation()
	probeErr = inspectionErr
	if confirmed {
		outcome.AlreadyExited = true
	}
	if child.groupReleased {
		waitErr := waitOwnedReap(ctx, child)
		outcome.Complete = waitErr == nil
		return outcome, errors.Join(waitErr, finish())
	}
	if outcome.AlreadyExited {
		reapCtx, reapCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer reapCancel()
		descendants := false
		if policy.targetsGroup() {
			var err error
			descendants, err = groupHasDescendants(reapCtx, child.pid)
			if err != nil {
				return outcome, err
			}
		}
		if !descendants {
			child.startWait(reapCtx)
			waitErr := waitOwnedReap(reapCtx, child)
			outcome.Complete = waitErr == nil
			return outcome, errors.Join(waitErr, finish())
		}
	}

	var signalErr error
	if gracefulTerminationSupported {
		if policy.targetsGroup() {
			signalErr = terminatePIDGroup(child.pid, true)
		} else {
			signalErr = child.cmd.Process.Signal(terminationSignal)
		}
	} else {
		outcome.Result.Forced = true
		signalErr = child.cmd.Process.Kill()
	}
	if isExited(signalErr) {
		signalErr = nil
	}
	graceCtx := ctx
	graceCancel := func() {}
	if policy.KillAfterGrace || signalErr != nil {
		graceCtx, graceCancel = context.WithTimeout(ctx, grace)
	}
	waitErr := waitOwnedExit(graceCtx, child, policy.targetsGroup())
	graceCancel()
	if waitErr == nil {
		reapCtx, reapCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer reapCancel()
		child.startWait(reapCtx)
		waitErr = waitOwnedReap(reapCtx, child)
		outcome.Complete = waitErr == nil
		return outcome, errors.Join(signalErr, waitErr, finish())
	}
	if !errors.Is(waitErr, context.Canceled) && !errors.Is(waitErr, context.DeadlineExceeded) {
		return outcome, errors.Join(signalErr, waitErr, finish())
	}
	reapCtx, reapCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer reapCancel()
	confirmed, inspectionErr = child.retryObservation()
	probeErr = errors.Join(probeErr, inspectionErr)
	if confirmed {
		descendants := false
		if policy.targetsGroup() {
			var err error
			descendants, err = groupHasDescendants(reapCtx, child.pid)
			if err != nil {
				return outcome, errors.Join(signalErr, err, finish())
			}
		}
		if !descendants {
			child.startWait(reapCtx)
			waitErr = waitOwnedReap(reapCtx, child)
			outcome.Complete = waitErr == nil
			outcome.Result.Canceled = errors.Is(ctx.Err(), context.Canceled)
			outcome.Result.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
			return outcome, errors.Join(waitErr, ctx.Err(), finish())
		}
	}

	outcome.Result.Forced = true
	outcome.Result.Canceled = errors.Is(ctx.Err(), context.Canceled)
	outcome.Result.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
	var killErr error
	if policy.targetsGroup() {
		killErr = killPIDGroup(child.pid, true)
	}
	if isExited(killErr) {
		killErr = nil
	}
	directKillErr := child.cmd.Process.Kill()
	if isExited(directKillErr) {
		directKillErr = nil
	}
	if directKillErr != nil {
		_, exited, inspectErr := inspectChildExit(child.cmd)
		if inspectErr == nil && exited {
			directKillErr = nil
		} else {
			directKillErr = errors.Join(directKillErr, inspectErr)
		}
	}
	killErr = errors.Join(killErr, directKillErr)
	if killErr != nil {
		return outcome, errors.Join(signalErr, fmt.Errorf("force termination failed: %w", killErr), ctx.Err())
	}
	waitErr = waitOwnedExit(reapCtx, child, policy.targetsGroup())
	if waitErr == nil {
		child.startWait(reapCtx)
		waitErr = waitOwnedReap(reapCtx, child)
	}
	outcome.Complete = waitErr == nil
	outcome.Result.TimedOut = outcome.Result.TimedOut || errors.Is(waitErr, context.DeadlineExceeded)
	var exitErr error
	select {
	case <-child.done:
		exitErr = finish()
	default:
	}
	return outcome, errors.Join(signalErr, exitErr, waitErr,
		goerrors.Internal(fmt.Errorf("process required forced termination")).WithDetail("forced", true), ctx.Err())
}

func preserveShutdownFlags(previous, result *Result) {
	if previous != nil {
		result.Forced = result.Forced || previous.Forced
		result.Canceled = result.Canceled || previous.Canceled
		result.TimedOut = result.TimedOut || previous.TimedOut
	}
}

func unexpectedWaitError(err error) error {
	var exitErr *exec.ExitError
	if err == nil || errors.As(err, &exitErr) {
		return nil
	}
	return goerrors.Internal(err)
}
