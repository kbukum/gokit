package process

import (
	"context"
	stderrors "errors"
	"maps"
	"os"
	"os/exec"
	"slices"
	"sync"
	"time"

	goerrors "github.com/kbukum/gokit/errors"
)

// Supervisor tracks live child processes and tears them all down on Shutdown.
//
// A caller registers a started *exec.Cmd with Track (handing reaping to the supervisor)
// or a bare pid with TrackPid (best-effort, signal-only). Shutdown gracefully terminates
// every still-tracked child, waits the policy grace period, escalates to SIGKILL when
// enabled, and drains each to completion. It is safe for concurrent use and idempotent:
// double cleanup is a no-op. On non-Unix platforms a pid-only child that cannot be
// signaled yields an honest error from Shutdown rather than a silent success.
type Supervisor struct {
	policy LifecyclePolicy

	mu         sync.Mutex
	nextID     int
	children   map[int]*trackedChild
	shutdownMu sync.Mutex
	lastReport ShutdownReport
	lastErr    error
	outcomes   map[int]ChildOutcome
	errors     map[int]error
	retrying   bool
}

type trackedChild struct {
	cmd         *exec.Cmd // nil for pid-only tracking
	pid         int
	reaped      bool
	owned       *ownedChild
	outcome     ShutdownOutcome
	shutdownErr error
}

// ChildOutcome reports one child's shutdown. Reaped is false for signal-only TrackPid registrations.
type ChildOutcome struct {
	PID    int
	Reaped bool
	ShutdownOutcome
}

// ShutdownReport records the outcomes of a supervised teardown in registration order.
type ShutdownReport struct {
	Reason   string
	Children []ChildOutcome
}

// TrackHandle identifies a tracked child for explicit ownership transfer before cleanup.
type TrackHandle int

// NewSupervisor creates a Supervisor governed by the given lifecycle policy. A zero-value
// policy is replaced with DefaultLifecyclePolicy.
func NewSupervisor(policy LifecyclePolicy) *Supervisor {
	if policy == (LifecyclePolicy{}) {
		policy = DefaultLifecyclePolicy()
	}
	if policy.GracePeriod <= 0 {
		policy.GracePeriod = DefaultGracePeriod
	}
	return &Supervisor{policy: policy, children: make(map[int]*trackedChild)}
}

// Track registers a started command for supervised shutdown and hands its reaping to the
// supervisor. The caller must successfully Release before calling cmd.Wait itself.
// Track returns a zero handle when cmd has not started or has already been reaped.
func (s *Supervisor) Track(cmd *exec.Cmd) TrackHandle {
	if cmd == nil || cmd.Process == nil || cmd.ProcessState != nil {
		return 0
	}
	return s.add(&trackedChild{cmd: cmd, pid: cmd.Process.Pid})
}

// TrackPid registers a bare pid for best-effort supervised shutdown. The supervisor can
// signal the pid but cannot reap it, so the owner remains responsible for wait/reap.
func (s *Supervisor) TrackPid(pid int) TrackHandle {
	if pid <= 0 {
		return 0
	}
	return s.add(&trackedChild{pid: pid})
}

func (s *Supervisor) add(child *trackedChild) TrackHandle {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, registered := range s.children {
		if child.cmd != nil && registered.cmd == child.cmd {
			return TrackHandle(id)
		}
	}
	s.nextID++
	id := s.nextID
	s.children[id] = child
	return TrackHandle(id)
}

// Release transfers a tracked child's ownership back to the caller before cleanup starts.
// Once cleanup owns the command, Release returns an error rather than abandoning it.
// It is a no-op for an unknown or zero handle.
func (s *Supervisor) Release(handle TrackHandle) error {
	s.shutdownMu.Lock()
	defer s.shutdownMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if child := s.children[int(handle)]; child != nil && child.owned != nil {
		return goerrors.InvalidInput("handle", "cleanup owns this command; retry Shutdown before releasing")
	}
	delete(s.children, int(handle))
	return nil
}

// Len reports the number of children currently tracked.
func (s *Supervisor) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.children)
}

