package bootstrap

import (
	"errors"
	"fmt"
)

// ErrLifecycleUsed is returned when Run, RunTask, or Startup is called on an App whose single lifecycle has already started or shut down. Create a new App for each lifecycle.
var ErrLifecycleUsed = errors.New("bootstrap: application lifecycle already used")

// ErrShutdownRequested is the cause of a startup that Shutdown interrupted. Shutdown cancels the startup context with this cause, and startup rolls back after the running phase returns.
var ErrShutdownRequested = errors.New("bootstrap: shutdown requested during startup")

// Phase names the startup phase that failed.
type Phase string

const (
	PhaseConfigure   Phase = "configure"
	PhaseBeforeStart Phase = "before_start"
	PhaseStart       Phase = "start"
	PhaseAfterStart  Phase = "after_start"
	PhaseReady       Phase = "ready"
)

// StartupError reports a failed startup phase. Cause is the phase failure. Rollback is the *ShutdownError from tearing down what earlier phases created, or nil when teardown succeeded. A component start failure arrives as a *component.StartError Cause, which carries its own registry rollback outcome.
type StartupError struct {
	Phase    Phase
	Cause    error
	Rollback error
}

func (e *StartupError) Error() string {
	msg := fmt.Sprintf("bootstrap: startup failed in %s phase: %v", e.Phase, e.Cause)
	if e.Rollback != nil {
		msg += fmt.Sprintf("; rollback: %v", e.Rollback)
	}
	return msg
}

// Unwrap returns the phase cause and, when present, the rollback failure.
func (e *StartupError) Unwrap() []error { return causes(e.Cause, e.Rollback) }

// TaskError reports a failed RunTask task. Cause is the task's error. Shutdown is the *ShutdownError from the teardown that followed, or nil when teardown succeeded.
type TaskError struct {
	Cause    error
	Shutdown error
}

func (e *TaskError) Error() string {
	msg := fmt.Sprintf("bootstrap: task failed: %v", e.Cause)
	if e.Shutdown != nil {
		msg += fmt.Sprintf("; %v", e.Shutdown)
	}
	return msg
}

// Unwrap returns the task cause and, when present, the shutdown failure.
func (e *TaskError) Unwrap() []error { return causes(e.Cause, e.Shutdown) }

// ShutdownError reports failures from quiescing, stop hooks, draining, releasing components and container resources, or releasing the App-owned logger.
type ShutdownError struct {
	Cause error
}

func (e *ShutdownError) Error() string { return fmt.Sprintf("bootstrap: shutdown failed: %v", e.Cause) }

// Unwrap returns the joined teardown failures.
func (e *ShutdownError) Unwrap() error { return e.Cause }

func causes(cause, cleanup error) []error {
	if cleanup == nil {
		return []error{cause}
	}
	return []error{cause, cleanup}
}
