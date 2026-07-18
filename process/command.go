package process

import (
	"io"
	"time"
)

// Command configures a subprocess to execute.
type Command struct {
	// Binary is the executable path or name (resolved via PATH).
	Binary string
	// Args are the command-line arguments.
	Args []string
	// Dir is the working directory. If empty, uses the current directory.
	Dir string
	// Env is additional environment variables (key=value). By default these are merged with the parent environment.
	Env []string
	// ScrubEnv starts from an empty environment instead of inheriting the parent.
	ScrubEnv bool
	// Stdin provides input to the process. May be nil.
	Stdin io.Reader
	// MaxOutputBytes bounds captured stdout and stderr independently. Zero or negative means unlimited capture.
	MaxOutputBytes int
	// GracePeriod is how long to wait after SIGTERM before SIGKILL. Defaults to 5 seconds if zero.
	GracePeriod time.Duration
}
