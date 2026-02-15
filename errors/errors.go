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
	Code       ErrorCode              `json:"code"`
	Message    string                 `json:"message"`
	Retryable  bool                   `json:"retryable"`
	HTTPStatus int                    `json:"-"`
	Details    map[string]interface{} `json:"details,omitempty"`
	Cause      error                  `json:"-"`
}

func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (cause: %v)", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error { return e.Cause }

func (e *AppError) WithCause(cause error) *AppError {
	e.Cause = cause
	return e
}

func (e *AppError) WithDetails(details map[string]interface{}) *AppError {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	for k, v := range details {
		e.Details[k] = v
	}
	return e
}

func (e *AppError) WithDetail(key string, value interface{}) *AppError {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
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

func ServiceUnavailable(service string) *AppError {
	return &AppError{
		Code: ErrCodeServiceUnavailable, Message: fmt.Sprintf("The %s is temporarily unavailable. Please try again.", service),
		HTTPStatus: http.StatusServiceUnavailable, Retryable: true,
		Details: map[string]interface{}{"service": service},
	}
}

func ConnectionFailed(service string) *AppError {
	return &AppError{
		Code: ErrCodeConnectionFailed, Message: fmt.Sprintf("Unable to connect to %s. Please verify the service is running.", service),
		HTTPStatus: http.StatusServiceUnavailable, Retryable: true,
		Details: map[string]interface{}{"service": service},
	}
}

func Timeout(operation string) *AppError {
	return &AppError{
		Code: ErrCodeTimeout, Message: "The request took too long. Please try again.",
		HTTPStatus: http.StatusGatewayTimeout, Retryable: true,
		Details: map[string]interface{}{"operation": operation},
	}
}

func RateLimited() *AppError {
	return &AppError{
		Code: ErrCodeRateLimited, Message: "Too many requests. Please wait a moment and try again.",
		HTTPStatus: http.StatusTooManyRequests, Retryable: true,
	}
}

func NotFound(resource, id string) *AppError {
	details := map[string]interface{}{"resource": resource}
	if id != "" {
		details["id"] = id
	}
	return &AppError{
		Code: ErrCodeNotFound, Message: fmt.Sprintf("The requested %s was not found.", resource),
		HTTPStatus: http.StatusNotFound, Retryable: false, Details: details,
	}
}

func AlreadyExists(resource string) *AppError {
	return &AppError{
		Code: ErrCodeAlreadyExists, Message: fmt.Sprintf("A %s with these details already exists.", resource),
		HTTPStatus: http.StatusConflict, Retryable: false,
		Details: map[string]interface{}{"resource": resource},
	}
}

func Conflict(reason string) *AppError {
	return &AppError{
		Code: ErrCodeConflict, Message: reason,
		HTTPStatus: http.StatusConflict, Retryable: false,
	}
}

func InvalidInput(field, reason string) *AppError {
	details := make(map[string]interface{})
	if field != "" {
		details["field"] = field
	}
	return &AppError{
		Code: ErrCodeInvalidInput, Message: fmt.Sprintf("Invalid input: %s", reason),
		HTTPStatus: http.StatusBadRequest, Retryable: false, Details: details,
	}
}

func Validation(message string) *AppError {
	return &AppError{
		Code: ErrCodeInvalidInput, Message: message,
		HTTPStatus: http.StatusBadRequest, Retryable: false,
	}
}

func MissingField(field string) *AppError {
	return &AppError{
		Code: ErrCodeMissingField, Message: fmt.Sprintf("Missing required field: %s", field),
		HTTPStatus: http.StatusBadRequest, Retryable: false,
		Details: map[string]interface{}{"field": field},
	}
}

func InvalidFormat(field, expectedFormat string) *AppError {
	return &AppError{
		Code: ErrCodeInvalidFormat, Message: fmt.Sprintf("Invalid format for %s. Expected: %s", field, expectedFormat),
		HTTPStatus: http.StatusBadRequest, Retryable: false,
		Details: map[string]interface{}{"field": field, "expected_format": expectedFormat},
	}
}

func Unauthorized(reason string) *AppError {
	if reason == "" {
		reason = "Authentication required."
	}
	return &AppError{
		Code: ErrCodeUnauthorized, Message: reason,
		HTTPStatus: http.StatusUnauthorized, Retryable: false,
	}
}

func Forbidden(reason string) *AppError {
	if reason == "" {
		reason = "You don't have permission to perform this action."
	}
	return &AppError{
		Code: ErrCodeForbidden, Message: reason,
		HTTPStatus: http.StatusForbidden, Retryable: false,
	}
}

func TokenExpired() *AppError {
	return &AppError{
		Code: ErrCodeTokenExpired, Message: "Your session has expired. Please log in again.",
		HTTPStatus: http.StatusUnauthorized, Retryable: false,
	}
}

func InvalidToken() *AppError {
	return &AppError{
		Code: ErrCodeInvalidToken, Message: "Invalid authentication token. Please log in again.",
		HTTPStatus: http.StatusUnauthorized, Retryable: false,
	}
}

func Internal(cause error) *AppError {
	return &AppError{
		Code: ErrCodeInternal, Message: "An unexpected error occurred. Please try again or contact support.",
		HTTPStatus: http.StatusInternalServerError, Retryable: true, Cause: cause,
	}
}

func DatabaseError(cause error) *AppError {
	return &AppError{
		Code: ErrCodeDatabaseError, Message: "A database error occurred. Please try again.",
		HTTPStatus: http.StatusInternalServerError, Retryable: true, Cause: cause,
	}
}

func ExternalServiceError(service string, cause error) *AppError {
	return &AppError{
		Code: ErrCodeExternalService, Message: fmt.Sprintf("The %s service encountered an error. Please try again.", service),
		HTTPStatus: http.StatusBadGateway, Retryable: true,
		Details: map[string]interface{}{"service": service}, Cause: cause,
	}
}
