package client

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"

	"github.com/kbukum/gokit/resilience"
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
	conn *grpc.ClientConn,
	connectTimeout time.Duration,
	opener StreamOpener[T],
) (T, error) {
	var zero T

	if connectTimeout <= 0 {
		return opener(ctx)
	}

	_, err := resilience.Await(ctx, connectTimeout, func(waitCtx context.Context) (struct{}, error) {
		return struct{}{}, waitForConnectionReady(waitCtx, conn)
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			if ctx.Err() != nil {
				return zero, ctx.Err()
			}
			return zero, fmt.Errorf("stream connection timeout after %v: %w", connectTimeout, context.DeadlineExceeded)
		}
		return zero, err
	}
	return opener(ctx)
}

// TryOpenStream attempts to open a stream with a short timeout.
// If it fails, it returns the zero value of T and the error without blocking the caller.
func TryOpenStream[T any](
	ctx context.Context,
	conn *grpc.ClientConn,
	timeout time.Duration,
	opener StreamOpener[T],
) (T, error) {
	return OpenStreamWithTimeout(ctx, conn, timeout, opener)
}

func waitForConnectionReady(ctx context.Context, conn *grpc.ClientConn) error {
	if conn == nil {
		return fmt.Errorf("grpc: client connection is nil")
	}

	conn.Connect()
	for {
		state := conn.GetState()
		switch state {
		case connectivity.Ready:
			return nil
		case connectivity.Shutdown:
			return fmt.Errorf("grpc: client connection is shut down")
		case connectivity.Idle, connectivity.Connecting, connectivity.TransientFailure:
		}

		if !conn.WaitForStateChange(ctx, state) {
			return ctx.Err()
		}
	}
}
