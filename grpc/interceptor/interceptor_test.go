package interceptor

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/resilience"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// testConn creates a lightweight gRPC client (lazy — no real connection).
func testConn(t *testing.T) *grpc.ClientConn {
	t.Helper()
	cc, err := grpc.NewClient("passthrough:///test-target",
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cc.Close() })
	return cc
}

func testLogger() *logging.Logger {
	return logging.NewDefault("test")
}

func mockInvoker(retErr error) grpc.UnaryInvoker {
	return func(ctx context.Context, method string, req, reply any,
		cc *grpc.ClientConn, opts ...grpc.CallOption,
	) error {
		return retErr
	}
}

func deadlineCapturingInvoker(captured *time.Time) grpc.UnaryInvoker {
	return func(ctx context.Context, method string, req, reply any,
		cc *grpc.ClientConn, opts ...grpc.CallOption,
	) error {
		if dl, ok := ctx.Deadline(); ok {
			*captured = dl
		}
		return nil
	}
}

type mockClientStream struct{ grpc.ClientStream }

func mockStreamer(stream grpc.ClientStream, retErr error) grpc.Streamer {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn,
		method string, opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		return stream, retErr
	}
}

// ---------------------------------------------------------------------------
// UnaryClientResilienceInterceptor
// ---------------------------------------------------------------------------

func TestUnaryResilienceInterceptor_AppliesTimeoutWhenUnset(t *testing.T) {
	t.Parallel()

	interceptor := UnaryClientResilienceInterceptor(resilience.NewPolicy().WithTimeoutIfUnset(500 * time.Millisecond))
	cc := testConn(t)

	var captured time.Time
	err := interceptor(context.Background(), "/pkg.Svc/Method", nil, nil,
		cc, deadlineCapturingInvoker(&captured))

	require.NoError(t, err)
	assert.False(t, captured.IsZero(), "deadline should have been set")
	assert.WithinDuration(t, time.Now().Add(500*time.Millisecond), captured, 100*time.Millisecond)
}

func TestUnaryResilienceInterceptor_PreservesExistingDeadline(t *testing.T) {
	t.Parallel()

	interceptor := UnaryClientResilienceInterceptor(resilience.NewPolicy().WithTimeoutIfUnset(500 * time.Millisecond))
	cc := testConn(t)

	existingDeadline := time.Now().Add(10 * time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), existingDeadline)
	defer cancel()

	var captured time.Time
	err := interceptor(ctx, "/pkg.Svc/Method", nil, nil,
		cc, deadlineCapturingInvoker(&captured))

	require.NoError(t, err)
	assert.Equal(t, existingDeadline.Unix(), captured.Unix(),
		"existing deadline should NOT be overridden")
}

func TestUnaryResilienceInterceptor_NilPolicy(t *testing.T) {
	t.Parallel()

	interceptor := UnaryClientResilienceInterceptor(nil)
	cc := testConn(t)

	var captured time.Time
	err := interceptor(context.Background(), "/pkg.Svc/Method", nil, nil,
		cc, deadlineCapturingInvoker(&captured))

	require.NoError(t, err)
	assert.True(t, captured.IsZero(), "nil policy should not set a deadline")
}

func TestUnaryResilienceInterceptor_PropagatesInvokerError(t *testing.T) {
	t.Parallel()

	policy := resilience.NewPolicy().WithTimeoutIfUnset(time.Second)
	interceptor := UnaryClientResilienceInterceptor(policy)
	cc := testConn(t)

	wantErr := status.Error(codes.Internal, "boom")
	err := interceptor(context.Background(), "/pkg.Svc/Method", nil, nil,
		cc, mockInvoker(wantErr))

	assert.Equal(t, wantErr, err)
}

func TestRetriesRequireExplicitIdempotency(t *testing.T) {
	t.Parallel()
	for _, idempotent := range []bool{false, true} {
		t.Run(fmt.Sprint(idempotent), func(t *testing.T) {
			t.Parallel()
			cfg := resilience.DefaultRetryConfig()
			cfg.InitialBackoff = time.Nanosecond
			policy := resilience.NewPolicy().WithRetry(cfg)
			calls := 0
			invoke := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
				calls++
				if calls == 1 {
					return status.Error(codes.Unavailable, "retry hint is not idempotency")
				}
				return nil
			}
			var opts []grpc.CallOption
			if idempotent {
				opts = append(opts, Idempotent())
			}
			err := UnaryClientResilienceInterceptor(policy)(context.Background(), "/svc/Create", nil, nil, nil, invoke, opts...)
			if idempotent {
				require.NoError(t, err)
				require.Equal(t, 2, calls)
			} else {
				require.Error(t, err)
				require.Equal(t, 1, calls)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// UnaryClientLoggingInterceptor
// ---------------------------------------------------------------------------

func TestUnaryLoggingInterceptor_Success(t *testing.T) {
	t.Parallel()

	log := testLogger()
	interceptor := UnaryClientLoggingInterceptor(log)
	cc := testConn(t)

	err := interceptor(context.Background(), "/my.pkg.Svc/GetUser", nil, nil,
		cc, mockInvoker(nil))
	require.NoError(t, err)
}

func TestUnaryLoggingInterceptor_Error(t *testing.T) {
	t.Parallel()

	log := testLogger()
	interceptor := UnaryClientLoggingInterceptor(log)
	cc := testConn(t)

	grpcErr := status.Error(codes.NotFound, "user not found")
	err := interceptor(context.Background(), "/my.pkg.Svc/GetUser", nil, nil,
		cc, mockInvoker(grpcErr))

	require.Error(t, err)
	assert.Equal(t, grpcErr, err, "error should be passed through")
}

// ---------------------------------------------------------------------------
// StreamClientLoggingInterceptor
// ---------------------------------------------------------------------------

func TestStreamLoggingInterceptor_Success(t *testing.T) {
	t.Parallel()

	log := testLogger()
	interceptor := StreamClientLoggingInterceptor(log)
	cc := testConn(t)
	desc := &grpc.StreamDesc{ServerStreams: true}

	stream, err := interceptor(context.Background(), desc, cc,
		"/my.pkg.Svc/StreamEvents", mockStreamer(&mockClientStream{}, nil))

	require.NoError(t, err)
	assert.NotNil(t, stream)
}

func TestStreamLoggingInterceptor_Error(t *testing.T) {
	t.Parallel()

	log := testLogger()
	interceptor := StreamClientLoggingInterceptor(log)
	cc := testConn(t)
	desc := &grpc.StreamDesc{ServerStreams: true}

	grpcErr := status.Error(codes.Unavailable, "server down")
	stream, err := interceptor(context.Background(), desc, cc,
		"/my.pkg.Svc/StreamEvents", mockStreamer(nil, grpcErr))

	require.Error(t, err)
	assert.Nil(t, stream)
}
