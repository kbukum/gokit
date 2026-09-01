package process

import (
	"context"
	stderrors "errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	goerrors "github.com/kbukum/gokit/errors"
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

// Stream executes a subprocess and emits stdout/stderr chunks while it runs. When emit is non-nil,
// Stream invokes it sequentially from an internal goroutine. The callback should return promptly;
// a slow callback can still apply backpressure to subprocess pipe reads after the internal buffer fills.
func Stream(ctx context.Context, cmd Command, emit func(StreamChunk)) (*Result, error) {
	if cmd.Binary == "" {
		return nil, goerrors.MissingField("binary")
	}

	policy := resolveLifecycle(cmd)

	// A fresh command is built on every start attempt: an ETXTBSY retry cannot reuse a
	// Cmd whose Start already failed. The pipes are recreated per attempt and captured
	// for the reader goroutines below once a start succeeds.
	var (
		c                      *exec.Cmd
		stdoutPipe, stderrPipe io.ReadCloser
	)
	var buildErr error
	err := startWithETXTBSYRetry(func() error {
		c = exec.CommandContext(ctx, cmd.Binary, cmd.Args...) //nolint:gosec // dynamic args are the purpose of this package
		c.Dir = cmd.Dir
		c.Env = mergeEnv(cmd.Env, cmd.EnvPolicy)
		applyInput(c, cmd)
		applyLifecycle(c, policy)

		if stdoutPipe, buildErr = c.StdoutPipe(); buildErr != nil {
			return buildErr
		}
		if stderrPipe, buildErr = c.StderrPipe(); buildErr != nil {
			return buildErr
		}
		return c.Start()
	})
	if buildErr != nil {
		return nil, fmt.Errorf("process: pipe: %w", buildErr)
	}
	if err != nil {
		return nil, SpawnError(fmt.Sprintf("process: start %s", cmd.Binary), err)
	}

	start := time.Now()
	stdout := newLimitedBuffer(cmd.MaxOutputBytes)
	stderr := newLimitedBuffer(cmd.MaxOutputBytes)

	var wg sync.WaitGroup
	var emitWG sync.WaitGroup
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

	var chunks chan StreamChunk
	if emit != nil {
		chunks = make(chan StreamChunk, 64)
		emitWG.Add(1)
		go func() {
			defer emitWG.Done()
			for chunk := range chunks {
				emit(chunk)
			}
		}()
	}

	wg.Add(2)
	go func() {
		defer wg.Done()
		recordCopyErr(copyStream(stdoutPipe, stdout, StreamStdout, chunks))
	}()
	go func() {
		defer wg.Done()
		recordCopyErr(copyStream(stderrPipe, stderr, StreamStderr, chunks))
	}()

	wg.Wait()
	waitErr := c.Wait()
	if chunks != nil {
		close(chunks)
		emitWG.Wait()
	}
	duration := time.Since(start)

	result := &Result{
		Stdout:          stdout.Bytes(),
		StdoutTruncated: stdout.Truncated(),
		Stderr:          stderr.Bytes(),
		StderrTruncated: stderr.Truncated(),
		ExitCode:        exitCodeOf(c.ProcessState),
		Duration:        duration,
	}

	if copyErr != nil {
		return result, fmt.Errorf("process: stream output: %w", copyErr)
	}
	if waitErr != nil {
		switch {
		case stderrors.Is(ctx.Err(), context.DeadlineExceeded):
			result.TimedOut = true
			return result, goerrors.Timeout("process").WithCause(waitErr)
		case stderrors.Is(ctx.Err(), context.Canceled):
			result.Canceled = true
			return result, goerrors.Canceled("process").WithCause(waitErr)
		}
		return result, goerrors.Internal(
			fmt.Errorf("process: exit code %s: %w", exitCodeLabel(result.ExitCode), waitErr),
		).WithDetail("exit_code", result.ExitCodeOr(-1))
	}

	return result, nil
}

func copyStream(r io.Reader, capture *limitedBuffer, stream StreamName, chunks chan<- StreamChunk) error {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			data := buf[:n]
			if chunks == nil {
				if _, writeErr := capture.Write(data); writeErr != nil {
					return writeErr
				}
			} else {
				chunk := append([]byte(nil), data...)
				if _, writeErr := capture.Write(chunk); writeErr != nil {
					return writeErr
				}
				chunks <- StreamChunk{Stream: stream, Data: chunk}
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
