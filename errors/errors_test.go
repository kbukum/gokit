package errors

import (
	stderrors "errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestAppError_New_Success(t *testing.T) {
	err := New(ErrCodeNotFound, "not found", http.StatusNotFound)
	if err.Code != ErrCodeNotFound {
		t.Errorf("expected code %s, got %s", ErrCodeNotFound, err.Code)
	}
	if err.Message != "not found" {
		t.Errorf("expected message 'not found', got %q", err.Message)
	}
	if err.HTTPStatus != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, err.HTTPStatus)
	}
	if err.Retryable != false {
		t.Error("NOT_FOUND should not be retryable")
	}
}

func TestAppError_New_Retryable(t *testing.T) {
	err := New(ErrCodeTimeout, "timed out", http.StatusGatewayTimeout)
	if !err.Retryable {
		t.Error("TIMEOUT should be retryable")
	}
}

func TestAppError_NotFound_Success(t *testing.T) {
	err := NotFound("user", "123")
	if err.Code != ErrCodeNotFound {
		t.Errorf("expected NOT_FOUND, got %s", err.Code)
	}
	if err.HTTPStatus != http.StatusNotFound {
		t.Errorf("expected 404, got %d", err.HTTPStatus)
	}
	if err.Details["resource"] != "user" {
		t.Errorf("expected resource=user, got %v", err.Details["resource"])
	}
	if err.Details["id"] != "123" {
		t.Errorf("expected id=123, got %v", err.Details["id"])
	}
	if err.Retryable {
		t.Error("NotFound should not be retryable")
	}
}

func TestAppError_NotFound_EmptyID(t *testing.T) {
	err := NotFound("user", "")
	if _, ok := err.Details["id"]; ok {
		t.Error("expected no 'id' key in details when id is empty")
	}
}

func TestAppError_Internal_Success(t *testing.T) {
	cause := fmt.Errorf("db connection lost")
	err := Internal(cause)
	if err.Code != ErrCodeInternal {
		t.Errorf("expected INTERNAL_ERROR, got %s", err.Code)
	}
	if err.HTTPStatus != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", err.HTTPStatus)
	}
	if err.Cause != cause {
		t.Error("expected cause to be set")
	}
	if err.Retryable {
		t.Error("Internal should NOT be retryable by default")
	}
}

func TestAppError_Unauthorized_Success(t *testing.T) {
	err := Unauthorized("")
	if err.Code != ErrCodeUnauthorized {
		t.Errorf("expected UNAUTHORIZED, got %s", err.Code)
	}
	if err.Message != "Authentication required." {
		t.Errorf("expected default message, got %q", err.Message)
	}

	err2 := Unauthorized("bad token")
	if err2.Message != "bad token" {
		t.Errorf("expected custom message, got %q", err2.Message)
	}
}

func TestAppError_Forbidden_Success(t *testing.T) {
	err := Forbidden("")
	if err.HTTPStatus != http.StatusForbidden {
		t.Errorf("expected 403, got %d", err.HTTPStatus)
	}
	if !strings.Contains(err.Message, "permission") {
		t.Errorf("expected default message with 'permission', got %q", err.Message)
	}
}

func TestAppError_TokenExpired_Success(t *testing.T) {
	err := TokenExpired()
	if err.Code != ErrCodeTokenExpired {
		t.Errorf("expected TOKEN_EXPIRED, got %s", err.Code)
	}
	if err.HTTPStatus != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", err.HTTPStatus)
	}
}

func TestAppError_InvalidInput_Success(t *testing.T) {
	err := InvalidInput("email", "must be valid")
	if err.Code != ErrCodeInvalidInput {
		t.Errorf("expected INVALID_INPUT, got %s", err.Code)
	}
	if err.Details["field"] != "email" {
		t.Errorf("expected field=email, got %v", err.Details["field"])
	}
}

func TestAppError_WithCause_Chain(t *testing.T) {
	cause := fmt.Errorf("root cause")
	err := NotFound("item", "1").WithCause(cause)
	if err.Cause != cause {
		t.Error("expected cause to be set via WithCause")
	}
	if !strings.Contains(err.Error(), "root cause") {
		t.Errorf("Error() should contain cause, got %q", err.Error())
	}
}

