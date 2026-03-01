package rest

import "github.com/kbukum/gokit/httpclient"

// REST error helpers delegate to httpclient's error classification.
// These are convenience re-exports so REST client users don't need
// to import httpclient directly for error checking.

// IsNotFound checks if the error is a 404 Not Found.
func IsNotFound(err error) bool { return httpclient.IsNotFound(err) }

// IsAuth checks if the error is a 401/403 authentication error.
func IsAuth(err error) bool { return httpclient.IsAuth(err) }

// IsRateLimit checks if the error is a 429 Too Many Requests.
func IsRateLimit(err error) bool { return httpclient.IsRateLimit(err) }

// IsServerError checks if the error is a 5xx server error.
func IsServerError(err error) bool { return httpclient.IsServerError(err) }

// IsRetryable checks if the error can be retried.
func IsRetryable(err error) bool { return httpclient.IsRetryable(err) }

// IsTimeout checks if the error is a timeout.
func IsTimeout(err error) bool { return httpclient.IsTimeout(err) }
