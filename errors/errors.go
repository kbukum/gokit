// Package errors provides unified error handling for Go services.
// It implements structured error types with error codes, HTTP status mapping,
// and retryable detection following RFC 9457 and Google AIP-193.
package errors

import (
	"fmt"
	"time"
)

// AppError is the unified application error type.
type AppError struct {
	// Code is a machine-readable error code.
	Code ErrorCode `json:"code"`
	// Reason is an optional, stable domain reason a feature can branch on
	// (e.g. "GATE_BLOCKED"). It is finer-grained than Code and, unlike Message,
	// is not meant for display. Empty when the code alone is enough.
	Reason string `json:"reason,omitempty"`
	// Message is a human-readable, safe error message.
	Message string `json:"message"`
	// Violations lists field-level problems with canonical paths. It supersedes
	// stuffing field errors into Details.
	Violations []Violation `json:"violations,omitempty"`
	// Retryable is a transient-failure hint, not permission to repeat a non-idempotent operation.
	Retryable bool `json:"retryable"`
	// RetryAfter is an optional hint for how long to wait before retrying. It is
	// only meaningful when Retryable is true; zero means "no specific delay".
	RetryAfter time.Duration `json:"-"`
	// TraceID correlates this failure with server logs and traces. It is safe to
	// expose and never carries the underlying cause.
	TraceID string `json:"traceId,omitempty"`
	// Details carries RFC 9457 problem-detail extension members. These are, by definition,
	// arbitrary JSON and cannot be given a closed type without losing that openness,
	// so map[string]any is a deliberate,
	// documented opaque-value exception to the no-any rule.
	// Values should be JSON-encodable.
	Details map[string]any `json:"details,omitempty"`
	// Cause is the underlying error that caused this error.
	Cause error `json:"-"`
	// origin identifies the canonical sentinel this error was derived from via a
	// copy-on-write builder (WithCause/WithDetails/WithDetail). It lets errors.Is
	// keep matching the sentinel after enrichment without exposing shared mutable
	// state. It is unexported and never serialized.
	origin *AppError
}

