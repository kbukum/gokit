package grpc

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apperrors "github.com/kbukum/gokit/errors"
)

// ---------------------------------------------------------------------------
// FromGRPC — nil / connection-error fast paths
// ---------------------------------------------------------------------------

func TestFromGRPC_NilError(t *testing.T) {
	t.Parallel()
	assert.Nil(t, FromGRPC(nil, "svc"))
}

func TestFromGRPC_ConnectionError(t *testing.T) {
	t.Parallel()
	err := fmt.Errorf("dial tcp: connection refused")
	appErr := FromGRPC(err, "payments")

	assert.Equal(t, apperrors.ErrCodeServiceUnavailable, appErr.Code)
	assert.True(t, appErr.Retryable)
	require.Error(t, appErr.Cause)
}

func TestFromGRPC_NonStatusError(t *testing.T) {
	t.Parallel()
	// An error that is NOT a gRPC status and NOT a connection error
	err := errors.New("some random failure")
	appErr := FromGRPC(err, "svc")

	assert.Equal(t, apperrors.ErrCodeInternal, appErr.Code)
}

// ---------------------------------------------------------------------------
// FromGRPC — every gRPC status code
// ---------------------------------------------------------------------------

func TestFromGRPC_StatusCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		grpcCode     codes.Code
		grpcMsg      string
		wantCode     apperrors.ErrorCode
		wantHTTP     int
		wantRetry    bool
		wantContains string // substring in message (optional)
	}{
		{
			name:      "Unavailable",
			grpcCode:  codes.Unavailable,
			wantCode:  apperrors.ErrCodeServiceUnavailable,
			wantHTTP:  http.StatusServiceUnavailable,
			wantRetry: true,
		},
		{
			name:      "DeadlineExceeded",
			grpcCode:  codes.DeadlineExceeded,
			grpcMsg:   "context deadline exceeded",
			wantCode:  apperrors.ErrCodeTimeout,
			wantHTTP:  http.StatusGatewayTimeout,
			wantRetry: true,
		},
		{
			name:     "NotFound",
			grpcCode: codes.NotFound,
			wantCode: apperrors.ErrCodeNotFound,
			wantHTTP: http.StatusNotFound,
		},
		{
			name:         "InvalidArgument_WithMessage",
			grpcCode:     codes.InvalidArgument,
			grpcMsg:      "field 'email' is required",
			wantCode:     apperrors.ErrCodeInvalidInput,
			wantHTTP:     http.StatusBadRequest,
			wantContains: "Invalid input: field 'email' is required",
		},
		{
			name:         "InvalidArgument_EmptyMessage",
			grpcCode:     codes.InvalidArgument,
			grpcMsg:      "",
			wantCode:     apperrors.ErrCodeInvalidInput,
			wantHTTP:     http.StatusBadRequest,
			wantContains: "Invalid input. Please check your request.",
		},
		{
			name:     "AlreadyExists",
			grpcCode: codes.AlreadyExists,
			wantCode: apperrors.ErrCodeAlreadyExists,
			wantHTTP: http.StatusConflict,
		},
		{
			name:     "PermissionDenied",
			grpcCode: codes.PermissionDenied,
			wantCode: apperrors.ErrCodeForbidden,
			wantHTTP: http.StatusForbidden,
		},
		{
			name:     "Unauthenticated",
			grpcCode: codes.Unauthenticated,
			wantCode: apperrors.ErrCodeUnauthorized,
			wantHTTP: http.StatusUnauthorized,
		},
		{
			name:      "ResourceExhausted",
			grpcCode:  codes.ResourceExhausted,
			wantCode:  apperrors.ErrCodeRateLimited,
			wantHTTP:  http.StatusTooManyRequests,
			wantRetry: true,
		},
		{
			name:     "FailedPrecondition",
			grpcCode: codes.FailedPrecondition,
			grpcMsg:  "version mismatch",
			wantCode: apperrors.ErrCodeConflict,
			wantHTTP: http.StatusConflict,
		},
		{
			name:      "Aborted",
			grpcCode:  codes.Aborted,
			wantCode:  apperrors.ErrCodeConflict,
			wantHTTP:  http.StatusConflict,
			wantRetry: true,
		},
		{
			name:     "Canceled",
			grpcCode: codes.Canceled,
			wantCode: apperrors.ErrCodeInternal,
			wantHTTP: http.StatusRequestTimeout,
		},
		{
			name:     "Internal",
			grpcCode: codes.Internal,
			wantCode: apperrors.ErrCodeInternal,
			wantHTTP: http.StatusInternalServerError,
		},
		{
			name:     "Unknown_DefaultCase",
			grpcCode: codes.Unknown,
			wantCode: apperrors.ErrCodeInternal,
			wantHTTP: http.StatusInternalServerError,
		},
		{
			name:     "DataLoss_DefaultCase",
			grpcCode: codes.DataLoss,
			wantCode: apperrors.ErrCodeInternal,
			wantHTTP: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			grpcErr := status.Error(tc.grpcCode, tc.grpcMsg)
			appErr := FromGRPC(grpcErr, "test-svc")

			require.NotNil(t, appErr)
			assert.Equal(t, tc.wantCode, appErr.Code, "error code")
			assert.Equal(t, tc.wantHTTP, appErr.HTTPStatus, "HTTP status")
			assert.Equal(t, tc.wantRetry, appErr.Retryable, "retryable")
			require.Error(t, appErr.Cause, "cause should be set")

			if tc.wantContains != "" {
				assert.Contains(t, appErr.Message, tc.wantContains, "message content")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ToGRPCStatus
// ---------------------------------------------------------------------------

func TestToGRPCStatus_NilError(t *testing.T) {
	t.Parallel()
	require.NoError(t, ToGRPCStatus(nil))
}

func TestToGRPCStatus_AllAppErrorCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		appCode  apperrors.ErrorCode
		wantGRPC codes.Code
	}{
		{"NotFound", apperrors.ErrCodeNotFound, codes.NotFound},
		{"AlreadyExists", apperrors.ErrCodeAlreadyExists, codes.AlreadyExists},
		{"InvalidInput", apperrors.ErrCodeInvalidInput, codes.InvalidArgument},
		{"MissingField", apperrors.ErrCodeMissingField, codes.InvalidArgument},
		{"InvalidFormat", apperrors.ErrCodeInvalidFormat, codes.InvalidArgument},
		{"Unauthorized", apperrors.ErrCodeUnauthorized, codes.Unauthenticated},
		{"TokenExpired", apperrors.ErrCodeTokenExpired, codes.Unauthenticated},
		{"InvalidToken", apperrors.ErrCodeInvalidToken, codes.Unauthenticated},
		{"Forbidden", apperrors.ErrCodeForbidden, codes.PermissionDenied},
		{"Conflict", apperrors.ErrCodeConflict, codes.FailedPrecondition},
		{"Timeout", apperrors.ErrCodeTimeout, codes.DeadlineExceeded},
		{"RateLimited", apperrors.ErrCodeRateLimited, codes.ResourceExhausted},
		{"ServiceUnavailable", apperrors.ErrCodeServiceUnavailable, codes.Unavailable},
		{"ConnectionFailed", apperrors.ErrCodeConnectionFailed, codes.Unavailable},
		{"DatabaseError", apperrors.ErrCodeDatabaseError, codes.Internal},
		{"ExternalService", apperrors.ErrCodeExternalService, codes.Internal},
		{"Internal", apperrors.ErrCodeInternal, codes.Internal},
		{"UnknownCode_Default", apperrors.ErrorCode("DOES_NOT_EXIST"), codes.Internal},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			appErr := &apperrors.AppError{Code: tc.appCode, Message: "test msg"}
			grpcErr := ToGRPCStatus(appErr)

			require.Error(t, grpcErr)
			st, ok := status.FromError(grpcErr)
			require.True(t, ok, "should be a gRPC status")
			assert.Equal(t, tc.wantGRPC, st.Code(), "gRPC code")
			assert.Equal(t, "test msg", st.Message(), "message preserved")
		})
	}
}

