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
	// Env is additional environment variables (key=value).
	// By default these are merged with the parent environment.
	Env []string
	// ScrubEnv starts from an empty environment instead of inheriting the parent.
	ScrubEnv bool
	// Stdin provides input to the process. May be nil. When set it takes precedence
	// over Input and is fed to the child, then closed.
	Stdin io.Reader
	// Input selects the stdin policy when Stdin is nil: closed (default) or inherited
	// from the parent process.
	Input InputPolicy
	// IO selects how stdout and stderr are wired. The zero value, IOCaptured, pipes both
	// into bounded buffers on Result. IOInherited forwards them to the parent's terminal
	// without capture. Stream always observes output live regardless of this field.
	IO IOMode
	// MaxOutputBytes bounds captured stdout and stderr independently. Zero
	// or negative means unlimited capture.
	MaxOutputBytes int
	// GracePeriod is how long to wait after SIGTERM before SIGKILL. Defaults to 5 seconds if zero.
	// When Lifecycle is set its GracePeriod takes precedence.
	GracePeriod time.Duration
	// Lifecycle configures process-group isolation and shutdown escalation. When nil,
	// Run and Stream apply DefaultLifecyclePolicy with GracePeriod honored as an override.
	Lifecycle *LifecyclePolicy
}
