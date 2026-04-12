package connect

import (
	"errors"
	"strings"

	connectrpc "connectrpc.com/connect"
)

// IsTransientError checks if a ConnectRPC error is transient and retryable.
// Returns true for ConnectRPC status codes that indicate temporary failures
// (Unavailable, ResourceExhausted, Aborted) and for common connection-level errors.
//
// Use this as a retry predicate:
//
//	retryCfg.RetryIf = connect.IsTransientError
func IsTransientError(err error) bool {
	if err == nil {
		return false
	}

	var connectErr *connectrpc.Error
	if errors.As(err, &connectErr) {
		switch connectErr.Code() {
		case connectrpc.CodeUnavailable, connectrpc.CodeResourceExhausted, connectrpc.CodeAborted:
			return true
		default:
			// other codes are not considered transient at the RPC level
		}
	}

	msg := err.Error()
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "unavailable")
}
