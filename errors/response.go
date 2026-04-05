package errors

import (
	stderrors "errors"
	"strings"
)

// ErrorResponse is the JSON structure returned to clients following RFC 7807.
type ErrorResponse struct {
	// Error contains the error details.
	Error ErrorBody `json:"error"`
}

// ErrorBody contains the error details sent to clients.
type ErrorBody struct {
	// Code is a machine-readable error code.
	Code ErrorCode `json:"code" example:"NOT_FOUND"`
	// Message is a human-readable error message.
	Message string `json:"message" example:"Resource not found"`
	// Retryable indicates if the operation can be retried.
	Retryable bool `json:"retryable" example:"false"`
	// Details contains additional context for the error.
	Details map[string]any `json:"details,omitempty"`
}

// ToResponse converts an AppError to an ErrorResponse for JSON serialization.
func (e *AppError) ToResponse() ErrorResponse {
	return ErrorResponse{
		Error: ErrorBody{
			Code:      e.Code,
			Message:   e.Message,
			Retryable: e.Retryable,
			Details:   e.Details,
		},
	}
}

// RFC7807Response follows the RFC 7807 Problem Details for HTTP APIs format.
// Use this for standards-compliant error responses alongside the existing ErrorResponse.
type RFC7807Response struct {
	// Type is a URI identifying the problem type.
	Type string `json:"type"`
	// Title is a short human-readable summary.
	Title string `json:"title"`
	// Status is the HTTP status code.
	Status int `json:"status"`
	// Detail is a human-readable explanation.
	Detail string `json:"detail"`
	// Instance is an optional URI identifying the specific occurrence.
	Instance string `json:"instance,omitempty"`
	// Extensions contains additional context.
	Extensions map[string]string `json:"extensions,omitempty"`
}

// ToRFC7807 converts an AppError to an RFC 7807 Problem Details response.
func (e *AppError) ToRFC7807() RFC7807Response {
	kebab := strings.ToLower(strings.ReplaceAll(string(e.Code), "_", "-"))
	return RFC7807Response{
		Type:   "https://gokit.dev/errors/" + kebab,
		Title:  string(e.Code),
		Status: e.HTTPStatus,
		Detail: e.Message,
	}
}

// IsAppError checks if an error is an AppError.
func IsAppError(err error) bool {
	var appErr *AppError
	return stderrors.As(err, &appErr)
}

// AsAppError converts an error to an AppError if possible.
func AsAppError(err error) (*AppError, bool) {
	var appErr *AppError
	if stderrors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}
