package client

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	apperrors "github.com/kbukum/gokit/errors"
	grpccfg "github.com/kbukum/gokit/grpc"
	"github.com/kbukum/gokit/grpc/interceptor"
	grpctestutil "github.com/kbukum/gokit/grpc/testutil"
	"github.com/kbukum/gokit/resilience"
)

// These tests exercise the gRPC client stack end to end against an in-process
// fake server (bufconn, no real network): dialing, deadlines, cancellation, and
// the canonical AppError <-> gRPC status mapping proven over the wire.

func dialFake(t *testing.T, srv *grpctestutil.Server, opts ...grpc.DialOption) *grpc.ClientConn {
	t.Helper()
	conn, err := srv.Dial(opts...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func newFakeServer(t *testing.T) *grpctestutil.Server {
	t.Helper()
	srv := grpctestutil.NewServer()
	t.Cleanup(func() { _ = srv.Stop(context.Background()) })
	return srv
}

// ---------------------------------------------------------------------------
// canonical AppError <-> status mapping, proven over the wire
// ---------------------------------------------------------------------------

func TestClient_CanonicalMapping_OverWire(t *testing.T) {
	t.Parallel()

	codesUnderTest := []apperrors.ErrorCode{
		apperrors.ErrCodeServiceUnavailable,
		apperrors.ErrCodeTimeout,
		apperrors.ErrCodeRateLimited,
		apperrors.ErrCodeNotFound,
		apperrors.ErrCodeAlreadyExists,
		apperrors.ErrCodeConflict,
		apperrors.ErrCodeInvalidInput,
		apperrors.ErrCodeUnauthorized,
		apperrors.ErrCodeForbidden,
		apperrors.ErrCodeInternal,
	}

	for _, code := range codesUnderTest {
		t.Run(string(code), func(t *testing.T) {
			t.Parallel()
			want := &apperrors.AppError{
				Code:      code,
				Message:   "boundary failure for " + string(code),
				Retryable: apperrors.IsRetryableCode(code),
				Details:   map[string]any{"trace": string(code)},
			}
			localSrv := newFakeServer(t)
			localSrv.SetUnaryHandler(func(context.Context) error {
				return grpccfg.AppErrorToStatus(want).Err()
			})
			localConn := dialFake(t, localSrv)

			err := localSrv.Invoke(context.Background(), localConn)
			require.Error(t, err)

			st := status.Convert(err)
			assert.Equal(t, grpccfg.ErrorCodeToGRPCCode(code), st.Code(),
				"gRPC code must survive the wire")

			got := grpccfg.StatusToAppError(st)
			require.NotNil(t, got)
			assert.Equal(t, want.Code, got.Code, "error code must round-trip losslessly")
			assert.Equal(t, want.Message, got.Message, "message must round-trip losslessly")
			assert.Equal(t, want.Retryable, got.Retryable)
			require.NotNil(t, got.Details)
			assert.Equal(t, string(code), got.Details["trace"], "extension members must round-trip")
		})
	}
}

// ---------------------------------------------------------------------------
// user-facing FromGRPC mapping, proven over the wire
// ---------------------------------------------------------------------------

func TestClient_FromGRPC_OverWire(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		status   *status.Status
		wantCode apperrors.ErrorCode
	}{
		{"unavailable", status.New(codes.Unavailable, "down"), apperrors.ErrCodeServiceUnavailable},
		{"not_found", status.New(codes.NotFound, "missing"), apperrors.ErrCodeNotFound},
		{"invalid_argument", status.New(codes.InvalidArgument, "bad"), apperrors.ErrCodeInvalidInput},
		{"permission_denied", status.New(codes.PermissionDenied, "nope"), apperrors.ErrCodeForbidden},
		{"resource_exhausted", status.New(codes.ResourceExhausted, "slow down"), apperrors.ErrCodeRateLimited},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := newFakeServer(t)
			srv.SetUnaryHandler(func(context.Context) error { return tc.status.Err() })
			conn := dialFake(t, srv)

			err := srv.Invoke(context.Background(), conn)
			require.Error(t, err)

			appErr := grpccfg.FromGRPC(err, "orders")
			require.NotNil(t, appErr)
			assert.Equal(t, tc.wantCode, appErr.Code)
			assert.ErrorIs(t, appErr, err, "mapping must preserve the underlying cause")
		})
	}
}

// ---------------------------------------------------------------------------
// deadline / cancellation, proven over the wire
// ---------------------------------------------------------------------------

func TestClient_DeadlineExceeded_OverWire(t *testing.T) {
	t.Parallel()

	srv := newFakeServer(t)
	srv.SetUnaryHandler(func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	conn := dialFake(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := srv.Invoke(ctx, conn)
	require.Error(t, err)
	assert.Equal(t, codes.DeadlineExceeded, status.Code(err), "deadline must surface as DeadlineExceeded")

	appErr := grpccfg.FromGRPC(err, "orders")
	require.NotNil(t, appErr)
	assert.Equal(t, apperrors.ErrCodeTimeout, appErr.Code)
}

func TestClient_Cancellation_OverWire(t *testing.T) {
	t.Parallel()

	srv := newFakeServer(t)
	started := make(chan struct{})
	srv.SetUnaryHandler(func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	conn := dialFake(t, srv)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Invoke(ctx, conn) }()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("call never reached the server")
	}
	cancel()

	select {
	case err := <-errCh:
		require.Error(t, err)
		assert.Equal(t, codes.Canceled, status.Code(err), "cancellation must surface as Canceled")
	case <-time.After(2 * time.Second):
		t.Fatal("canceled call did not return")
	}
}

// ---------------------------------------------------------------------------
// connection failure (no reachable server)
// ---------------------------------------------------------------------------

func TestClient_ConnectionFailure_OverWire(t *testing.T) {
	t.Parallel()

	conn := unavailableConn(t)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := conn.Invoke(ctx, grpctestutil.MethodUnary, new(emptypb.Empty), new(emptypb.Empty))
	require.Error(t, err)

	appErr := grpccfg.FromGRPC(err, "orders")
	require.NotNil(t, appErr)
	assert.Contains(t,
		[]apperrors.ErrorCode{apperrors.ErrCodeServiceUnavailable, apperrors.ErrCodeTimeout},
		appErr.Code,
		"a dead endpoint must map to an unavailable/timeout error, never a success-shaped zero value")
}

// ---------------------------------------------------------------------------
// resilience: retries flow through the shared resilience seam, not a hand roll
// ---------------------------------------------------------------------------

func TestClient_ResilienceInterceptor_RetriesOverWire(t *testing.T) {
	t.Parallel()

	srv := newFakeServer(t)
	var calls atomic.Int32
	srv.SetUnaryHandler(func(context.Context) error {
		if calls.Add(1) < 3 {
			return status.Error(codes.Unavailable, "warming up")
		}
		return nil
	})

	policy := resilience.NewPolicy().
		WithTimeout(5 * time.Second).
		WithRetry(*DefaultRetryPolicy())
	conn := dialFake(t, srv,
		grpc.WithChainUnaryInterceptor(interceptor.UnaryClientResilienceInterceptor(policy)))

	err := srv.Invoke(context.Background(), conn)
	require.NoError(t, err, "retryable Unavailable responses must be retried to success")
	assert.Equal(t, int32(3), calls.Load(), "resilience policy should retry until the call succeeds")
}