func TestAppError_WithDetails_Merge(t *testing.T) {
	err := NotFound("item", "1").WithDetails(map[string]any{
		"extra": "info",
	})
	if err.Details["extra"] != "info" {
		t.Errorf("expected extra=info in details")
	}
	if err.Details["resource"] != "item" {
		t.Error("expected original details to be preserved")
	}

	// Test merging into existing details
	err.WithDetails(map[string]any{
		"another": "detail",
	})
	if err.Details["another"] != "detail" {
		t.Error("expected another=detail to be merged")
	}
	if err.Details["extra"] != "info" {
		t.Error("expected extra=info to be preserved after second merge")
	}
}

func TestAppError_WithDetails_Nil(t *testing.T) {
	err := Internal(nil).WithDetails(nil)
	if err.Details == nil {
		t.Fatal("expected Details map to be initialized even with nil input")
	}
}

func TestAppError_WithDetail_Single(t *testing.T) {
	err := Internal(nil).WithDetail("trace", "abc")
	if err.Details["trace"] != "abc" {
		t.Errorf("expected trace=abc in details")
	}

	// Test overwriting
	err.WithDetail("trace", "def")
	if err.Details["trace"] != "def" {
		t.Errorf("expected trace=def after overwrite")
	}
}

func TestAppError_WithDetail_NilMap(t *testing.T) {
	err := &AppError{}
	err.WithDetail("key", "value")
	if err.Details == nil {
		t.Fatal("expected Details map to be initialized")
	}
	if err.Details["key"] != "value" {
		t.Errorf("expected key=value, got %v", err.Details["key"])
	}
}

func TestAppError_Error_Format(t *testing.T) {
	err := NotFound("user", "5")
	s := err.Error()
	if !strings.Contains(s, "NOT_FOUND") {
		t.Errorf("expected error string to contain code, got %q", s)
	}
	if !strings.Contains(s, "not found") {
		t.Errorf("expected error string to contain message, got %q", s)
	}
}

func TestAppError_Unwrap_Success(t *testing.T) {
	cause := fmt.Errorf("underlying")
	err := Internal(cause)
	if err.Unwrap() != cause {
		t.Error("Unwrap should return the cause")
	}

	err2 := NotFound("x", "")
	if err2.Unwrap() != nil {
		t.Error("Unwrap should return nil when no cause")
	}
}

func TestAppError_Constructors_Table(t *testing.T) {
	tests := []struct {
		name      string
		err       *AppError
		code      ErrorCode
		status    int
		retryable bool
	}{
		{"ServiceUnavailable", ServiceUnavailable("api"), ErrCodeServiceUnavailable, http.StatusServiceUnavailable, true},
		{"ConnectionFailed", ConnectionFailed("db"), ErrCodeConnectionFailed, http.StatusServiceUnavailable, true},
		{"Timeout", Timeout("query"), ErrCodeTimeout, http.StatusGatewayTimeout, true},
		{"RateLimited", RateLimited(), ErrCodeRateLimited, http.StatusTooManyRequests, true},
		{"AlreadyExists", AlreadyExists("user"), ErrCodeAlreadyExists, http.StatusConflict, false},
		{"Conflict", Conflict("version mismatch"), ErrCodeConflict, http.StatusConflict, false},
		{"MissingField", MissingField("name"), ErrCodeMissingField, http.StatusBadRequest, false},
		{"InvalidFormat", InvalidFormat("date", "RFC3339"), ErrCodeInvalidFormat, http.StatusBadRequest, false},
		{"InvalidToken", InvalidToken(), ErrCodeInvalidToken, http.StatusUnauthorized, false},
		{"DatabaseError", DatabaseError(nil), ErrCodeDatabaseError, http.StatusInternalServerError, true},
		{"ExternalServiceError", ExternalServiceError("stripe", nil), ErrCodeExternalService, http.StatusBadGateway, true},
		{"Validation", Validation("bad input"), ErrCodeInvalidInput, http.StatusBadRequest, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err.Code != tc.code {
				t.Errorf("expected code %s, got %s", tc.code, tc.err.Code)
			}
			if tc.err.HTTPStatus != tc.status {
				t.Errorf("expected status %d, got %d", tc.status, tc.err.HTTPStatus)
			}
			if tc.err.Retryable != tc.retryable {
				t.Errorf("expected retryable=%v, got %v", tc.retryable, tc.err.Retryable)
			}
		})
	}
}

