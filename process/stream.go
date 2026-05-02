package process

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// StreamName identifies a subprocess output stream.
type StreamName string

const (
	// StreamStdout identifies standard output chunks.
	StreamStdout StreamName = "stdout"
	// StreamStderr identifies standard error chunks.
	StreamStderr StreamName = "stderr"
)

// StreamChunk is one chunk of subprocess output.
type StreamChunk struct {
	Stream StreamName
	Data   []byte
}

// Stream executes a subprocess and emits stdout/stderr chunks while it runs.
func Stream(ctx context.Context, cmd Command, emit func(StreamChunk)) (*Result, error) {
	if cmd.Binary == "" {
		return nil, fmt.Errorf("process: binary is required")
	}

	gracePeriod := cmd.GracePeriod
	if gracePeriod == 0 {
		gracePeriod = 5 * time.Second
	}

	c := exec.CommandContext(ctx, cmd.Binary, cmd.Args...) //nolint:gosec // dynamic args are the purpose of this package
	c.Dir = cmd.Dir
	c.Env = mergeEnv(cmd.Env, cmd.ScrubEnv)
	if cmd.Stdin != nil {
		c.Stdin = cmd.Stdin
	}

	configureSysProcAttr(c)
	c.Cancel = func() error {
		return terminateGracefully(c)
	}
	c.WaitDelay = gracePeriod

	stdoutPipe, err := c.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("process: stdout pipe: %w", err)
	}
	stderrPipe, err := c.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("process: stderr pipe: %w", err)
	}

	if err := c.Start(); err != nil {
		return nil, fmt.Errorf("process: start %s: %w", cmd.Binary, err)
	}

	start := time.Now()
	stdout := newLimitedBuffer(cmd.MaxOutputBytes)
	stderr := newLimitedBuffer(cmd.MaxOutputBytes)

	var wg sync.WaitGroup
	var copyErr error
	var copyMu sync.Mutex
	recordCopyErr := func(err error) {
		if err == nil {
			return
		}
		copyMu.Lock()
		defer copyMu.Unlock()
		if copyErr == nil {
			copyErr = err
		}
	}

	wg.Add(2)
	go func() {
		defer wg.Done()
		recordCopyErr(copyStream(stdoutPipe, stdout, StreamStdout, emit))
	}()
	go func() {
		defer wg.Done()
		recordCopyErr(copyStream(stderrPipe, stderr, StreamStderr, emit))
	}()

	waitErr := c.Wait()
	wg.Wait()
	duration := time.Since(start)

	exitCode := -1
	if c.ProcessState != nil {
		exitCode = c.ProcessState.ExitCode()
	}

	result := &Result{
		Stdout:          stdout.Bytes(),
		StdoutTruncated: stdout.Truncated(),
		Stderr:          stderr.Bytes(),
		StderrTruncated: stderr.Truncated(),
		ExitCode:        exitCode,
		Duration:        duration,
	}

	if copyErr != nil {
		return result, fmt.Errorf("process: stream output: %w", copyErr)
	}
	if waitErr != nil {
		if ctx.Err() != nil {
			return result, fmt.Errorf("process: killed by context: %w", ctx.Err())
		}
		return result, fmt.Errorf("process: exit code %d: %w", result.ExitCode, waitErr)
	}

	return result, nil
}

func copyStream(r io.Reader, capture *limitedBuffer, stream StreamName, emit func(StreamChunk)) error {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			if _, writeErr := capture.Write(chunk); writeErr != nil {
				return writeErr
			}
			if emit != nil {
				emit(StreamChunk{Stream: stream, Data: chunk})
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}
