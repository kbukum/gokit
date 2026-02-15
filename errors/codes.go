package errors

// ErrorCode represents a machine-readable error code.
type ErrorCode string

// Connection/Availability errors (retryable)
const (
	ErrCodeServiceUnavailable ErrorCode = "SERVICE_UNAVAILABLE"
	ErrCodeConnectionFailed   ErrorCode = "CONNECTION_FAILED"
	ErrCodeTimeout            ErrorCode = "TIMEOUT"
	ErrCodeRateLimited        ErrorCode = "RATE_LIMITED"
)

// Resource errors
const (
	ErrCodeNotFound      ErrorCode = "NOT_FOUND"
	ErrCodeAlreadyExists ErrorCode = "ALREADY_EXISTS"
	ErrCodeConflict      ErrorCode = "CONFLICT"
)

// Validation errors
const (
	ErrCodeInvalidInput  ErrorCode = "INVALID_INPUT"
	ErrCodeMissingField  ErrorCode = "MISSING_FIELD"
	ErrCodeInvalidFormat ErrorCode = "INVALID_FORMAT"
)

// Authentication/Authorization errors
const (
	ErrCodeUnauthorized ErrorCode = "UNAUTHORIZED"
	ErrCodeForbidden    ErrorCode = "FORBIDDEN"
	ErrCodeTokenExpired ErrorCode = "TOKEN_EXPIRED"
	ErrCodeInvalidToken ErrorCode = "INVALID_TOKEN"
)

// Internal errors
const (
	ErrCodeInternal        ErrorCode = "INTERNAL_ERROR"
	ErrCodeDatabaseError   ErrorCode = "DATABASE_ERROR"
	ErrCodeExternalService ErrorCode = "EXTERNAL_SERVICE_ERROR"
)

var retryableCodes = map[ErrorCode]bool{
	ErrCodeServiceUnavailable: true,
	ErrCodeConnectionFailed:   true,
	ErrCodeTimeout:            true,
	ErrCodeRateLimited:        true,
	ErrCodeDatabaseError:      true,
	ErrCodeExternalService:    true,
	ErrCodeInternal:           true,
}

// IsRetryableCode returns true if the error code indicates a retryable error.
func IsRetryableCode(code ErrorCode) bool {
	return retryableCodes[code]
}
