package component

import (
	"errors"
	"fmt"
)

// StartError reports a failed StartAll or StartAllConcurrent call. Cause is the start failure; Rollback joins failures from stopping the components started in the same call, including cleanup of a component whose Start timed out. Rollback is nil when every rollback stop succeeded. Both remain reachable through errors.Is and errors.As.
type StartError struct {
	// Component names the component whose Start failed. It is empty when startup was abandoned because the context ended before a component failed.
	Component string
	Cause     error
	Rollback  error
}

func (e *StartError) Error() string {
	msg := e.Cause.Error()
	if e.Component != "" {
		msg = fmt.Sprintf("failed to start %s: %v", e.Component, e.Cause)
	}
	if e.Rollback != nil {
		msg += fmt.Sprintf("; rollback: %v", e.Rollback)
	}
	return msg
}

// Unwrap returns the start cause and, when present, the rollback failure.
func (e *StartError) Unwrap() []error {
	if e.Rollback == nil {
		return []error{e.Cause}
	}
	return []error{e.Cause, e.Rollback}
}

func newStartError(name string, cause error, rollback ...error) *StartError {
	return &StartError{Component: name, Cause: cause, Rollback: errors.Join(rollback...)}
}
