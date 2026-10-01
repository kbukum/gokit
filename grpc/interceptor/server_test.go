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
	"google.golang.org/grpc/status"

	apperrors "github.com/kbukum/gokit/errors"
	grpccfg "github.com/kbukum/gokit/grpc"
)

func TestDecodedRemoteFailureIsNotForwarded(t *testing.T) {
	t.Parallel()
	remote, err := grpccfg.DecodeError(status.Error(codes.Internal, "upstream private detail"))
	require.NoError(t, err)
	handler := UnaryServerNormalizingInterceptor(nil)
	_, result := handler(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/svc/Call"}, func(context.Context, any) (any, error) {
		return nil, remote
	})
	require.Equal(t, codes.Internal, status.Code(result))
	require.NotContains(t, result.Error(), "upstream private detail")
}

func TestDeadlineClampsLongerClientBudget(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
	defer cancel()
	start := time.Now()
	_, err := UnaryServerDeadlineInterceptor(time.Hour)(ctx, nil, &grpc.UnaryServerInfo{}, func(callCtx context.Context, _ any) (any, error) {
		deadline, ok := callCtx.Deadline()
		require.True(t, ok)
		require.False(t, deadline.Before(start.Add(time.Hour)))
		require.False(t, deadline.After(time.Now().Add(time.Hour)))
		return struct{}{}, nil
	})
	require.NoError(t, err)
}

func TestUnaryServerLoggingInterceptor(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		handler  grpc.UnaryHandler
		wantErr  error
		wantResp any
	}{
		{
			name: "success",
			handler: func(context.Context, any) (any, error) {
				return "response", nil
			},
			wantResp: "response",
		},
		{
			name: "error",
			handler: func(context.Context, any) (any, error) {
				return nil, status.Error(codes.NotFound, "missing")
			},
			wantErr: status.Error(codes.NotFound, "missing"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			interceptor := UnaryServerLoggingInterceptor(testLogger())
			resp, err := interceptor(context.Background(), "request", &grpc.UnaryServerInfo{FullMethod: "/pkg.Service/Get"}, tc.handler)
			if tc.wantErr != nil {
				require.Error(t, err)
				assert.Equal(t, status.Code(tc.wantErr), status.Code(err))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantResp, resp)
		})
	}
}

func TestUnaryServerNormalizingInterceptor(t *testing.T) {
	t.Parallel()

	info := &grpc.UnaryServerInfo{FullMethod: "/gokit.test.v1.Service/Get"}

	t.Run("success passes response", func(t *testing.T) {
		t.Parallel()
		resp, err := UnaryServerNormalizingInterceptor(testLogger())(
			context.Background(), "request", info,
			func(context.Context, any) (any, error) { return "ok", nil },
		)
		require.NoError(t, err)
		assert.Equal(t, "ok", resp)
	})

	t.Run("app error maps to coded status with exact code", func(t *testing.T) {
		t.Parallel()
		_, err := UnaryServerNormalizingInterceptor(testLogger())(
			context.Background(), "request", info,
			func(context.Context, any) (any, error) { return nil, apperrors.NotFound("user", "123") },
		)
		require.Error(t, err)
		assert.Equal(t, codes.NotFound, status.Code(err))
		decoded, decodeErr := grpccfg.DecodeError(err)
		require.NoError(t, decodeErr)
		assert.Equal(t, apperrors.ErrCodeNotFound, decoded.Code)
	})

	t.Run("plain error is normalized, not leaked", func(t *testing.T) {
		t.Parallel()
		_, err := UnaryServerNormalizingInterceptor(testLogger())(
			context.Background(), "request", info,
			func(context.Context, any) (any, error) { return nil, errors.New("raw db dump secret") },
		)
		require.Error(t, err)
		assert.Equal(t, codes.Internal, status.Code(err))
		assert.NotContains(t, status.Convert(err).Message(), "secret")
	})

	t.Run("existing status error is left untouched", func(t *testing.T) {
		t.Parallel()
		sentinel := status.Error(codes.PermissionDenied, "denied")
		_, err := UnaryServerNormalizingInterceptor(testLogger())(
			context.Background(), "request", info,
			func(context.Context, any) (any, error) { return nil, sentinel },
		)
		require.Error(t, err)
		assert.Equal(t, sentinel, err)
	})

	t.Run("app error wrapping a status cause owns its own encoding", func(t *testing.T) {
		t.Parallel()
		wrapped := apperrors.Internal(status.Error(codes.Unavailable, "private backend diagnostic"))
		_, err := UnaryServerNormalizingInterceptor(testLogger())(
			context.Background(), "request", info,
			func(context.Context, any) (any, error) { return nil, wrapped },
		)
		require.Error(t, err)
		assert.Equal(t, codes.Internal, status.Code(err))
		decoded, decodeErr := grpccfg.DecodeError(err)
		require.NoError(t, decodeErr)
		assert.Equal(t, apperrors.ErrCodeInternal, decoded.Code)
		assert.NotContains(t, status.Convert(err).Message(), "private backend diagnostic")
	})
}

type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *fakeServerStream) Context() context.Context { return s.ctx }

func TestStreamServerNormalizingInterceptor(t *testing.T) {
	t.Parallel()

	info := &grpc.StreamServerInfo{FullMethod: "/gokit.test.v1.Service/Stream"}
	ss := &fakeServerStream{ctx: context.Background()}

	t.Run("app error maps to coded status", func(t *testing.T) {
		t.Parallel()
		err := StreamServerNormalizingInterceptor(testLogger())(
			nil, ss, info,
			func(any, grpc.ServerStream) error { return apperrors.ServiceUnavailable("db") },
		)
		require.Error(t, err)
		assert.Equal(t, codes.Unavailable, status.Code(err))
	})

	t.Run("success passes through", func(t *testing.T) {
		t.Parallel()
		err := StreamServerNormalizingInterceptor(testLogger())(
			nil, ss, info,
			func(any, grpc.ServerStream) error { return nil },
		)
		require.NoError(t, err)
	})
}

func TestUnaryServerDeadlineInterceptor(t *testing.T) {
	t.Parallel()

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Service/Get"}
	const noDeadline = "no-deadline"
	captureDeadline := func(ctx context.Context, _ any) (any, error) {
		if dl, ok := ctx.Deadline(); ok {
			return dl, nil
		}
		return noDeadline, nil
	}

	t.Run("applies max when client sends none", func(t *testing.T) {
		t.Parallel()
		resp, err := UnaryServerDeadlineInterceptor(200*time.Millisecond)(
			context.Background(), "request", info, captureDeadline)
		require.NoError(t, err)
		dl, ok := resp.(time.Time)
		require.True(t, ok, "expected a deadline to be set")
		assert.WithinDuration(t, time.Now().Add(200*time.Millisecond), dl, 100*time.Millisecond)
	})

	t.Run("preserves a tighter client deadline", func(t *testing.T) {
		t.Parallel()
		tight := time.Now().Add(50 * time.Millisecond)
		ctx, cancel := context.WithDeadline(context.Background(), tight)
		defer cancel()
		resp, err := UnaryServerDeadlineInterceptor(5*time.Second)(
			ctx, "request", info, captureDeadline)
		require.NoError(t, err)
		dl := resp.(time.Time)
		assert.Equal(t, tight.UnixNano(), dl.UnixNano())
	})

	t.Run("non-positive max disables the bound", func(t *testing.T) {
		t.Parallel()
		resp, err := UnaryServerDeadlineInterceptor(0)(
			context.Background(), "request", info, captureDeadline)
		require.NoError(t, err)
		assert.Equal(t, noDeadline, resp, "no deadline should be set")
	})
}
