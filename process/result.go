package process

import "time"

// Result holds the output and status of a completed subprocess.
type Result struct {
	// Stdout is the captured standard output.
	Stdout []byte
	// Stderr is the captured standard error.
	Stderr []byte
	// ExitCode is the process exit code. -1 if the process was killed.
	ExitCode int
	// Duration is how long the process ran.
	Duration time.Duration
}
