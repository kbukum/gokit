// Package errors provides unified error handling for Go services.
// It implements structured error types with error codes, HTTP status mapping,
// and retryable detection following RFC 7807 and Google AIP-193.
package errors

import (
	"fmt"
	"net/http"
)

// AppError is the unified application error type.
type AppError struct {
	// Code is a machine-readable error code.
	Code ErrorCode `json:"code"`
	// Message is a human-readable error message.
	Message string `json:"message"`
	// Retryable indicates if the operation can be retried.
	Retryable bool `json:"retryable"`
	// HTTPStatus is the recommended HTTP status code for this error.
	HTTPStatus int `json:"-"`
	// Details contains additional context for the error.
	Details map[string]any `json:"details,omitempty"`
	// Cause is the underlying error that caused this error.
	Cause error `json:"-"`
}

// Error returns the string representation of the error.
func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (cause: %v)", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying cause of the error.
func (e *AppError) Unwrap() error { return e.Cause }

// WithCause sets the underlying cause of the error and returns the receiver.
func (e *AppError) WithCause(cause error) *AppError {
	e.Cause = cause
	return e
}

// WithDetails merges the provided details into the error and returns the receiver.
func (e *AppError) WithDetails(details map[string]any) *AppError {
	if e.Details == nil {
		e.Details = make(map[string]any)
	}
	for k, v := range details {
		e.Details[k] = v
	}
	return e
}

// WithDetail sets a single detail key-value pair and returns the receiver.
func (e *AppError) WithDetail(key string, value any) *AppError {
	if e.Details == nil {
		e.Details = make(map[string]any)
	}
	e.Details[key] = value
	return e
}

// New creates a new AppError with automatic retryable detection.
func New(code ErrorCode, message string, httpStatus int) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		HTTPStatus: httpStatus,
		Retryable:  IsRetryableCode(code),
	}
}

// --- Common Error Constructors ---

// ServiceUnavailable creates a new AppError for a service that is temporarily unavailable.
func ServiceUnavailable(service string) *AppError {
	return &AppError{
		Code: ErrCodeServiceUnavailable, Message: fmt.Sprintf("The %s is temporarily unavailable. Please try again.", service),
		HTTPStatus: http.StatusServiceUnavailable, Retryable: true,
		Details: map[string]any{"service": service},
	}
}

// ConnectionFailed creates a new AppError for a failed connection to a service.
func ConnectionFailed(service string) *AppError {
	return &AppError{
		Code: ErrCodeConnectionFailed, Message: fmt.Sprintf("Unable to connect to %s. Please verify the service is running.", service),
		HTTPStatus: http.StatusServiceUnavailable, Retryable: true,
		Details: map[string]any{"service": service},
	}
}

// Timeout creates a new AppError for a request that timed out.
func Timeout(operation string) *AppError {
	return &AppError{
		Code: ErrCodeTimeout, Message: "The request took too long. Please try again.",
		HTTPStatus: http.StatusGatewayTimeout, Retryable: true,
		Details: map[string]any{"operation": operation},
	}
}

// RateLimited creates a new AppError for too many requests.
func RateLimited() *AppError {
	return &AppError{
		Code: ErrCodeRateLimited, Message: "Too many requests. Please wait a moment and try again.",
		HTTPStatus: http.StatusTooManyRequests, Retryable: true,
	}
}

// NotFound creates a new AppError for a resource that was not found.
func NotFound(resource, id string) *AppError {
	details := map[string]any{"resource": resource}
	if id != "" {
		details["id"] = id
	}
	return &AppError{
		Code: ErrCodeNotFound, Message: fmt.Sprintf("The requested %s was not found.", resource),
		HTTPStatus: http.StatusNotFound, Retryable: false, Details: details,
	}
}

// AlreadyExists creates a new AppError for a resource that already exists.
func AlreadyExists(resource string) *AppError {
	return &AppError{
		Code: ErrCodeAlreadyExists, Message: fmt.Sprintf("A %s with these details already exists.", resource),
		HTTPStatus: http.StatusConflict, Retryable: false,
		Details: map[string]any{"resource": resource},
	}
}

