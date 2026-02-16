package grpc

import (
	"net/http"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apperrors "github.com/kbukum/gokit/errors"
)

// FromGRPC converts a gRPC error to an AppError.
// It translates gRPC status codes to user-friendly error messages.
func FromGRPC(err error, serviceName string) *apperrors.AppError {
	if err == nil {
		return nil
	}

	// Check connection errors first (before extracting status)
	if IsConnectionError(err) {
		return apperrors.ServiceUnavailable(serviceName).WithCause(err)
	}

	st, ok := status.FromError(err)
	if !ok {
		return apperrors.Internal(err)
	}

	switch st.Code() {
	case codes.Unavailable:
		return apperrors.ServiceUnavailable(serviceName).WithCause(err)

	case codes.DeadlineExceeded:
		if IsConnectionError(err) {
			return apperrors.ConnectionFailed(serviceName).WithCause(err)
		}
		return apperrors.Timeout(serviceName).WithCause(err)

	case codes.NotFound:
		return (&apperrors.AppError{
			Code:       apperrors.ErrCodeNotFound,
			Message:    "The requested resource was not found.",
			HTTPStatus: http.StatusNotFound,
			Retryable:  false,
		}).WithCause(err)

	case codes.InvalidArgument:
		return (&apperrors.AppError{
			Code:       apperrors.ErrCodeInvalidInput,
			Message:    sanitizeGRPCMessage(st.Message()),
			HTTPStatus: http.StatusBadRequest,
			Retryable:  false,
		}).WithCause(err)

	case codes.AlreadyExists:
		return (&apperrors.AppError{
			Code:       apperrors.ErrCodeAlreadyExists,
			Message:    "This resource already exists.",
			HTTPStatus: http.StatusConflict,
			Retryable:  false,
		}).WithCause(err)

	case codes.PermissionDenied:
		return apperrors.Forbidden("").WithCause(err)

	case codes.Unauthenticated:
		return (&apperrors.AppError{
			Code:       apperrors.ErrCodeUnauthorized,
			Message:    "Your session has expired. Please log in again.",
			HTTPStatus: http.StatusUnauthorized,
			Retryable:  false,
		}).WithCause(err)

	case codes.ResourceExhausted:
		return apperrors.RateLimited().WithCause(err)

	case codes.FailedPrecondition:
		return apperrors.Conflict(sanitizeGRPCMessage(st.Message())).WithCause(err)

	case codes.Aborted:
		return (&apperrors.AppError{
			Code:       apperrors.ErrCodeConflict,
			Message:    "The operation was aborted due to a conflict. Please try again.",
			HTTPStatus: http.StatusConflict,
			Retryable:  true,
		}).WithCause(err)

	case codes.Canceled:
		return (&apperrors.AppError{
			Code:       apperrors.ErrCodeInternal,
			Message:    "The request was cancelled.",
			HTTPStatus: http.StatusRequestTimeout,
			Retryable:  false,
		}).WithCause(err)

	case codes.Internal:
		return apperrors.Internal(err)

	default:
		return apperrors.Internal(err)
	}
}

// ToGRPCStatus converts an AppError to a gRPC status error.
func ToGRPCStatus(appErr *apperrors.AppError) error {
	if appErr == nil {
		return nil
	}

	var code codes.Code
	switch appErr.Code {
	case apperrors.ErrCodeNotFound:
		code = codes.NotFound
	case apperrors.ErrCodeAlreadyExists:
		code = codes.AlreadyExists
	case apperrors.ErrCodeInvalidInput, apperrors.ErrCodeMissingField, apperrors.ErrCodeInvalidFormat:
		code = codes.InvalidArgument
	case apperrors.ErrCodeUnauthorized, apperrors.ErrCodeTokenExpired, apperrors.ErrCodeInvalidToken:
		code = codes.Unauthenticated
	case apperrors.ErrCodeForbidden:
		code = codes.PermissionDenied
	case apperrors.ErrCodeConflict:
		code = codes.FailedPrecondition
	case apperrors.ErrCodeTimeout:
		code = codes.DeadlineExceeded
	case apperrors.ErrCodeRateLimited:
		code = codes.ResourceExhausted
	case apperrors.ErrCodeServiceUnavailable, apperrors.ErrCodeConnectionFailed:
		code = codes.Unavailable
	case apperrors.ErrCodeDatabaseError, apperrors.ErrCodeExternalService, apperrors.ErrCodeInternal:
		code = codes.Internal
	default:
		code = codes.Internal
	}

	return status.Error(code, appErr.Message)
}

// IsConnectionError checks if a gRPC error is a connection-level failure.
func IsConnectionError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	patterns := []string{
		"connection refused",
		"connection reset",
		"no such host",
		"transport is closing",
		"connection closed",
		"unavailable",
	}
	for _, p := range patterns {
		if strings.Contains(errStr, p) {
			return true
		}
	}
	return false
}

// sanitizeGRPCMessage removes potentially sensitive information from gRPC messages.
func sanitizeGRPCMessage(msg string) string {
	if msg == "" {
		return "Invalid input. Please check your request."
	}
	return "Invalid input: " + msg
}