// Shutdown terminates every still-tracked child and drains each to completion. It is
// idempotent and returns each child's outcome plus joined failures, including forced termination. The context bounds graceful waiting; remaining owned children are then force-killed and reaped.
func (s *Supervisor) Shutdown(ctx context.Context, reason string) (ShutdownReport, error) {
	s.shutdownMu.Lock()
	defer s.shutdownMu.Unlock()
	s.mu.Lock()
	children := maps.Clone(s.children)
	s.mu.Unlock()

	if len(children) == 0 {
		return s.lastReport, s.lastErr
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ids := make([]int, 0, len(children))
	for id := range children {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	if !s.retrying {
		s.outcomes = make(map[int]ChildOutcome)
		s.errors = make(map[int]error)
	} else {
		reason = s.lastReport.Reason
	}
	results := make([]ChildOutcome, len(ids))
	childErrors := make([]error, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		child := children[id]
		wg.Add(1)
		go func() {
			defer wg.Done()
			outcome, err := s.terminate(ctx, child)
			results[i] = ChildOutcome{PID: child.pid, Reaped: child.reaped, ShutdownOutcome: outcome}
			childErrors[i] = err
			if child.cmd == nil || outcome.Complete {
				s.mu.Lock()
				delete(s.children, id)
				s.mu.Unlock()
			}
		}()
	}
	wg.Wait()
	for i, id := range ids {
		s.outcomes[id], s.errors[id] = results[i], childErrors[i]
	}
	ids = ids[:0]
	for id := range s.outcomes {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	report := ShutdownReport{Reason: reason, Children: make([]ChildOutcome, len(ids))}
	errs := make([]error, len(ids))
	for i, id := range ids {
		report.Children[i], errs[i] = s.outcomes[id], s.errors[id]
	}
	s.lastReport, s.lastErr = report, stderrors.Join(errs...)
	s.mu.Lock()
	s.retrying = len(s.children) != 0
	s.mu.Unlock()
	return s.lastReport, s.lastErr
}

// terminate signals one child, waits the grace period, and escalates to a force kill.
func (s *Supervisor) terminate(ctx context.Context, child *trackedChild) (ShutdownOutcome, error) {
	group := s.policy.targetsGroup()
	grace := s.policy.grace()

	if child.cmd != nil {
		if child.owned == nil {
			child.owned = newOwnedChild(child.cmd)
		}
		outcome, err := shutdownOwned(ctx, child.owned, s.policy, grace)
		preserveShutdownFlags(child.outcome.Result, outcome.Result)
		err = stderrors.Join(child.shutdownErr, err)
		select {
		case <-child.owned.done:
			child.reaped = true
			err = stderrors.Join(err, unexpectedWaitError(child.owned.waitErr))
		default:
		}
		child.outcome, child.shutdownErr = outcome, err
		return outcome, err
	}
	return s.terminatePid(ctx, child.pid, group, grace)
}

// terminatePid tears down a pid-only child by signaling and polling liveness.
func (s *Supervisor) terminatePid(ctx context.Context, pid int, group bool, grace time.Duration) (ShutdownOutcome, error) {
	outcome := ShutdownOutcome{Result: &Result{}}
	if err := terminatePIDGroup(pid, group); err != nil {
		if isExited(err) {
			outcome.AlreadyExited = true
			return outcome, nil
		}
		return outcome, err
	}

	deadline := time.NewTimer(grace)
	defer deadline.Stop()
	poll := time.NewTicker(5 * time.Millisecond)
	defer poll.Stop()

	for {
		select {
		case <-poll.C:
			if !pidGroupAlive(pid, group) {
				return outcome, nil
			}
		case <-deadline.C:
			if s.policy.KillAfterGrace {
				outcome.Result.Forced = true
				return outcome, stderrors.Join(killPIDGroup(pid, group), outcome.Result.Check())
			}
		case <-ctx.Done():
			outcome.Result.Forced = true
			outcome.Result.Canceled = stderrors.Is(ctx.Err(), context.Canceled)
			outcome.Result.TimedOut = stderrors.Is(ctx.Err(), context.DeadlineExceeded)
			return outcome, stderrors.Join(killPIDGroup(pid, group), ctx.Err())
		}
	}
}

// isExited reports whether a signal/kill error indicates the process was already gone.
// A kill on a finished process reports os.ErrProcessDone; a group signal to a reaped
// process reports a platform "no such process" errno (see signalErrIsGone). Neither is a
// real teardown failure.
func isExited(err error) bool {
	return err != nil && (stderrors.Is(err, os.ErrProcessDone) || signalErrIsGone(err))
}
