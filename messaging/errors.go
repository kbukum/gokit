package messaging

import (
	"errors"
	"strings"
)

// ErrClosed is returned when a producer or consumer is used after Close.
var ErrClosed = errors.New("messaging: closed")

// ErrUnsupported is returned when an adapter cannot honor a requested messaging capability.
var ErrUnsupported = errors.New("messaging: unsupported capability")

// ErrorClassifier categorizes errors for retry/circuit-breaker decisions. Each broker implementation provides its own classification logic.
type ErrorClassifier interface {
	// IsConnectionError checks if the error is a connection-level error.
	IsConnectionError(err error) bool
	// IsRetryableError determines if the error should trigger a retry.
	IsRetryableError(err error) bool
}

// ConnectionPatterns contains generic connection error patterns common to most message brokers (TCP-level failures, DNS errors, etc.).
var ConnectionPatterns = []string{
	"connection refused",
	"connection reset",
	"broken pipe",
	"i/o timeout",
	"no route to host",
	"network is unreachable",
	"connection closed",
	"dial tcp",
}

// RetryablePatterns contains generic retryable error patterns that are not connection-specific but typically warrant a retry.
var RetryablePatterns = []string{
	"temporary",
	"request timed out",
}

// IsConnectionError checks if err matches any connection pattern. Default ConnectionPatterns are always checked; additional broker-specific patterns can be appended via the variadic argument.
func IsConnectionError(err error, extra ...string) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	for _, p := range ConnectionPatterns {
		if strings.Contains(errStr, p) {
			return true
		}
	}
	for _, p := range extra {
		if strings.Contains(errStr, p) {
			return true
		}
	}
	return false
}

// IsRetryableError checks if err should trigger a retry. Connection errors are always retryable. Additional broker-specific retryable patterns can be appended via the variadic argument.
func IsRetryableError(err error, extra ...string) bool {
	if err == nil {
		return false
	}
	if IsConnectionError(err) {
		return true
	}
	errStr := strings.ToLower(err.Error())
	for _, p := range RetryablePatterns {
		if strings.Contains(errStr, p) {
			return true
		}
	}
	for _, p := range extra {
		if strings.Contains(errStr, p) {
			return true
		}
	}
	return false
}