// Error returns the string representation of the error.
func (e *AppError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (cause: %v)", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying cause of the error.
func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Is reports whether target is this error or the canonical sentinel it was
// derived from, so a shared sentinel still matches under errors.Is after
// copy-on-write enrichment via WithCause/WithDetails/WithDetail.
func (e *AppError) Is(target error) bool {
	t, ok := target.(*AppError)
	if !ok {
		return false
	}
	return e == t || (e != nil && t != nil && e.origin == t)
}

// clone returns a shallow copy of e with an independent Details map. The
// copy-on-write builders use it so they never mutate a shared receiver such as a
// package-global sentinel. The copy remembers the sentinel it derives from so
// errors.Is keeps matching the original after enrichment.
func (e *AppError) clone() *AppError {
	c := *e
	if e.Details != nil {
		c.Details = make(map[string]any, len(e.Details))
		for k, v := range e.Details {
			c.Details[k] = v
		}
	}
	if e.Violations != nil {
		c.Violations = make([]Violation, len(e.Violations))
		copy(c.Violations, e.Violations)
	}
	if c.origin == nil {
		c.origin = e
	}
	return &c
}

// WithCause returns a copy of the error with the underlying cause set. The
// receiver is not modified, so shared sentinels stay immutable and safe to
// enrich concurrently.
func (e *AppError) WithCause(cause error) *AppError {
	c := e.clone()
	c.Cause = cause
	return c
}

// WithDetails returns a copy of the error with the provided RFC 9457 extension
// members merged in. The receiver is not modified.
// The value type is a documented opaque-value exception (see AppError.Details);
// values should be JSON-encodable.
func (e *AppError) WithDetails(details map[string]any) *AppError {
	c := e.clone()
	if c.Details == nil {
		c.Details = make(map[string]any)
	}
	for k, v := range details {
		c.Details[k] = v
	}
	return c
}

// WithDetail returns a copy of the error with a single RFC 9457 extension member
// set. The receiver is not modified.
// The value type is a documented opaque-value exception (see AppError.Details);
// the value should be JSON-encodable.
func (e *AppError) WithDetail(key string, value any) *AppError {
	c := e.clone()
	if c.Details == nil {
		c.Details = make(map[string]any)
	}
	c.Details[key] = value
	return c
}

// WithReason returns a copy of the error with the domain reason set. The
// receiver is not modified.
func (e *AppError) WithReason(reason string) *AppError {
	c := e.clone()
	c.Reason = reason
	return c
}

// WithViolations returns a copy of the error with the given field violations
// set, replacing any existing ones. The receiver is not modified.
func (e *AppError) WithViolations(violations ...Violation) *AppError {
	c := e.clone()
	if len(violations) == 0 {
		c.Violations = nil
		return c
	}
	c.Violations = make([]Violation, len(violations))
	copy(c.Violations, violations)
	return c
}

// WithRetryAfter returns a copy of the error marked retryable with the given
// retry delay. A zero delay still marks the error retryable with no specific
// wait. The receiver is not modified.
func (e *AppError) WithRetryAfter(d time.Duration) *AppError {
	c := e.clone()
	c.Retryable = true
	c.RetryAfter = d
	return c
}

// WithRetryable overrides the transient-failure hint. Disabling it also clears the delay.
func (e *AppError) WithRetryable(retryable bool) *AppError {
	c := e.clone()
	c.Retryable = retryable
	if !retryable {
		c.RetryAfter = 0
	}
	return c
}

// HTTPStatus returns the REST status derived from Code. Protocols such as Connect own their HTTP mapping.
func (e *AppError) HTTPStatus() int { return HTTPStatusFor(e.Code) }

// WithTraceID returns a copy of the error with the trace id set. The receiver is
// not modified.
func (e *AppError) WithTraceID(traceID string) *AppError {
	c := e.clone()
	c.TraceID = traceID
	return c
}

// New creates an application error with the code's default transient-failure hint.
func New(code ErrorCode, message string) *AppError {
	s := specFor(code)
	return &AppError{
		Code:      code,
		Message:   message,
		Retryable: s.retryable,
	}
}

// --- Common Error Constructors ---

// ServiceUnavailable creates a new AppError for a service that is temporarily unavailable.
func ServiceUnavailable(service string) *AppError {
	e := New(ErrCodeServiceUnavailable, fmt.Sprintf("The %s is temporarily unavailable. Please try again.", service))
	e.Details = map[string]any{"service": service}
	return e
}

// ConnectionFailed creates a new AppError for a failed connection to a service.
func ConnectionFailed(service string) *AppError {
	e := New(ErrCodeConnectionFailed, fmt.Sprintf("Unable to connect to %s. Please verify the service is running.", service))
	e.Details = map[string]any{"service": service}
	return e
}

// Timeout creates a new AppError for a request that timed out.
func Timeout(operation string) *AppError {
	e := New(ErrCodeTimeout, "The request took too long. Please try again.")
	e.Details = map[string]any{"operation": operation}
	return e
}

// RateLimited creates a new AppError for too many requests.
func RateLimited() *AppError {
	return New(ErrCodeRateLimited, "Too many requests. Please wait a moment and try again.")
}

// NotFound creates a new AppError for a resource that was not found.
func NotFound(resource, id string) *AppError {
	details := map[string]any{"resource": resource}
	if id != "" {
		details["id"] = id
	}
	e := New(ErrCodeNotFound, fmt.Sprintf("The requested %s was not found.", resource))
	e.Details = details
	return e
}

// AlreadyExists creates a new AppError for a resource that already exists.
func AlreadyExists(resource string) *AppError {
	e := New(ErrCodeAlreadyExists, fmt.Sprintf("A %s with these details already exists.", resource))
	e.Details = map[string]any{"resource": resource}
	return e
}

// Conflict creates a new AppError for a conflict with the current state of the resource.
func Conflict(reason string) *AppError {
	return New(ErrCodeConflict, reason)
}

// InvalidInput creates a new AppError for invalid input.
func InvalidInput(field, reason string) *AppError {
	e := New(ErrCodeInvalidInput, fmt.Sprintf("Invalid input: %s", reason))
	if field != "" {
		e.Violations = []Violation{{Field: field, Reason: ViolationInvalidValue, Message: reason}}
	}
	return e
}

// Validation creates a new AppError for validation errors.
func Validation(message string) *AppError {
	return New(ErrCodeInvalidInput, message)
}

// MissingField creates a new AppError for a missing required field.
func MissingField(field string) *AppError {
	e := New(ErrCodeMissingField, fmt.Sprintf("Missing required field: %s", field))
	e.Violations = []Violation{{Field: field, Reason: ViolationRequired, Message: "is required"}}
	return e
}

// InvalidFormat creates a new AppError for an invalid field format.
func InvalidFormat(field, expectedFormat string) *AppError {
	e := New(ErrCodeInvalidFormat, fmt.Sprintf("Invalid format for %s. Expected: %s", field, expectedFormat))
	e.Violations = []Violation{{Field: field, Reason: ViolationInvalidFormat, Message: "expected " + expectedFormat}}
	return e
}

// Unauthorized creates a new AppError for unauthorized access.
func Unauthorized(reason string) *AppError {
	if reason == "" {
		reason = "Authentication required."
	}
	return New(ErrCodeUnauthorized, reason)
}

// Forbidden creates a new AppError for forbidden access.
func Forbidden(reason string) *AppError {
	if reason == "" {
		reason = "You don't have permission to perform this action."
	}
	return New(ErrCodeForbidden, reason)
}

// TokenExpired creates a new AppError for an expired authentication token.
func TokenExpired() *AppError {
	return New(ErrCodeTokenExpired, "Your session has expired. Please log in again.")
}

// InvalidToken creates a new AppError for an invalid authentication token.
func InvalidToken() *AppError {
	return New(ErrCodeInvalidToken, "Invalid authentication token. Please log in again.")
}

// Internal creates a new AppError for an internal server error.
func Internal(cause error) *AppError {
	e := New(ErrCodeInternal, "An unexpected error occurred. Please try again or contact support.")
	e.Cause = cause
	return e
}

// DatabaseError creates a new AppError for a database error.
func DatabaseError(cause error) *AppError {
	e := New(ErrCodeDatabaseError, "A database error occurred.")
	e.Cause = cause
	return e
}

// ExternalServiceError creates a new AppError for an error from an external service.
func ExternalServiceError(service string, cause error) *AppError {
	e := New(ErrCodeExternalService, fmt.Sprintf("The %s service encountered an error. Please try again.", service))
	e.Details = map[string]any{"service": service}
	e.Cause = cause
	return e
}

// Canceled creates a new AppError for an operation that was canceled.
func Canceled(operation string) *AppError {
	e := New(ErrCodeCanceled, fmt.Sprintf("The %s operation was canceled.", operation))
	e.Details = map[string]any{"operation": operation}
	return e
}

// Wrap converts a standard error to an AppError.
// If the error is already an AppError it is returned as-is; otherwise it is wrapped as Internal.
// Returns nil when err is nil.
func Wrap(err error) *AppError {
	if err == nil {
		return nil
	}
	if appErr, ok := AsAppError(err); ok {
		return appErr
	}
	return Internal(err)
}

// FormatResourceError creates a not-found error with a formatted identifier of any type.
// The identifier is rendered with the default format verb.
func FormatResourceError[T any](resource string, id T) *AppError {
	return NotFound(resource, fmt.Sprintf("%v", id))
}
