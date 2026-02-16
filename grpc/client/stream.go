package client

import (
	"context"
	"fmt"
	"time"
)

// StreamOpener is a function type that opens a gRPC stream.
type StreamOpener[T any] func(ctx context.Context) (T, error)

// OpenStreamWithTimeout opens a gRPC stream with a connection establishment timeout.
//
// Unlike context.WithTimeout, this function only applies the timeout to the
// stream establishment phase. Once the stream is successfully opened, it runs
// with the original context (no timeout affecting the stream lifetime).
//
// This solves the fundamental problem with gRPC stream timeouts:
//   - If you pass a timeout context to the stream, the timeout kills the stream
//   - If you don't use a timeout, WaitForReady can block indefinitely
func OpenStreamWithTimeout[T any](
	ctx context.Context,
	connectTimeout time.Duration,
	opener StreamOpener[T],
) (T, error) {
	var zero T

	if connectTimeout <= 0 {
		return opener(ctx)
	}

	type result struct {
		stream T
		err    error
	}
	resultCh := make(chan result, 1)

	go func() {
		stream, err := opener(ctx)
		resultCh <- result{stream: stream, err: err}
	}()

	select {
	case res := <-resultCh:
		return res.stream, res.err
	case <-time.After(connectTimeout):
		return zero, fmt.Errorf("stream connection timeout after %v", connectTimeout)
	case <-ctx.Done():
		return zero, ctx.Err()
	}
}

// TryOpenStream attempts to open a stream with a short timeout.
// If it fails, it returns nil and the error without blocking the caller.
func TryOpenStream[T any](
	ctx context.Context,
	timeout time.Duration,
	opener StreamOpener[T],
) (T, error) {
	return OpenStreamWithTimeout(ctx, timeout, opener)
}
