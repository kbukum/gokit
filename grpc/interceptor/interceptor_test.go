package interceptor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/kbukum/gokit/logger"
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

func testLogger() *logger.Logger {
	return logger.NewDefault("test")
}

func mockInvoker(retErr error) grpc.UnaryInvoker {
	return func(ctx context.Context, method string, req, reply interface{},
		cc *grpc.ClientConn, opts ...grpc.CallOption,
	) error {
		return retErr
	}
}

func deadlineCapturingInvoker(captured *time.Time) grpc.UnaryInvoker {
	return func(ctx context.Context, method string, req, reply interface{},
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
// UnaryClientTimeoutInterceptor
// ---------------------------------------------------------------------------

func TestUnaryTimeoutInterceptor_AppliesTimeout(t *testing.T) {
	t.Parallel()

	interceptor := UnaryClientTimeoutInterceptor(500 * time.Millisecond)
	cc := testConn(t)

	var captured time.Time
	err := interceptor(context.Background(), "/pkg.Svc/Method", nil, nil,
		cc, deadlineCapturingInvoker(&captured))

	require.NoError(t, err)
	assert.False(t, captured.IsZero(), "deadline should have been set")
	assert.WithinDuration(t, time.Now().Add(500*time.Millisecond), captured, 100*time.Millisecond)
}

func TestUnaryTimeoutInterceptor_PreservesExistingDeadline(t *testing.T) {
	t.Parallel()

	interceptor := UnaryClientTimeoutInterceptor(500 * time.Millisecond)
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

func TestUnaryTimeoutInterceptor_ZeroTimeout(t *testing.T) {
	t.Parallel()

	interceptor := UnaryClientTimeoutInterceptor(0)
	cc := testConn(t)

	var captured time.Time
	err := interceptor(context.Background(), "/pkg.Svc/Method", nil, nil,
		cc, deadlineCapturingInvoker(&captured))

	require.NoError(t, err)
	assert.True(t, captured.IsZero(), "zero timeout should not set a deadline")
}

func TestUnaryTimeoutInterceptor_PropagatesInvokerError(t *testing.T) {
	t.Parallel()

	interceptor := UnaryClientTimeoutInterceptor(time.Second)
	cc := testConn(t)

	wantErr := status.Error(codes.Internal, "boom")
	err := interceptor(context.Background(), "/pkg.Svc/Method", nil, nil,
		cc, mockInvoker(wantErr))

	assert.Equal(t, wantErr, err)
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

// ---------------------------------------------------------------------------
// ErrorMapper
// ---------------------------------------------------------------------------

func TestErrorMapper_NilError(t *testing.T) {
	t.Parallel()
	msg, retry := ErrorMapper(nil)
	assert.Empty(t, msg)
	assert.False(t, retry)
}

func TestErrorMapper_NonStatusError(t *testing.T) {
	t.Parallel()
	msg, retry := ErrorMapper(errors.New("plain error"))
	assert.Contains(t, msg, "unexpected error")
	assert.True(t, retry)
}

func TestErrorMapper_AllCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code      codes.Code
		wantRetry bool
		wantSub   string
	}{
		{codes.Unavailable, true, "temporarily unavailable"},
		{codes.DeadlineExceeded, true, "took too long"},
		{codes.NotFound, false, "not found"},
		{codes.InvalidArgument, false, "Invalid request"},
		{codes.PermissionDenied, false, "permission"},
		{codes.Unauthenticated, false, "Authentication required"},
		{codes.ResourceExhausted, true, "Too many requests"},
		{codes.Internal, true, "internal error"},
		{codes.Canceled, false, "canceled"},
		{codes.Aborted, true, "aborted"},
		{codes.DataLoss, true, "unexpected"}, // default/unknown case
	}

	for _, tc := range tests {
		t.Run(tc.code.String(), func(t *testing.T) {
			t.Parallel()
			grpcErr := status.Error(tc.code, "test")
			msg, retry := ErrorMapper(grpcErr)

			assert.Contains(t, msg, tc.wantSub, "message substring")
			assert.Equal(t, tc.wantRetry, retry, "retryable")
		})
	}
}

// ---------------------------------------------------------------------------
// IsRetryableCode / IsRetryable
// ---------------------------------------------------------------------------

func TestIsRetryableCode_Retryable(t *testing.T) {
	t.Parallel()

	retryable := []codes.Code{
		codes.Unavailable,
		codes.DeadlineExceeded,
		codes.ResourceExhausted,
		codes.Aborted,
	}
	for _, c := range retryable {
		t.Run(c.String(), func(t *testing.T) {
			t.Parallel()
			assert.True(t, IsRetryableCode(c))
		})
	}
}

func TestIsRetryableCode_NonRetryable(t *testing.T) {
	t.Parallel()

	nonRetryable := []codes.Code{
		codes.OK,
		codes.NotFound,
		codes.InvalidArgument,
		codes.PermissionDenied,
		codes.Unauthenticated,
		codes.Internal,
		codes.Canceled,
		codes.FailedPrecondition,
	}
	for _, c := range nonRetryable {
		t.Run(c.String(), func(t *testing.T) {
			t.Parallel()
			assert.False(t, IsRetryableCode(c))
		})
	}
}

func TestIsRetryable_NilError(t *testing.T) {
	t.Parallel()
	assert.False(t, IsRetryable(nil))
}

func TestIsRetryable_NonGRPCError(t *testing.T) {
	t.Parallel()
	assert.False(t, IsRetryable(errors.New("not a grpc error")))
}

func TestIsRetryable_RetryableGRPCError(t *testing.T) {
	t.Parallel()
	err := status.Error(codes.Unavailable, "down")
	assert.True(t, IsRetryable(err))
}

func TestIsRetryable_NonRetryableGRPCError(t *testing.T) {
	t.Parallel()
	err := status.Error(codes.NotFound, "gone")
	assert.False(t, IsRetryable(err))
}
