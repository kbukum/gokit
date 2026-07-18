// Package errors classifies driver-level database errors into portable categories.
//
// [FromDatabase] maps a raw driver error to a typed application error,
// and the Is* predicates ([IsConnectionError], [IsRetryableError], [IsNotFoundError], [IsDuplicateError]) let callers make retry
// and control-flow decisions without matching driver-specific error strings.
package errors
