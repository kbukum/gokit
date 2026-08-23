package process

import (
	"fmt"
	"time"

	goerrors "github.com/kbukum/gokit/errors"
)

// Result holds the output and status of a completed subprocess.
type Result struct {
	// Stdout is the captured standard output.
	Stdout []byte
	// StdoutTruncated reports whether stdout exceeded MaxOutputBytes.
	StdoutTruncated bool
	// Stderr is the captured standard error.
	Stderr []byte
	// StderrTruncated reports whether stderr exceeded MaxOutputBytes.
	StderrTruncated bool
	// ExitCode is the process exit code. -1 if the process was killed.
	ExitCode int
	// Duration is how long the process ran.
	Duration time.Duration
	// TimedOut reports whether the process was killed because the context deadline was exceeded.
	TimedOut bool
	// Canceled reports whether the process was killed because the context was canceled by the caller.
	Canceled bool
}

// Success reports whether the process exited cleanly with exit code 0.
func (r *Result) Success() bool {
	return r.ExitCode == 0
}

// Check verifies the process completed successfully and returns a typed error otherwise.
// Cancellation and timeout take precedence over the exit code, then a non-zero exit code
// (or a killed process, reported as exit code -1) yields an internal AppError describing
// the outcome. It returns nil when the process succeeded.
func (r *Result) Check() error {
	switch {
	case r.Canceled:
		return goerrors.Canceled("process")
	case r.TimedOut:
		return goerrors.Timeout("process")
	case r.ExitCode == 0:
		return nil
	case r.ExitCode == -1:
		return goerrors.Internal(fmt.Errorf("process killed (exit code -1)")).
			WithDetail("killed", true)
	default:
		return goerrors.Internal(fmt.Errorf("process exited with code %d", r.ExitCode)).
			WithDetail("exit_code", r.ExitCode)
	}
}
