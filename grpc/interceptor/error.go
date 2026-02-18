package interceptor

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorMapper maps gRPC status codes to user-friendly messages.
// It returns the message and whether the caller should retry.
func ErrorMapper(err error) (message string, retryable bool) {
	if err == nil {
		return "", false
	}

	st, ok := status.FromError(err)
	if !ok {
		return "An unexpected error occurred. Please try again.", true
	}

	switch st.Code() {
	case codes.Unavailable:
		return "The service is temporarily unavailable. Please try again in a moment.", true
	case codes.DeadlineExceeded:
		return "The request took too long to complete. Please try again.", true
	case codes.NotFound:
		return "The requested resource was not found.", false
	case codes.InvalidArgument:
		return "Invalid request. Please check your input and try again.", false
	case codes.PermissionDenied:
		return "You don't have permission to perform this action.", false
	case codes.Unauthenticated:
		return "Authentication required. Please log in again.", false
	case codes.ResourceExhausted:
		return "Too many requests. Please wait a moment and try again.", true
	case codes.Internal:
		return "An internal error occurred. Please try again or contact support.", true
	case codes.Canceled:
		return "The request was canceled.", false
	case codes.Aborted:
		return "The operation was aborted. Please try again.", true
	default:
		return "An unexpected error occurred. Please try again.", true
	}
}

// IsRetryableCode returns true if the given gRPC status code is typically retryable.
func IsRetryableCode(code codes.Code) bool {
	switch code {
	case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted, codes.Aborted:
		return true
	default:
		return false
	}
}

// IsRetryable returns true if the error represents a retryable gRPC failure.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	return IsRetryableCode(st.Code())
}
