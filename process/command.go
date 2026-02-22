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
	// Env is additional environment variables (key=value). Merged with os.Environ.
	Env []string
	// Stdin provides input to the process. May be nil.
	Stdin io.Reader
	// GracePeriod is how long to wait after SIGTERM before SIGKILL.
	// Defaults to 5 seconds if zero.
	GracePeriod time.Duration
}
