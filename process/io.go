package process

import (
	"os"
	"os/exec"
)

// IOMode selects how a subprocess's standard streams are wired.
type IOMode int

const (
	// IOCaptured pipes stdout and stderr into bounded in-memory buffers exposed on Result.
	// This is the default and matches Run's historical behavior.
	IOCaptured IOMode = iota
	// IOObserved streams stdout and stderr live through a callback while it runs.
	// This mode is realized by Stream; Run treats it as IOCaptured.
	IOObserved
	// IOInherited passes the parent's os.Stdout and os.Stderr straight to the child,
	// so its output goes to the terminal and nothing is captured on Result.
	IOInherited
)

// InputPolicy selects how a subprocess's standard input is wired.
//
// When Command.Stdin is non-nil it always takes precedence (the Bytes
// policy): the reader is fed to the child and then closed. Otherwise the
// policy chooses between a closed stdin and inheriting the parent's stdin.
type InputPolicy int

const (
	// InputClosed leaves the child without stdin. This is the default.
	InputClosed InputPolicy = iota
	// InputInherit passes the parent's os.Stdin to the child.
	InputInherit
)

// applyInput wires the child's stdin according to Command.Stdin and Command.Input.
func applyInput(c *exec.Cmd, cmd Command) {
	switch {
	case cmd.Stdin != nil:
		c.Stdin = cmd.Stdin
	case cmd.Input == InputInherit:
		c.Stdin = os.Stdin
	}
}
