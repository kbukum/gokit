package process

import (
	"context"
	stderrors "errors"
	"os"
	"os/exec"
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

	mu       sync.Mutex
	nextID   int
	children map[int]*trackedChild
}

type trackedChild struct {
	cmd *exec.Cmd // nil for pid-only tracking
	pid int
}

// TrackHandle identifies a tracked child so it can be released after normal completion.
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
// supervisor. The returned handle can be passed to Release once the caller has observed
// the child exit on its own. Track returns a zero handle when cmd has not started.
func (s *Supervisor) Track(cmd *exec.Cmd) TrackHandle {
	if cmd == nil || cmd.Process == nil {
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
	s.nextID++
	id := s.nextID
	s.children[id] = child
	return TrackHandle(id)
}

// Release removes a tracked child from supervision, for use when the caller has already
// reaped it. It is a no-op for an unknown or zero handle.
func (s *Supervisor) Release(handle TrackHandle) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.children, int(handle))
}

// Len reports the number of children currently tracked.
func (s *Supervisor) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.children)
}

// Shutdown terminates every still-tracked child and drains each to completion. It is
// idempotent and returns the joined error of any child that could not be torn down. The
// context bounds the whole operation; when it is done, remaining children are force-killed.
func (s *Supervisor) Shutdown(ctx context.Context, reason string) error {
	s.mu.Lock()
	children := s.children
	s.children = make(map[int]*trackedChild)
	s.mu.Unlock()

	if len(children) == 0 {
		return nil
	}

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	for _, child := range children {
		wg.Add(1)
		go func(child *trackedChild) {
			defer wg.Done()
			if err := s.terminate(ctx, child); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(child)
	}
	wg.Wait()

	if len(errs) > 0 {
		return goerrors.Internal(stderrors.Join(errs...))
	}
	return nil
}

// terminate signals one child, waits the grace period, and escalates to a force kill.
func (s *Supervisor) terminate(ctx context.Context, child *trackedChild) error {
	group := s.policy.targetsGroup()
	grace := s.policy.grace()

	if child.cmd != nil {
		return s.terminateOwned(ctx, child.cmd, group, grace)
	}
	return s.terminatePid(ctx, child.pid, group, grace)
}

// terminateOwned tears down a child the supervisor owns and reaps it via Wait.
func (s *Supervisor) terminateOwned(ctx context.Context, cmd *exec.Cmd, group bool, grace time.Duration) error {
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	signal := TerminateGroup
	if !group {
		signal = func(c *exec.Cmd) error { return c.Process.Kill() }
	}
	if err := signal(cmd); err != nil && !isExited(err) {
		_ = KillGroup(cmd) // fall back to a hard kill when graceful signaling fails
	}

	timer := time.NewTimer(grace)
	defer timer.Stop()

	select {
	case <-done:
		return nil
	case <-timer.C:
	case <-ctx.Done():
	}

	if s.policy.KillAfterGrace || ctx.Err() != nil {
		_ = KillGroup(cmd)
	}
	<-done
	return nil
}

// terminatePid tears down a pid-only child by signaling and polling liveness.
func (s *Supervisor) terminatePid(ctx context.Context, pid int, group bool, grace time.Duration) error {
	if err := terminatePIDGroup(pid, group); err != nil {
		return err
	}

	deadline := time.NewTimer(grace)
	defer deadline.Stop()
	poll := time.NewTicker(5 * time.Millisecond)
	defer poll.Stop()

	for {
		select {
		case <-poll.C:
			if !pidGroupAlive(pid, group) {
				return nil
			}
		case <-deadline.C:
			if s.policy.KillAfterGrace {
				_ = killPIDGroup(pid, group)
			}
			return nil
		case <-ctx.Done():
			_ = killPIDGroup(pid, group)
			return nil
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