// ---------------------------------------------------------------------------
// Round-trip: AppError → gRPC → AppError preserves error code
// ---------------------------------------------------------------------------

func TestFromGRPC_RoundTrip(t *testing.T) {
	t.Parallel()

	roundTrips := []struct {
		appCode  apperrors.ErrorCode
		wantCode apperrors.ErrorCode
	}{
		{apperrors.ErrCodeNotFound, apperrors.ErrCodeNotFound},
		{apperrors.ErrCodeInvalidInput, apperrors.ErrCodeInvalidInput},
		{apperrors.ErrCodeForbidden, apperrors.ErrCodeForbidden},
		{apperrors.ErrCodeTimeout, apperrors.ErrCodeTimeout},
		{apperrors.ErrCodeRateLimited, apperrors.ErrCodeRateLimited},
		{apperrors.ErrCodeAlreadyExists, apperrors.ErrCodeAlreadyExists},
	}

	for _, tc := range roundTrips {
		t.Run(string(tc.appCode), func(t *testing.T) {
			t.Parallel()
			original := &apperrors.AppError{Code: tc.appCode, Message: "round-trip test"}
			grpcErr := ToGRPCStatus(original)
			result := FromGRPC(grpcErr, "svc")

			require.NotNil(t, result)
			assert.Equal(t, tc.wantCode, result.Code, "code preserved after round-trip")
		})
	}
}

