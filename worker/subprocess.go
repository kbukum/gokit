package worker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/kbukum/gokit/process"
)

const defaultSubprocessLineBytes = 1024 * 1024

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
// Uses the process package for argv-only execution, process group isolation,
// and SIGTERM→SIGKILL graceful shutdown.
func NewSubprocessHandler(cfg SubprocessConfig) Handler[SubprocessInput, SubprocessOutput] {
	cfg = cfg.withDefaults()

	return HandlerFunc[SubprocessInput, SubprocessOutput](func(
		ctx context.Context, task SubprocessInput, emit func(Event[SubprocessOutput]),
	) error {
		if cfg.Command.Binary == "" {
			return fmt.Errorf("worker: subprocess binary is required")
		}

		cmd := cfg.Command
		cmd.Args = task.Args
		cmd.Stdin = task.Stdin

		maxLineBytes := cfg.Command.MaxOutputBytes
		if maxLineBytes <= 0 {
			maxLineBytes = defaultSubprocessLineBytes
		}

		lines := map[process.StreamName]*lineEmitter{
			process.StreamStdout: {stream: string(process.StreamStdout), maxBytes: maxLineBytes, emit: emit},
			process.StreamStderr: {stream: string(process.StreamStderr), maxBytes: maxLineBytes, emit: emit},
		}
		_, err := process.Stream(ctx, cmd, func(chunk process.StreamChunk) {
			lines[chunk.Stream].Write(chunk.Data)
		})
		for _, line := range lines {
			line.Flush()
		}
		if err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("worker: %s killed by context: %w", cfg.Command.Binary, ctx.Err())
			}
			return fmt.Errorf("worker: %s: %w", cfg.Command.Binary, err)
		}

		return nil
	})
}

type lineEmitter struct {
	stream   string
	pending  []byte
	maxBytes int
	emit     func(Event[SubprocessOutput])
}

func (e *lineEmitter) Write(data []byte) {
	e.pending = append(e.pending, data...)
	for {
		idx := bytes.IndexByte(e.pending, '\n')
		if idx < 0 {
			if len(e.pending) >= e.maxBytes {
				e.emitLine(e.pending[:e.maxBytes])
				e.pending = e.pending[e.maxBytes:]
				continue
			}
			return
		}
		line := bytes.TrimSuffix(e.pending[:idx], []byte("\r"))
		e.pending = e.pending[idx+1:]
		e.emitLine(line)
	}
}

func (e *lineEmitter) Flush() {
	if len(e.pending) == 0 {
		return
	}
	e.emitLine(bytes.TrimSuffix(e.pending, []byte("\r")))
	e.pending = nil
}

func (e *lineEmitter) emitLine(line []byte) {
	e.emit(PartialEvent(SubprocessOutput{
		Stream: e.stream,
		Line:   string(line),
	}))
}
