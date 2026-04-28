package errors

import "google.golang.org/grpc/codes"

// ErrorCode represents a machine-readable error code.
type ErrorCode string

// Connection/Availability errors (retryable)
const (
	// ErrCodeServiceUnavailable indicates the service is temporarily unavailable.
	ErrCodeServiceUnavailable ErrorCode = "SERVICE_UNAVAILABLE"
	// ErrCodeConnectionFailed indicates a failed connection to a service.
	ErrCodeConnectionFailed ErrorCode = "CONNECTION_FAILED"
	// ErrCodeTimeout indicates the request timed out.
	ErrCodeTimeout ErrorCode = "TIMEOUT"
	// ErrCodeRateLimited indicates the client is rate limited.
	ErrCodeRateLimited ErrorCode = "RATE_LIMITED"
)

// Resource errors
const (
	// ErrCodeNotFound indicates the requested resource was not found.
	ErrCodeNotFound ErrorCode = "NOT_FOUND"
	// ErrCodeAlreadyExists indicates the resource already exists.
	ErrCodeAlreadyExists ErrorCode = "ALREADY_EXISTS"
	// ErrCodeConflict indicates a conflict with the current state of the resource.
	ErrCodeConflict ErrorCode = "CONFLICT"
)

// Validation errors
const (
	// ErrCodeInvalidInput indicates the input is invalid.
	ErrCodeInvalidInput ErrorCode = "INVALID_INPUT"
	// ErrCodeMissingField indicates a required field is missing.
	ErrCodeMissingField ErrorCode = "MISSING_FIELD"
	// ErrCodeInvalidFormat indicates a field has an invalid format.
	ErrCodeInvalidFormat ErrorCode = "INVALID_FORMAT"
)

// Authentication/Authorization errors
const (
	// ErrCodeUnauthorized indicates the request is unauthorized.
	ErrCodeUnauthorized ErrorCode = "UNAUTHORIZED"
	// ErrCodeForbidden indicates the request is forbidden.
	ErrCodeForbidden ErrorCode = "FORBIDDEN"
	// ErrCodeTokenExpired indicates the authentication token has expired.
	ErrCodeTokenExpired ErrorCode = "TOKEN_EXPIRED"
	// ErrCodeInvalidToken indicates the authentication token is invalid.
	ErrCodeInvalidToken ErrorCode = "INVALID_TOKEN"
)

// Internal errors
const (
	// ErrCodeInternal indicates an internal server error.
	ErrCodeInternal ErrorCode = "INTERNAL_ERROR"
	// ErrCodeDatabaseError indicates a database error.
	ErrCodeDatabaseError ErrorCode = "DATABASE_ERROR"
	// ErrCodeExternalService indicates an error from an external service.
	ErrCodeExternalService ErrorCode = "EXTERNAL_SERVICE_ERROR"
)

// Lifecycle errors (not retryable, not a server fault)
const (
	// ErrCodeCanceled indicates the operation was canceled by the caller or system
	// before completion (e.g., context cancellation, client disconnect).
	ErrCodeCanceled ErrorCode = "CANCELED"
)

var retryableCodes = map[ErrorCode]bool{
	ErrCodeServiceUnavailable: true,
	ErrCodeConnectionFailed:   true,
	ErrCodeTimeout:            true,
	ErrCodeRateLimited:        true,
	ErrCodeExternalService:    true,
	ErrCodeInternal:           false,
	ErrCodeCanceled:           false,
}

// IsRetryableCode returns true if the error code indicates a retryable error.
func IsRetryableCode(code ErrorCode) bool {
	return retryableCodes[code]
}

// GRPCCode returns the gRPC status code corresponding to this error code.
func (c ErrorCode) GRPCCode() codes.Code {
	if code, ok := grpcCodeMap[c]; ok {
		return code
	}
	return codes.Internal
}

var grpcCodeMap = map[ErrorCode]codes.Code{
	ErrCodeNotFound:           codes.NotFound,
	ErrCodeAlreadyExists:      codes.AlreadyExists,
	ErrCodeInvalidInput:       codes.InvalidArgument,
	ErrCodeMissingField:       codes.InvalidArgument,
	ErrCodeInvalidFormat:      codes.InvalidArgument,
	ErrCodeUnauthorized:       codes.Unauthenticated,
	ErrCodeTokenExpired:       codes.Unauthenticated,
	ErrCodeInvalidToken:       codes.Unauthenticated,
	ErrCodeForbidden:          codes.PermissionDenied,
	ErrCodeConflict:           codes.Aborted,
	ErrCodeTimeout:            codes.DeadlineExceeded,
	ErrCodeRateLimited:        codes.ResourceExhausted,
	ErrCodeServiceUnavailable: codes.Unavailable,
	ErrCodeConnectionFailed:   codes.Unavailable,
	ErrCodeInternal:           codes.Internal,
	ErrCodeDatabaseError:      codes.Internal,
	ErrCodeExternalService:    codes.Internal,
	ErrCodeCanceled:           codes.Canceled,
}
