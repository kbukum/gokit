package connect

import (
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
	if asConnectError(err, &connectErr) {
		switch connectErr.Code() {
		case connectrpc.CodeUnavailable, connectrpc.CodeResourceExhausted, connectrpc.CodeAborted:
			return true
		}
	}

	msg := err.Error()
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "unavailable")
}

// asConnectError extracts a connect.Error from an error chain.
func asConnectError(err error, target **connectrpc.Error) bool {
	for err != nil {
		if ce, ok := err.(*connectrpc.Error); ok {
			*target = ce
			return true
		}
		if unwrapper, ok := err.(interface{ Unwrap() error }); ok {
			err = unwrapper.Unwrap()
		} else {
			return false
		}
	}
	return false
}
