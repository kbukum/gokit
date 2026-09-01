package process

import (
	"io"
	"time"
)

// EnvPolicy selects the base environment a subprocess starts from before Env overrides are
// applied.
type EnvPolicy int

const (
	// EnvInherit inherits the parent process environment, then applies Env overrides on top.
	// This is the zero value and the default.
	EnvInherit EnvPolicy = iota
	// EnvEmpty starts from an empty environment and applies only the explicit Env entries.
	EnvEmpty
)

// Command configures a subprocess to execute.
type Command struct {
	// Binary is the executable path or name (resolved via PATH).
	Binary string
	// Args are the command-line arguments.
	Args []string
	// Dir is the working directory. If empty, uses the current directory.
	Dir string
	// Env holds environment variables applied to the child as key=value pairs, merged onto
	// the base environment selected by EnvPolicy.
	Env map[string]string
	// EnvPolicy selects the base environment: inherit the parent (default) or start empty.
	EnvPolicy EnvPolicy
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
