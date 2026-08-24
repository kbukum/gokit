package testutil_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kbukum/gokit/component"
	gtestutil "github.com/kbukum/gokit/grpc/testutil"
)

func TestServer_Invoke_DefaultSuccess(t *testing.T) {
	t.Parallel()

	srv := gtestutil.NewServer()
	t.Cleanup(func() { _ = srv.Stop(context.Background()) })

	conn, err := srv.Dial()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	require.NoError(t, srv.Invoke(context.Background(), conn))
}

func TestServer_Invoke_HandlerError(t *testing.T) {
	t.Parallel()

	srv := gtestutil.NewServer()
	t.Cleanup(func() { _ = srv.Stop(context.Background()) })
	srv.SetUnaryHandler(func(context.Context) error {
		return status.Error(codes.PermissionDenied, "denied")
	})

	conn, err := srv.Dial()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	err = srv.Invoke(context.Background(), conn)
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestServer_Invoke_HandlerObservesCancellation(t *testing.T) {
	t.Parallel()

	srv := gtestutil.NewServer()
	t.Cleanup(func() { _ = srv.Stop(context.Background()) })

	handlerStarted := make(chan struct{})
	handlerDone := make(chan struct{})
	srv.SetUnaryHandler(func(ctx context.Context) error {
		close(handlerStarted)
		<-ctx.Done()
		close(handlerDone)
		return ctx.Err()
	})

	conn, err := srv.Dial()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Invoke(ctx, conn) }()

	select {
	case <-handlerStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("server handler was never entered")
	}
	cancel()
	select {
	case <-handlerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("server handler did not observe cancellation")
	}
	require.Error(t, <-errCh)
}

func TestServer_Reset_ClearsHandler(t *testing.T) {
	t.Parallel()

	srv := gtestutil.NewServer()
	t.Cleanup(func() { _ = srv.Stop(context.Background()) })
	srv.SetUnaryHandler(func(context.Context) error {
		return status.Error(codes.Internal, "boom")
	})

	require.NoError(t, srv.Reset(context.Background()))

	conn, err := srv.Dial()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, srv.Invoke(context.Background(), conn))
}

func TestServer_Health(t *testing.T) {
	t.Parallel()

	srv := gtestutil.NewServer()
	assert.True(t, srv.Health(context.Background()).IsHealthy())

	require.NoError(t, srv.Stop(context.Background()))
	assert.False(t, srv.Health(context.Background()).IsHealthy())

	require.NoError(t, srv.Start(context.Background()))
	assert.True(t, srv.Health(context.Background()).IsHealthy())
	t.Cleanup(func() { _ = srv.Stop(context.Background()) })
}

func TestServer_SnapshotRestoreUnsupported(t *testing.T) {
	t.Parallel()

	srv := gtestutil.NewServer()
	t.Cleanup(func() { _ = srv.Stop(context.Background()) })

	_, err := srv.Snapshot(context.Background())
	require.ErrorIs(t, err, errors.ErrUnsupported)
	require.ErrorIs(t, srv.Restore(context.Background(), nil), errors.ErrUnsupported)
}

func TestServer_ImplementsTestComponent(t *testing.T) {
	t.Parallel()

	var _ component.Component = (*gtestutil.Server)(nil)
	// Compile-time assertion mirrored at runtime for documentation value.
	var calls atomic.Int32
	srv := gtestutil.NewServer()
	t.Cleanup(func() { _ = srv.Stop(context.Background()) })
	srv.SetUnaryHandler(func(context.Context) error {
		calls.Add(1)
		return nil
	})
	conn, err := srv.Dial()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, srv.Invoke(context.Background(), conn))
	assert.Equal(t, int32(1), calls.Load())
}
