package messaging

// ErrorClassifier categorizes errors for retry/circuit-breaker decisions.
// Each broker implementation provides its own classification logic.
type ErrorClassifier interface {
	// IsConnectionError checks if the error is a connection-level error.
	IsConnectionError(err error) bool
	// IsRetryableError determines if the error should trigger a retry.
	IsRetryableError(err error) bool
}
