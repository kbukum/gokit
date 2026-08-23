package process

import "time"

// DefaultGracePeriod is how long Run, Stream, and supervised shutdown wait after graceful
// termination before escalating to SIGKILL.
const DefaultGracePeriod = 5 * time.Second

// LifecyclePolicy governs a spawned child's isolation and shutdown escalation.
//
// When IsolateProcessGroup is set the child is placed in its own process group so
// termination can target the whole group. When TerminateDescendants is set the group
// (not just the immediate child) is signaled. KillAfterGrace escalates to SIGKILL once
// GracePeriod elapses.
type LifecyclePolicy struct {
	// GracePeriod is how long to wait after graceful termination before kill escalation.
	GracePeriod time.Duration
	// IsolateProcessGroup places the child in a new process group where supported.
	IsolateProcessGroup bool
	// TerminateDescendants targets the whole process group rather than only the immediate child.
	TerminateDescendants bool
	// KillAfterGrace escalates to a force kill after GracePeriod expires. When false, no
	// escalation occurs: Run/Stream leave WaitDelay unset, so a child that ignores graceful
	// termination can block the call until it exits on its own or the context is canceled.
	KillAfterGrace bool
}

// DefaultLifecyclePolicy returns the standard policy: a 5s grace period, process-group
// isolation, descendant termination, and kill-after-grace escalation all enabled.
func DefaultLifecyclePolicy() LifecyclePolicy {
	return LifecyclePolicy{
		GracePeriod:          DefaultGracePeriod,
		IsolateProcessGroup:  true,
		TerminateDescendants: true,
		KillAfterGrace:       true,
	}
}

// targetsGroup reports whether termination should signal the process group.
func (p LifecyclePolicy) targetsGroup() bool {
	return p.IsolateProcessGroup && p.TerminateDescendants
}

// grace returns the effective grace period, falling back to the default when unset.
func (p LifecyclePolicy) grace() time.Duration {
	if p.GracePeriod <= 0 {
		return DefaultGracePeriod
	}
	return p.GracePeriod
}

// resolveLifecycle derives the effective policy for a Command, honoring an explicit
// Lifecycle and letting a non-zero GracePeriod override the policy's grace period.
func resolveLifecycle(cmd Command) LifecyclePolicy {
	policy := DefaultLifecyclePolicy()
	if cmd.Lifecycle != nil {
		policy = *cmd.Lifecycle
	}
	if cmd.GracePeriod > 0 {
		policy.GracePeriod = cmd.GracePeriod
	}
	if policy.GracePeriod <= 0 {
		policy.GracePeriod = DefaultGracePeriod
	}
	return policy
}
