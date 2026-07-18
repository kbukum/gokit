package httpclient

import (
	"errors"
	"fmt"
)

// ErrorCode classifies HTTP client errors.
type ErrorCode int

const (
	// ErrCodeTimeout indicates a request or connection timeout.
	ErrCodeTimeout ErrorCode = iota
	// ErrCodeConnection indicates a connection failure (refused, DNS, etc).
	ErrCodeConnection
	// ErrCodeAuth indicates an authentication/authorization failure (401/403).
	ErrCodeAuth
	// ErrCodeNotFound indicates the resource was not found (404).
	ErrCodeNotFound
	// ErrCodeRateLimit indicates rate limiting (429).
	ErrCodeRateLimit
	// ErrCodeValidation indicates a client-side validation error (400).
	ErrCodeValidation
	// ErrCodeServer indicates a server-side error (5xx).
	ErrCodeServer
)

// String returns the error code name.
func (c ErrorCode) String() string {
	switch c {
	case ErrCodeTimeout:
		return "timeout"
	case ErrCodeConnection:
		return "connection"
	case ErrCodeAuth:
		return "auth"
	case ErrCodeNotFound:
		return "not_found"
	case ErrCodeRateLimit:
		return "rate_limit"
	case ErrCodeValidation:
		return "validation"
	case ErrCodeServer:
		return "server"
	default:
		return "unknown"
	}
}

// Error is a structured HTTP client error with classification.
type Error struct {
	// StatusCode is the HTTP status code (0 for connection-level errors).
	StatusCode int
	// Code classifies the error.
	Code ErrorCode
	// Message describes the error.
	Message string
	// Retryable indicates whether the operation can be retried.
	Retryable bool
	// Body is the original response body (may be nil).
	Body []byte
	// Err is the underlying error.
	Err error
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("httpclient: %s (HTTP %d): %s", e.Code, e.StatusCode, e.Message)
	}
	return fmt.Sprintf("httpclient: %s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying error.
func (e *Error) Unwrap() error {
	return e.Err
}

// NewTimeoutError creates a timeout error.
func NewTimeoutError(err error) *Error {
	return &Error{
		Code:      ErrCodeTimeout,
		Message:   err.Error(),
		Retryable: true,
		Err:       err,
	}
}

// NewConnectionError creates a connection error.
func NewConnectionError(err error) *Error {
	return &Error{
		Code:      ErrCodeConnection,
		Message:   err.Error(),
		Retryable: true,
		Err:       err,
	}
}

// NewAuthError creates an authentication error.
func NewAuthError(statusCode int, body []byte) *Error {
	return &Error{
		StatusCode: statusCode,
		Code:       ErrCodeAuth,
		Message:    fmt.Sprintf("HTTP %d", statusCode),
		Retryable:  false,
		Body:       body,
	}
}

// NewNotFoundError creates a not-found error.
func NewNotFoundError(body []byte) *Error {
	return &Error{
		StatusCode: 404,
		Code:       ErrCodeNotFound,
		Message:    "HTTP 404",
		Retryable:  false,
		Body:       body,
	}
}

// NewRateLimitError creates a rate-limit error.
func NewRateLimitError(body []byte) *Error {
	return &Error{
		StatusCode: 429,
		Code:       ErrCodeRateLimit,
		Message:    "HTTP 429",
		Retryable:  true,
		Body:       body,
	}
}

// NewValidationError creates a validation error.
func NewValidationError(msg string) *Error {
	return &Error{
		Code:      ErrCodeValidation,
		Message:   msg,
		Retryable: false,
	}
}

// NewServerError creates a server error.
func NewServerError(statusCode int, body []byte) *Error {
	return &Error{
		StatusCode: statusCode,
		Code:       ErrCodeServer,
		Message:    fmt.Sprintf("HTTP %d", statusCode),
		Retryable:  true,
		Body:       body,
	}
}

// ClassifyStatusCode converts an HTTP status code into a typed error.
// Returns nil for 2xx status codes.
func ClassifyStatusCode(statusCode int, body []byte) *Error {
	switch {
	case statusCode >= 200 && statusCode < 300:
		return nil
	case statusCode == 401 || statusCode == 403:
		return NewAuthError(statusCode, body)
	case statusCode == 404:
		return NewNotFoundError(body)
	case statusCode == 429:
		return NewRateLimitError(body)
	case statusCode >= 400 && statusCode < 500:
		return &Error{
			StatusCode: statusCode,
			Code:       ErrCodeValidation,
			Message:    fmt.Sprintf("HTTP %d", statusCode),
			Retryable:  false,
			Body:       body,
		}
	case statusCode >= 500:
		return NewServerError(statusCode, body)
	default:
		return &Error{
			StatusCode: statusCode,
			Code:       ErrCodeServer,
			Message:    fmt.Sprintf("HTTP %d", statusCode),
			Retryable:  false,
			Body:       body,
		}
	}
}

// IsTimeout checks if an error is a timeout error.
func IsTimeout(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == ErrCodeTimeout
}

// IsConnection checks if an error is a connection error.
func IsConnection(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == ErrCodeConnection
}

// IsAuth checks if an error is an authentication error.
func IsAuth(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == ErrCodeAuth
}

// IsNotFound checks if an error is a not-found error.
func IsNotFound(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == ErrCodeNotFound
}

// IsRateLimit checks if an error is a rate-limit error.
func IsRateLimit(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == ErrCodeRateLimit
}

// IsServerError checks if an error is a server error.
func IsServerError(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == ErrCodeServer
}

// IsRetryable checks if an error is retryable.
func IsRetryable(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Retryable
}
