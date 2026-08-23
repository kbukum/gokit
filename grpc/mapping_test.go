package grpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apperrors "github.com/kbukum/gokit/errors"
)

func TestErrorCodeToGRPCCode_Canonical(t *testing.T) {
	t.Parallel()

	cases := []struct {
		code apperrors.ErrorCode
		want codes.Code
	}{
		{apperrors.ErrCodeServiceUnavailable, codes.Unavailable},
		{apperrors.ErrCodeConnectionFailed, codes.Unavailable},
		{apperrors.ErrCodeTimeout, codes.DeadlineExceeded},
		{apperrors.ErrCodeRateLimited, codes.ResourceExhausted},
		{apperrors.ErrCodeNotFound, codes.NotFound},
		{apperrors.ErrCodeAlreadyExists, codes.AlreadyExists},
		{apperrors.ErrCodeConflict, codes.Aborted},
		{apperrors.ErrCodeInvalidInput, codes.InvalidArgument},
		{apperrors.ErrCodeMissingField, codes.InvalidArgument},
		{apperrors.ErrCodeInvalidFormat, codes.InvalidArgument},
		{apperrors.ErrCodeUnauthorized, codes.Unauthenticated},
		{apperrors.ErrCodeTokenExpired, codes.Unauthenticated},
		{apperrors.ErrCodeInvalidToken, codes.Unauthenticated},
		{apperrors.ErrCodeForbidden, codes.PermissionDenied},
		{apperrors.ErrCodeInternal, codes.Internal},
		{apperrors.ErrCodeDatabaseError, codes.Internal},
		{apperrors.ErrCodeExternalService, codes.Internal},
		{apperrors.ErrorCode("DOES_NOT_EXIST"), codes.Unknown},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, ErrorCodeToGRPCCode(tc.code), "code %s", tc.code)
	}
}

func TestGRPCCodeToErrorCode_Canonical(t *testing.T) {
	t.Parallel()

	cases := []struct {
		code codes.Code
		want apperrors.ErrorCode
	}{
		{codes.Unavailable, apperrors.ErrCodeServiceUnavailable},
		{codes.DeadlineExceeded, apperrors.ErrCodeTimeout},
		{codes.ResourceExhausted, apperrors.ErrCodeRateLimited},
		{codes.NotFound, apperrors.ErrCodeNotFound},
		{codes.AlreadyExists, apperrors.ErrCodeAlreadyExists},
		{codes.Aborted, apperrors.ErrCodeConflict},
		{codes.FailedPrecondition, apperrors.ErrCodeConflict},
		{codes.InvalidArgument, apperrors.ErrCodeInvalidInput},
		{codes.OutOfRange, apperrors.ErrCodeInvalidInput},
		{codes.Unauthenticated, apperrors.ErrCodeUnauthorized},
		{codes.PermissionDenied, apperrors.ErrCodeForbidden},
		{codes.Internal, apperrors.ErrCodeInternal},
		{codes.Unknown, apperrors.ErrCodeInternal},
		{codes.Unimplemented, apperrors.ErrCodeInternal},
		{codes.DataLoss, apperrors.ErrCodeExternalService},
		{codes.OK, apperrors.ErrCodeExternalService},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, GRPCCodeToErrorCode(tc.code), "grpc code %s", tc.code)
	}
}

func TestAppErrorToStatus_NilAndCode(t *testing.T) {
	t.Parallel()

	assert.Nil(t, AppErrorToStatus(nil))

	st := AppErrorToStatus(&apperrors.AppError{Code: apperrors.ErrCodeNotFound, Message: "missing"})
	require.NotNil(t, st)
	assert.Equal(t, codes.NotFound, st.Code())
	assert.Equal(t, "missing", st.Message())
}

func TestStatusToAppError_Nil(t *testing.T) {
	t.Parallel()
	assert.Nil(t, StatusToAppError(nil))
}

func TestStatusToAppError_FromPlainStatus(t *testing.T) {
	t.Parallel()

	st := status.New(codes.Unauthenticated, "bad token")
	appErr := StatusToAppError(st)
	require.NotNil(t, appErr)
	assert.Equal(t, apperrors.ErrCodeUnauthorized, appErr.Code)
	assert.Equal(t, "bad token", appErr.Message)
}

func TestCanonicalRoundTrip_PreservesProblemDetails(t *testing.T) {
	t.Parallel()

	original := (&apperrors.AppError{
		Code:       apperrors.ErrCodeNotFound,
		Message:    "user 42 not found",
		HTTPStatus: 404,
		Retryable:  false,
	}).WithDetail("id", "42")

	st := AppErrorToStatus(original)
	require.NotNil(t, st)

	recovered := StatusToAppError(st)
	require.NotNil(t, recovered)
	assert.Equal(t, apperrors.ErrCodeNotFound, recovered.Code)
	assert.Equal(t, "user 42 not found", recovered.Message)
	assert.Equal(t, "42", recovered.Details["id"])
	assert.Equal(t, 404, recovered.HTTPStatus)
}

func TestCanonicalRoundTrip_AllCodes(t *testing.T) {
	t.Parallel()

	roundTrips := []apperrors.ErrorCode{
		apperrors.ErrCodeNotFound,
		apperrors.ErrCodeInvalidInput,
		apperrors.ErrCodeForbidden,
		apperrors.ErrCodeUnauthorized,
		apperrors.ErrCodeTimeout,
		apperrors.ErrCodeRateLimited,
		apperrors.ErrCodeAlreadyExists,
		apperrors.ErrCodeConflict,
		apperrors.ErrCodeServiceUnavailable,
	}
	for _, code := range roundTrips {
		t.Run(string(code), func(t *testing.T) {
			t.Parallel()
			st := AppErrorToStatus(&apperrors.AppError{Code: code, Message: "msg"})
			recovered := StatusToAppError(st)
			require.NotNil(t, recovered)
			assert.Equal(t, code, recovered.Code)
			assert.Equal(t, "msg", recovered.Message)
		})
	}
}
