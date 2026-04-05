package errors

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"gorm.io/gorm"

	apperrors "github.com/kbukum/gokit/errors"
)

// --- IsConnectionError ---

func TestIsConnectionError_Nil(t *testing.T) {
	if IsConnectionError(nil) {
		t.Error("expected false for nil error")
	}
}

func TestIsConnectionError_AllPatterns(t *testing.T) {
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
		t.Run(p, func(t *testing.T) {
			err := fmt.Errorf("db: %s by peer", p)
			if !IsConnectionError(err) {
				t.Errorf("expected true for pattern %q", p)
			}
		})
	}
}

func TestIsConnectionError_CaseInsensitive(t *testing.T) {
	err := fmt.Errorf("CONNECTION REFUSED")
	if !IsConnectionError(err) {
		t.Error("expected case-insensitive match")
	}
}

func TestIsConnectionError_NonMatching(t *testing.T) {
	err := fmt.Errorf("unique constraint violation")
	if IsConnectionError(err) {
		t.Error("expected false for non-connection error")
	}
}

// --- IsRetryableError ---

func TestIsRetryableError_Nil(t *testing.T) {
	if IsRetryableError(nil) {
		t.Error("expected false for nil error")
	}
}

func TestIsRetryableError_ConnectionErrors(t *testing.T) {
	err := fmt.Errorf("connection refused")
	if !IsRetryableError(err) {
		t.Error("connection errors should be retryable")
	}
}

func TestIsRetryableError_RetryPatterns(t *testing.T) {
	patterns := []string{
		"deadlock detected",
		"lock timeout exceeded",
		"too many connections",
		"connection pool exhausted",
	}
	for _, p := range patterns {
		t.Run(p, func(t *testing.T) {
			err := fmt.Errorf("db error: %s", p)
			if !IsRetryableError(err) {
				t.Errorf("expected true for pattern %q", p)
			}
		})
	}
}

func TestIsRetryableError_NonRetryable(t *testing.T) {
	err := fmt.Errorf("syntax error in SQL")
	if IsRetryableError(err) {
		t.Error("expected false for non-retryable error")
	}
}

// --- IsNotFoundError ---

func TestIsNotFoundError_RecordNotFound(t *testing.T) {
	if !IsNotFoundError(gorm.ErrRecordNotFound) {
		t.Error("expected true for gorm.ErrRecordNotFound")
	}
}

func TestIsNotFoundError_WrappedRecordNotFound(t *testing.T) {
	wrapped := fmt.Errorf("lookup failed: %w", gorm.ErrRecordNotFound)
	if !IsNotFoundError(wrapped) {
		t.Error("expected true for wrapped gorm.ErrRecordNotFound")
	}
}

func TestIsNotFoundError_OtherError(t *testing.T) {
	if IsNotFoundError(fmt.Errorf("something else")) {
		t.Error("expected false for non-NotFound error")
	}
}

func TestIsNotFoundError_Nil(t *testing.T) {
	if IsNotFoundError(nil) {
		t.Error("expected false for nil")
	}
}

// --- IsDuplicateError ---

func TestIsDuplicateError_DuplicatedKey(t *testing.T) {
	if !IsDuplicateError(gorm.ErrDuplicatedKey) {
		t.Error("expected true for gorm.ErrDuplicatedKey")
	}
}

func TestIsDuplicateError_WrappedDuplicatedKey(t *testing.T) {
	wrapped := fmt.Errorf("insert failed: %w", gorm.ErrDuplicatedKey)
	if !IsDuplicateError(wrapped) {
		t.Error("expected true for wrapped gorm.ErrDuplicatedKey")
	}
}

func TestIsDuplicateError_OtherError(t *testing.T) {
	if IsDuplicateError(fmt.Errorf("something else")) {
		t.Error("expected false for non-duplicate error")
	}
}

func TestIsDuplicateError_Nil(t *testing.T) {
	if IsDuplicateError(nil) {
		t.Error("expected false for nil")
	}
}

// --- FromDatabase ---

func TestFromDatabase_Nil(t *testing.T) {
	result := FromDatabase(nil, "user")
	if result != nil {
		t.Error("expected nil for nil error")
	}
}

func TestFromDatabase_NotFound(t *testing.T) {
	appErr := FromDatabase(gorm.ErrRecordNotFound, "user")
	if appErr == nil {
		t.Fatal("expected non-nil AppError")
	}
	if appErr.Code != apperrors.ErrCodeNotFound {
		t.Errorf("code = %s, want %s", appErr.Code, apperrors.ErrCodeNotFound)
	}
	if appErr.HTTPStatus != http.StatusNotFound {
		t.Errorf("status = %d, want %d", appErr.HTTPStatus, http.StatusNotFound)
	}
}

func TestFromDatabase_DuplicateKey(t *testing.T) {
	appErr := FromDatabase(gorm.ErrDuplicatedKey, "order")
	if appErr == nil {
		t.Fatal("expected non-nil AppError")
	}
	if appErr.Code != apperrors.ErrCodeAlreadyExists {
		t.Errorf("code = %s, want %s", appErr.Code, apperrors.ErrCodeAlreadyExists)
	}
	if appErr.HTTPStatus != http.StatusConflict {
		t.Errorf("status = %d, want %d", appErr.HTTPStatus, http.StatusConflict)
	}
	if appErr.Retryable {
		t.Error("duplicate key should not be retryable")
	}
	if !errors.Is(appErr.Cause, gorm.ErrDuplicatedKey) {
		t.Error("cause should be the original error")
	}
}

func TestFromDatabase_ConnectionError(t *testing.T) {
	connErr := fmt.Errorf("connection refused")
	appErr := FromDatabase(connErr, "user")
	if appErr == nil {
		t.Fatal("expected non-nil AppError")
	}
	if appErr.Code != apperrors.ErrCodeDatabaseError {
		t.Errorf("code = %s, want %s", appErr.Code, apperrors.ErrCodeDatabaseError)
	}
	if appErr.HTTPStatus != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", appErr.HTTPStatus, http.StatusServiceUnavailable)
	}
	if !appErr.Retryable {
		t.Error("connection error should be retryable")
	}
}

func TestFromDatabase_RetryableDeadlock(t *testing.T) {
	dlErr := fmt.Errorf("deadlock detected")
	appErr := FromDatabase(dlErr, "order")
	if appErr == nil {
		t.Fatal("expected non-nil AppError")
	}
	if appErr.HTTPStatus != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", appErr.HTTPStatus, http.StatusServiceUnavailable)
	}
	if !appErr.Retryable {
		t.Error("deadlock should be retryable")
	}
}

func TestFromDatabase_GenericError(t *testing.T) {
	genErr := fmt.Errorf("some unknown DB error")
	appErr := FromDatabase(genErr, "item")
	if appErr == nil {
		t.Fatal("expected non-nil AppError")
	}
	if appErr.Code != apperrors.ErrCodeDatabaseError {
		t.Errorf("code = %s, want %s", appErr.Code, apperrors.ErrCodeDatabaseError)
	}
	if appErr.HTTPStatus != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", appErr.HTTPStatus, http.StatusInternalServerError)
	}
}

func TestFromDatabase_ResourceInMessage(t *testing.T) {
	appErr := FromDatabase(gorm.ErrDuplicatedKey, "product")
	if appErr == nil {
		t.Fatal("expected non-nil AppError")
	}
	if appErr.Message == "" {
		t.Error("message should not be empty")
	}
}
