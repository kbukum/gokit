package storage

import (
	"net/http"
	"strings"

	apperrors "github.com/kbukum/gokit/errors"
)

// FromSupabase converts a Supabase error to an AppError.
// It parses Supabase error strings and translates them to user-friendly messages.
func FromSupabase(err error) *apperrors.AppError {
	if err == nil {
		return nil
	}

	errStr := strings.ToLower(err.Error())

	// Authentication errors
	if strings.Contains(errStr, "invalid login credentials") {
		return (&apperrors.AppError{
			Code:       apperrors.ErrCodeUnauthorized,
			Message:    "Invalid email or password.",
			HTTPStatus: http.StatusUnauthorized,
			Retryable:  false,
		}).WithCause(err)
	}

	if strings.Contains(errStr, "email not confirmed") {
		return (&apperrors.AppError{
			Code:       apperrors.ErrCodeUnauthorized,
			Message:    "Please verify your email address before logging in.",
			HTTPStatus: http.StatusUnauthorized,
			Retryable:  false,
		}).WithCause(err)
	}

	// Token errors
	if strings.Contains(errStr, "jwt expired") || strings.Contains(errStr, "token expired") {
		return apperrors.TokenExpired().WithCause(err)
	}

	if strings.Contains(errStr, "invalid jwt") || strings.Contains(errStr, "invalid token") {
		return apperrors.InvalidToken().WithCause(err)
	}

	// Rate limiting
	if strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "too many requests") {
		return apperrors.RateLimited().WithCause(err)
	}

	// User already exists
	if strings.Contains(errStr, "user already registered") {
		return (&apperrors.AppError{
			Code:       apperrors.ErrCodeAlreadyExists,
			Message:    "An account with this email already exists.",
			HTTPStatus: http.StatusConflict,
			Retryable:  false,
		}).WithCause(err)
	}

	// Password requirements
	if strings.Contains(errStr, "password") && strings.Contains(errStr, "weak") {
		return (&apperrors.AppError{
			Code:       apperrors.ErrCodeInvalidInput,
			Message:    "Password is too weak. Please choose a stronger password.",
			HTTPStatus: http.StatusBadRequest,
			Retryable:  false,
		}).WithCause(err)
	}

	// Email format
	if strings.Contains(errStr, "invalid email") {
		return (&apperrors.AppError{
			Code:       apperrors.ErrCodeInvalidFormat,
			Message:    "Please enter a valid email address.",
			HTTPStatus: http.StatusBadRequest,
			Retryable:  false,
		}).WithCause(err)
	}

	// Connection errors
	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "network") {
		return apperrors.ServiceUnavailable("authentication").WithCause(err)
	}

	// Not found errors
	if strings.Contains(errStr, "user not found") {
		return apperrors.NotFound("user", "").WithCause(err)
	}

	// Permission errors
	if strings.Contains(errStr, "permission denied") || strings.Contains(errStr, "not authorized") {
		return apperrors.Forbidden("").WithCause(err)
	}

	// Default: internal error
	return apperrors.Internal(err)
}
