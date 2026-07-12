package interceptor

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apperrors "github.com/kbukum/gokit/errors"
)

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

func TestUnaryServerErrorInterceptor(t *testing.T) {
	t.Parallel()

	plainErr := errors.New("plain")
	cases := []struct {
		name     string
		handler  grpc.UnaryHandler
		wantCode codes.Code
		wantResp any
		wantErr  error
	}{
		{
			name: "success passes response",
			handler: func(context.Context, any) (any, error) {
				return "ok", nil
			},
			wantResp: "ok",
		},
		{
			name: "app error maps to status",
			handler: func(context.Context, any) (any, error) {
				return "ignored", apperrors.NotFound("user", "123")
			},
			wantCode: codes.NotFound,
		},
		{
			name: "plain error passes through",
			handler: func(context.Context, any) (any, error) {
				return "partial", plainErr
			},
			wantResp: "partial",
			wantErr:  plainErr,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			resp, err := UnaryServerErrorInterceptor()(context.Background(), "request", &grpc.UnaryServerInfo{FullMethod: "/pkg.Service/Get"}, tc.handler)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				assert.Equal(t, tc.wantResp, resp)
				return
			}
			if tc.wantCode != codes.OK {
				require.Error(t, err)
				assert.Equal(t, tc.wantCode, status.Code(err))
				assert.Nil(t, resp)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantResp, resp)
		})
	}
}

func TestAppErrorToGRPCStatus(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		code apperrors.ErrorCode
		want codes.Code
	}{
		{name: "not found", code: apperrors.ErrCodeNotFound, want: codes.NotFound},
		{name: "already exists", code: apperrors.ErrCodeAlreadyExists, want: codes.AlreadyExists},
		{name: "invalid input", code: apperrors.ErrCodeInvalidInput, want: codes.InvalidArgument},
		{name: "missing field", code: apperrors.ErrCodeMissingField, want: codes.InvalidArgument},
		{name: "invalid format", code: apperrors.ErrCodeInvalidFormat, want: codes.InvalidArgument},
		{name: "unauthorized", code: apperrors.ErrCodeUnauthorized, want: codes.Unauthenticated},
		{name: "token expired", code: apperrors.ErrCodeTokenExpired, want: codes.Unauthenticated},
		{name: "invalid token", code: apperrors.ErrCodeInvalidToken, want: codes.Unauthenticated},
		{name: "forbidden", code: apperrors.ErrCodeForbidden, want: codes.PermissionDenied},
		{name: "conflict", code: apperrors.ErrCodeConflict, want: codes.FailedPrecondition},
		{name: "timeout", code: apperrors.ErrCodeTimeout, want: codes.DeadlineExceeded},
		{name: "rate limited", code: apperrors.ErrCodeRateLimited, want: codes.ResourceExhausted},
		{name: "service unavailable", code: apperrors.ErrCodeServiceUnavailable, want: codes.Unavailable},
		{name: "connection failed", code: apperrors.ErrCodeConnectionFailed, want: codes.Unavailable},
		{name: "database", code: apperrors.ErrCodeDatabaseError, want: codes.Internal},
		{name: "external", code: apperrors.ErrCodeExternalService, want: codes.Internal},
		{name: "internal", code: apperrors.ErrCodeInternal, want: codes.Internal},
		{name: "unknown", code: apperrors.ErrorCode("SOMETHING_ELSE"), want: codes.Internal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := appErrorToGRPCStatus(apperrors.New(tc.code, "mapped", 500))
			assert.Equal(t, tc.want, status.Code(err))
			assert.Equal(t, "mapped", status.Convert(err).Message())
		})
	}
}