// ---------------------------------------------------------------------------
// IsConnectionError
// ---------------------------------------------------------------------------

func TestIsConnectionError_NilError(t *testing.T) {
	t.Parallel()
	assert.False(t, IsConnectionError(nil))
}

func TestIsConnectionError_AllPatterns(t *testing.T) {
	t.Parallel()

	patterns := []struct {
		name string
		msg  string
	}{
		{"ConnectionRefused", "dial tcp 127.0.0.1:50051: connection refused"},
		{"ConnectionReset", "read tcp: connection reset by peer"},
		{"NoSuchHost", "dial tcp: lookup badhost: no such host"},
		{"TransportClosing", "transport is closing"},
		{"ConnectionClosed", "connection closed before server preface received"},
		{"Unavailable", "the server is currently unavailable"},
		{"UpperCase", "CONNECTION REFUSED"},
		{"MixedCase", "Transport Is Closing"},
	}

	for _, tc := range patterns {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.True(t, IsConnectionError(errors.New(tc.msg)),
				"should detect %q as connection error", tc.msg)
		})
	}
}

func TestIsConnectionError_NonConnectionErrors(t *testing.T) {
	t.Parallel()

	nonConn := []string{
		"permission denied",
		"not found",
		"invalid argument",
		"deadline exceeded",
		"internal error",
		"",
	}

	for _, msg := range nonConn {
		t.Run(msg, func(t *testing.T) {
			t.Parallel()
			if msg == "" {
				assert.False(t, IsConnectionError(errors.New(msg)))
			} else {
				assert.False(t, IsConnectionError(errors.New(msg)),
					"%q should NOT be a connection error", msg)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// sanitizeGRPCMessage (tested indirectly via FromGRPC InvalidArgument)
// ---------------------------------------------------------------------------

func TestFromGRPC_InvalidArgument_SanitizesEmptyMessage(t *testing.T) {
	t.Parallel()
	grpcErr := status.Error(codes.InvalidArgument, "")
	appErr := FromGRPC(grpcErr, "svc")

	assert.Equal(t, "Invalid input. Please check your request.", appErr.Message,
		"empty gRPC message should produce generic sanitized message")
}

func TestFromGRPC_InvalidArgument_SanitizesNonEmptyMessage(t *testing.T) {
	t.Parallel()
	grpcErr := status.Error(codes.InvalidArgument, "name too long")
	appErr := FromGRPC(grpcErr, "svc")

	assert.Equal(t, "Invalid input: name too long", appErr.Message)
}

// ---------------------------------------------------------------------------
// Security: no credentials leaked in error messages
// ---------------------------------------------------------------------------

func TestFromGRPC_NoCredentialsInErrorMessage(t *testing.T) {
	t.Parallel()

	grpcErr := status.Error(codes.Unauthenticated, "Bearer token=abc123secret")
	appErr := FromGRPC(grpcErr, "auth-svc")

	assert.NotContains(t, appErr.Message, "abc123secret",
		"credentials must not leak into user-facing message")
	assert.Equal(t, apperrors.ErrCodeUnauthorized, appErr.Code)
}

func TestFromGRPC_PermissionDenied_GenericMessage(t *testing.T) {
	t.Parallel()

	grpcErr := status.Error(codes.PermissionDenied, "user admin@corp.com lacks role X")
	appErr := FromGRPC(grpcErr, "authz-svc")

	assert.NotContains(t, appErr.Message, "admin@corp.com",
		"internal details must not leak into user-facing message")
}
