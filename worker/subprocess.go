package worker

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/kbukum/gokit/process"
)

// SubprocessConfig configures a subprocess-based handler.
// Uses process.Command for the static command definition; per-task
// arguments and stdin are supplied via SubprocessInput.
type SubprocessConfig struct {
	// Command defines the binary, working directory, environment, and
	// grace period. Args and Stdin in Command are ignored — use
	// SubprocessInput for per-task values.
	Command process.Command `yaml:"command" mapstructure:"command"`
}

func (c SubprocessConfig) withDefaults() SubprocessConfig {
	if c.Command.GracePeriod <= 0 {
		c.Command.GracePeriod = 5 * time.Second
	}
	return c
}

// SubprocessInput is the task input for SubprocessHandler.
type SubprocessInput struct {
	Args  []string
	Stdin io.Reader
}

// SubprocessOutput represents one line of subprocess output.
type SubprocessOutput struct {
	Stream string // "stdout" or "stderr"
	Line   string
}

// NewSubprocessHandler creates a Handler that runs a subprocess and
// emits each stdout/stderr line as an EventPartial.
// Uses process group isolation and SIGTERM→SIGKILL graceful shutdown.
func NewSubprocessHandler(cfg SubprocessConfig) Handler[SubprocessInput, SubprocessOutput] {
	cfg = cfg.withDefaults()

	return HandlerFunc[SubprocessInput, SubprocessOutput](func(
		ctx context.Context, task SubprocessInput, emit func(Event[SubprocessOutput]),
	) error {
		if cfg.Command.Binary == "" {
			return fmt.Errorf("worker: subprocess binary is required")
		}

		cmd := exec.CommandContext(ctx, cfg.Command.Binary, task.Args...) //nolint:gosec // dynamic args are the purpose
		cmd.Dir = cfg.Command.Dir

		if len(cfg.Command.Env) > 0 {
			cmd.Env = append(cmd.Environ(), cfg.Command.Env...)
		}

		if task.Stdin != nil {
			cmd.Stdin = task.Stdin
		}

		// Process group isolation for tree kill (mirrors process.Run setup)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

		// Graceful shutdown: SIGTERM first, then SIGKILL after grace period
		cmd.Cancel = func() error {
			if cmd.Process == nil {
				return nil
			}
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		}
		cmd.WaitDelay = cfg.Command.GracePeriod

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return fmt.Errorf("worker: stdout pipe: %w", err)
		}

		stderr, err := cmd.StderrPipe()
		if err != nil {
			return fmt.Errorf("worker: stderr pipe: %w", err)
		}

		if err := cmd.Start(); err != nil {
			return fmt.Errorf("worker: start %s: %w", cfg.Command.Binary, err)
		}

		// Stream stdout and stderr concurrently
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			scanLines(stdout, "stdout", emit)
		}()

		go func() {
			defer wg.Done()
			scanLines(stderr, "stderr", emit)
		}()

		wg.Wait()

		if err := cmd.Wait(); err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("worker: %s killed by context: %w", cfg.Command.Binary, ctx.Err())
			}
			return fmt.Errorf("worker: %s: %w", cfg.Command.Binary, err)
		}

		return nil
	})
}

func scanLines(r io.Reader, stream string, emit func(Event[SubprocessOutput])) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		emit(PartialEvent(SubprocessOutput{
			Stream: stream,
			Line:   scanner.Text(),
		}))
	}
}