func TestErrorCode_IsRetryableCode_Table(t *testing.T) {
	retryable := []ErrorCode{ErrCodeServiceUnavailable, ErrCodeConnectionFailed, ErrCodeTimeout, ErrCodeRateLimited, ErrCodeDatabaseError, ErrCodeExternalService}
	for _, code := range retryable {
		if !IsRetryableCode(code) {
			t.Errorf("expected %s to be retryable", code)
		}
	}

	nonRetryable := []ErrorCode{ErrCodeNotFound, ErrCodeAlreadyExists, ErrCodeInvalidInput, ErrCodeUnauthorized, ErrCodeForbidden, ErrCodeInternal}
	for _, code := range nonRetryable {
		if IsRetryableCode(code) {
			t.Errorf("expected %s to NOT be retryable", code)
		}
	}
}

func TestAppError_ToResponse_Success(t *testing.T) {
	err := NotFound("user", "42")
	resp := err.ToResponse()
	if resp.Error.Code != ErrCodeNotFound {
		t.Errorf("expected code NOT_FOUND in response, got %s", resp.Error.Code)
	}
	if resp.Error.Retryable != false {
		t.Error("expected retryable=false in response")
	}
	if resp.Error.Details["resource"] != "user" {
		t.Error("expected resource=user in response details")
	}
}

func TestAppError_IsAppError_Success(t *testing.T) {
	appErr := NotFound("x", "")
	if !IsAppError(appErr) {
		t.Error("expected IsAppError to return true for AppError")
	}

	wrapped := fmt.Errorf("wrapped: %w", appErr)
	if !IsAppError(wrapped) {
		t.Error("expected IsAppError to return true for wrapped AppError")
	}

	plain := fmt.Errorf("plain error")
	if IsAppError(plain) {
		t.Error("expected IsAppError to return false for plain error")
	}
}

func TestAppError_AsAppError_Success(t *testing.T) {
	appErr := Internal(nil)
	wrapped := fmt.Errorf("wrap: %w", appErr)

	got, ok := AsAppError(wrapped)
	if !ok {
		t.Fatal("expected AsAppError to succeed for wrapped AppError")
	}
	if got.Code != ErrCodeInternal {
		t.Errorf("expected INTERNAL_ERROR, got %s", got.Code)
	}

	_, ok = AsAppError(fmt.Errorf("not an app error"))
	if ok {
		t.Error("expected AsAppError to return false for non-AppError")
	}
}

func TestWrap_NilReturnsNil(t *testing.T) {
	if Wrap(nil) != nil {
		t.Error("Wrap(nil) should return nil")
	}
}

func TestWrap_AppErrorPassthrough(t *testing.T) {
	orig := NotFound("item", "1")
	got := Wrap(orig)
	if got != orig {
		t.Error("Wrap should return the original AppError unchanged")
	}
}

func TestWrap_WrappedAppError(t *testing.T) {
	orig := NotFound("item", "1")
	wrapped := fmt.Errorf("outer: %w", orig)
	got := Wrap(wrapped)
	if got.Code != ErrCodeNotFound {
		t.Errorf("expected NOT_FOUND, got %s", got.Code)
	}
}

func TestWrap_PlainError(t *testing.T) {
	plain := fmt.Errorf("something broke")
	got := Wrap(plain)
	if got.Code != ErrCodeInternal {
		t.Errorf("expected INTERNAL_ERROR, got %s", got.Code)
	}
	if got.Cause != plain {
		t.Error("expected cause to be the original error")
	}
}

func TestFormatResourceError_Success(t *testing.T) {
	err := FormatResourceError("user", 42)
	if err.Code != ErrCodeNotFound {
		t.Errorf("expected NOT_FOUND, got %s", err.Code)
	}
	if err.Details["id"] != "42" {
		t.Errorf("expected id=42, got %v", err.Details["id"])
	}
	if err.Details["resource"] != "user" {
		t.Errorf("expected resource=user, got %v", err.Details["resource"])
	}
}

func TestFormatResourceError_StringID(t *testing.T) {
	err := FormatResourceError("bot", "abc-123")
	if err.Details["id"] != "abc-123" {
		t.Errorf("expected id=abc-123, got %v", err.Details["id"])
	}
}

func TestAppError_ImplementsErrorInterface(t *testing.T) {
	var err error = NotFound("test", "1")
	if err.Error() == "" {
		t.Error("Error() should not be empty")
	}

	var appErr *AppError
	if !stderrors.As(err, &appErr) {
		t.Error("stderrors.As should work with AppError")
	}
}
