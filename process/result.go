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
	// ExitCode is the process exit code, or nil if the process was killed by a signal or
	// otherwise never produced an exit status (an explicit unknown-exit representation).
	ExitCode *int
	// Duration is how long the process ran.
	Duration time.Duration
	// TimedOut reports whether the process was killed because the context deadline was exceeded.
	TimedOut bool
	// Canceled reports whether the process was killed because the context was canceled by the caller.
	Canceled bool
}

// StdoutString returns the captured standard output as a string. The conversion is lossless:
// the exact bytes are preserved without replacement.
func (r *Result) StdoutString() string { return string(r.Stdout) }

// StderrString returns the captured standard error as a string. The conversion is lossless:
// the exact bytes are preserved without replacement.
func (r *Result) StderrString() string { return string(r.Stderr) }

// Success reports whether the process exited cleanly with exit code 0.
func (r *Result) Success() bool {
	return r.ExitCode != nil && *r.ExitCode == 0
}

// ExitCodeOr returns the process exit code, or fallback when the process was killed and
// produced no exit status (a nil receiver or nil ExitCode). It is the single accessor for
// the "exit code or sentinel" idiom used by callers that need a plain int.
func (r *Result) ExitCodeOr(fallback int) int {
	if r == nil || r.ExitCode == nil {
		return fallback
	}
	return *r.ExitCode
}

// Check verifies the process completed successfully and returns a typed error otherwise.
// Cancellation and timeout take precedence over the exit code; then a killed process (no
// exit status) or a non-zero exit code yields an internal AppError describing the outcome.
// It returns nil when the process succeeded.
func (r *Result) Check() error {
	switch {
	case r.Canceled:
		return goerrors.Canceled("process")
	case r.TimedOut:
		return goerrors.Timeout("process")
	case r.ExitCode == nil:
		return goerrors.Internal(fmt.Errorf("process killed")).
			WithDetail("killed", true)
	case *r.ExitCode == 0:
		return nil
	default:
		return goerrors.Internal(fmt.Errorf("process exited with code %d", *r.ExitCode)).
			WithDetail("exit_code", *r.ExitCode)
	}
}

// exitCodeLabel renders an exit code for diagnostics, using "killed" for an unknown exit.
func exitCodeLabel(code *int) string {
	if code == nil {
		return "killed"
	}
	return fmt.Sprintf("%d", *code)
}
