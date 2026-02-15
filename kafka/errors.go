package kafka

import "strings"

// IsConnectionError checks if a Kafka error is a connection-level error.
func IsConnectionError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	connectionPatterns := []string{
		"connection refused",
		"connection reset",
		"broken pipe",
		"i/o timeout",
		"no route to host",
		"network is unreachable",
		"broker not available",
		"leader not available",
		"connection closed",
		"dial tcp",
		"network exception",
	}
	for _, p := range connectionPatterns {
		if strings.Contains(errStr, p) {
			return true
		}
	}
	return false
}

// IsRetryableError determines if a Kafka error should trigger a retry.
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if IsConnectionError(err) {
		return true
	}
	errStr := strings.ToLower(err.Error())
	retryablePatterns := []string{
		"temporary",
		"request timed out",
		"not enough replicas",
		"offset out of range",
	}
	for _, p := range retryablePatterns {
		if strings.Contains(errStr, p) {
			return true
		}
	}
	return false
}

// IsNonRetryableError checks if the error should not be retried.
func IsNonRetryableError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	nonRetryablePatterns := []string{
		"message too large",
		"invalid topic",
		"invalid partition",
		"unknown topic",
		"authorization failed",
	}
	for _, p := range nonRetryablePatterns {
		if strings.Contains(errStr, p) {
			return true
		}
	}
	return false
}