// Conflict creates a new AppError for a conflict with the current state of the resource.
func Conflict(reason string) *AppError {
	return &AppError{
		Code: ErrCodeConflict, Message: reason,
		HTTPStatus: http.StatusConflict, Retryable: false,
	}
}

// InvalidInput creates a new AppError for invalid input.
func InvalidInput(field, reason string) *AppError {
	details := make(map[string]any)
	if field != "" {
		details["field"] = field
	}
	return &AppError{
		Code: ErrCodeInvalidInput, Message: fmt.Sprintf("Invalid input: %s", reason),
		HTTPStatus: http.StatusBadRequest, Retryable: false, Details: details,
	}
}

// Validation creates a new AppError for validation errors.
func Validation(message string) *AppError {
	return &AppError{
		Code: ErrCodeInvalidInput, Message: message,
		HTTPStatus: http.StatusBadRequest, Retryable: false,
	}
}

// MissingField creates a new AppError for a missing required field.
func MissingField(field string) *AppError {
	return &AppError{
		Code: ErrCodeMissingField, Message: fmt.Sprintf("Missing required field: %s", field),
		HTTPStatus: http.StatusBadRequest, Retryable: false,
		Details: map[string]any{"field": field},
	}
}

// InvalidFormat creates a new AppError for an invalid field format.
func InvalidFormat(field, expectedFormat string) *AppError {
	return &AppError{
		Code: ErrCodeInvalidFormat, Message: fmt.Sprintf("Invalid format for %s. Expected: %s", field, expectedFormat),
		HTTPStatus: http.StatusBadRequest, Retryable: false,
		Details: map[string]any{"field": field, "expected_format": expectedFormat},
	}
}

// Unauthorized creates a new AppError for unauthorized access.
func Unauthorized(reason string) *AppError {
	if reason == "" {
		reason = "Authentication required."
	}
	return &AppError{
		Code: ErrCodeUnauthorized, Message: reason,
		HTTPStatus: http.StatusUnauthorized, Retryable: false,
	}
}

// Forbidden creates a new AppError for forbidden access.
func Forbidden(reason string) *AppError {
	if reason == "" {
		reason = "You don't have permission to perform this action."
	}
	return &AppError{
		Code: ErrCodeForbidden, Message: reason,
		HTTPStatus: http.StatusForbidden, Retryable: false,
	}
}

// TokenExpired creates a new AppError for an expired authentication token.
func TokenExpired() *AppError {
	return &AppError{
		Code: ErrCodeTokenExpired, Message: "Your session has expired. Please log in again.",
		HTTPStatus: http.StatusUnauthorized, Retryable: false,
	}
}

// InvalidToken creates a new AppError for an invalid authentication token.
func InvalidToken() *AppError {
	return &AppError{
		Code: ErrCodeInvalidToken, Message: "Invalid authentication token. Please log in again.",
		HTTPStatus: http.StatusUnauthorized, Retryable: false,
	}
}

// Internal creates a new AppError for an internal server error.
func Internal(cause error) *AppError {
	return &AppError{
		Code: ErrCodeInternal, Message: "An unexpected error occurred. Please try again or contact support.",
		HTTPStatus: http.StatusInternalServerError, Retryable: false, Cause: cause,
	}
}

// DatabaseError creates a new AppError for a database error.
func DatabaseError(cause error) *AppError {
	return &AppError{
		Code: ErrCodeDatabaseError, Message: "A database error occurred. Please try again.",
		HTTPStatus: http.StatusInternalServerError, Retryable: true, Cause: cause,
	}
}

// ExternalServiceError creates a new AppError for an error from an external service.
func ExternalServiceError(service string, cause error) *AppError {
	return &AppError{
		Code: ErrCodeExternalService, Message: fmt.Sprintf("The %s service encountered an error. Please try again.", service),
		HTTPStatus: http.StatusBadGateway, Retryable: true,
		Details: map[string]any{"service": service}, Cause: cause,
	}
}
