// Package errors provides database error utilities
// and translation to gokit AppError types for consistent error handling.
package errors

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	apperrors "github.com/kbukum/gokit/errors"
)

// IsConnectionError checks if a database error is a connection error that might be resolved by retrying.
func IsConnectionError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	patterns := []string{
		"connection refused",
		"connection reset",
		"broken pipe",
		"i/o timeout",
		"no route to host",
		"network is unreachable",
		"connection closed",
		"connection lost",
		"driver: bad connection",
		"invalid connection",
	}
	for _, p := range patterns {
		if strings.Contains(errStr, p) {
			return true
		}
	}
	return false
}

// IsRetryableError determines if a database error should trigger a retry.
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if IsConnectionError(err) {
		return true
	}

	errStr := strings.ToLower(err.Error())
	patterns := []string{
		"deadlock",
		"lock timeout",
		"too many connections",
		"connection pool exhausted",
	}
	for _, p := range patterns {
		if strings.Contains(errStr, p) {
			return true
		}
	}
	return false
}

// IsNotFoundError checks if the error is a GORM record-not-found error.
func IsNotFoundError(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

// IsDuplicateError checks if the error is a GORM duplicate-key violation.
func IsDuplicateError(err error) bool {
	return errors.Is(err, gorm.ErrDuplicatedKey)
}

// FromDatabase converts a database error to an AppError. It translates GORM
// and database-specific errors to user-friendly messages.
func FromDatabase(err error, resource string) *apperrors.AppError {
	if err == nil {
		return nil
	}
	var appErr *apperrors.AppError
	if errors.As(err, &appErr) && appErr != nil {
		return appErr
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return apperrors.Normalize(err)
	}

	// Record not found
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperrors.NotFound(resource, "")
	}

	// Duplicate key violation
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return (&apperrors.AppError{
			Code:    apperrors.ErrCodeAlreadyExists,
			Message: fmt.Sprintf("A %s with these details already exists.", resource),

			Retryable: false,
		}).WithCause(err)
	}

	// Connection errors
	if IsConnectionError(err) {
		return (&apperrors.AppError{
			Code:    apperrors.ErrCodeDatabaseError,
			Message: "Database is temporarily unavailable. Please try again.",

			Retryable: true,
		}).WithCause(err)
	}

	// Retryable errors (deadlock, etc.)
	if IsRetryableError(err) {
		return (&apperrors.AppError{
			Code:    apperrors.ErrCodeDatabaseError,
			Message: "Database operation failed. Please try again.",

			Retryable: true,
		}).WithCause(err)
	}

	// Generic database error
	return apperrors.DatabaseError(err)
}
