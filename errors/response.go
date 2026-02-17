package errors

import (
	stderrors "errors"
)

// ErrorResponse is the JSON structure returned to clients following RFC 7807.
type ErrorResponse struct {
	// Error contains the error details.
	Error ErrorBody `json:"error"`
}

// ErrorBody contains the error details sent to clients.
type ErrorBody struct {
	// Code is a machine-readable error code.
	Code ErrorCode `json:"code"`
	// Message is a human-readable error message.
	Message string `json:"message"`
	// Retryable indicates if the operation can be retried.
	Retryable bool `json:"retryable"`
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
