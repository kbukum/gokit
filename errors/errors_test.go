package errors

import (
	"encoding/json"
	"errors"
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
	if !errors.Is(err.Cause, cause) {
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
	if !errors.Is(err.Cause, cause) {
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
	if !errors.Is(err.Unwrap(), cause) {
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
		{"MissingField", MissingField("name"), ErrCodeMissingField, http.StatusUnprocessableEntity, false},
		{"InvalidFormat", InvalidFormat("date", "RFC3339"), ErrCodeInvalidFormat, http.StatusUnprocessableEntity, false},
		{"InvalidToken", InvalidToken(), ErrCodeInvalidToken, http.StatusUnauthorized, false},
		{"DatabaseError", DatabaseError(nil), ErrCodeDatabaseError, http.StatusInternalServerError, false},
		{"ExternalServiceError", ExternalServiceError("stripe", nil), ErrCodeExternalService, http.StatusBadGateway, true},
		{"Validation", Validation("bad input"), ErrCodeInvalidInput, http.StatusUnprocessableEntity, false},
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
	retryable := []ErrorCode{ErrCodeServiceUnavailable, ErrCodeConnectionFailed, ErrCodeTimeout, ErrCodeRateLimited, ErrCodeExternalService}
	for _, code := range retryable {
		if !IsRetryableCode(code) {
			t.Errorf("expected %s to be retryable", code)
		}
	}

	nonRetryable := []ErrorCode{ErrCodeNotFound, ErrCodeAlreadyExists, ErrCodeInvalidInput, ErrCodeUnauthorized, ErrCodeForbidden, ErrCodeInternal, ErrCodeDatabaseError}
	for _, code := range nonRetryable {
		if IsRetryableCode(code) {
			t.Errorf("expected %s to NOT be retryable", code)
		}
	}
}

func TestAppError_ToProblemDetail_Success(t *testing.T) {
	err := NotFound("user", "42")
	pd := err.ToProblemDetail()
	if pd.Code != ErrCodeNotFound {
		t.Errorf("expected code NOT_FOUND in problem detail, got %s", pd.Code)
	}
	if pd.Retryable != false {
		t.Error("expected retryable=false in problem detail")
	}
	if pd.Details["resource"] != "user" {
		t.Error("expected resource=user in problem detail details")
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
	if !errors.Is(got.Cause, plain) {
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
	if !errors.As(err, &appErr) {
		t.Error("errors.As should work with AppError")
	}
}

// ---------------------------------------------------------------------------
// 1. Exhaustive ErrorCode → HTTP Status mapping for ALL 17 codes
// ---------------------------------------------------------------------------

func TestErrorCode_HTTPStatusMapping_All(t *testing.T) {
	t.Parallel()
	tests := []struct {
		code   ErrorCode
		status int
	}{
		// Connection/Availability (retryable)
		{ErrCodeServiceUnavailable, http.StatusServiceUnavailable},
		{ErrCodeConnectionFailed, http.StatusServiceUnavailable},
		{ErrCodeTimeout, http.StatusGatewayTimeout},
		{ErrCodeRateLimited, http.StatusTooManyRequests},
		// Resource
		{ErrCodeNotFound, http.StatusNotFound},
		{ErrCodeAlreadyExists, http.StatusConflict},
		{ErrCodeConflict, http.StatusConflict},
		// Validation
		{ErrCodeInvalidInput, http.StatusUnprocessableEntity},
		{ErrCodeMissingField, http.StatusUnprocessableEntity},
		{ErrCodeInvalidFormat, http.StatusUnprocessableEntity},
		// Auth
		{ErrCodeUnauthorized, http.StatusUnauthorized},
		{ErrCodeForbidden, http.StatusForbidden},
		{ErrCodeTokenExpired, http.StatusUnauthorized},
		{ErrCodeInvalidToken, http.StatusUnauthorized},
		// Internal
		{ErrCodeInternal, http.StatusInternalServerError},
		{ErrCodeDatabaseError, http.StatusInternalServerError},
		{ErrCodeExternalService, http.StatusBadGateway},
	}

	constructorForCode := map[ErrorCode]func() *AppError{
		ErrCodeServiceUnavailable: func() *AppError { return ServiceUnavailable("svc") },
		ErrCodeConnectionFailed:   func() *AppError { return ConnectionFailed("db") },
		ErrCodeTimeout:            func() *AppError { return Timeout("op") },
		ErrCodeRateLimited:        RateLimited,
		ErrCodeNotFound:           func() *AppError { return NotFound("res", "1") },
		ErrCodeAlreadyExists:      func() *AppError { return AlreadyExists("res") },
		ErrCodeConflict:           func() *AppError { return Conflict("reason") },
		ErrCodeInvalidInput:       func() *AppError { return InvalidInput("f", "r") },
		ErrCodeMissingField:       func() *AppError { return MissingField("f") },
		ErrCodeInvalidFormat:      func() *AppError { return InvalidFormat("f", "fmt") },
		ErrCodeUnauthorized:       func() *AppError { return Unauthorized("") },
		ErrCodeForbidden:          func() *AppError { return Forbidden("") },
		ErrCodeTokenExpired:       TokenExpired,
		ErrCodeInvalidToken:       InvalidToken,
		ErrCodeInternal:           func() *AppError { return Internal(nil) },
		ErrCodeDatabaseError:      func() *AppError { return DatabaseError(nil) },
		ErrCodeExternalService:    func() *AppError { return ExternalServiceError("ext", nil) },
	}

	for _, tc := range tests {
		t.Run(string(tc.code), func(t *testing.T) {
			t.Parallel()
			fn, ok := constructorForCode[tc.code]
			if !ok {
				t.Fatalf("no constructor registered for code %s", tc.code)
			}
			err := fn()
			if err.HTTPStatus != tc.status {
				t.Errorf("code %s: expected HTTP %d, got %d", tc.code, tc.status, err.HTTPStatus)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 2. Exhaustive IsRetryable for ALL 17 codes
// ---------------------------------------------------------------------------

func TestIsRetryableCode_Exhaustive(t *testing.T) {
	t.Parallel()
	tests := []struct {
		code      ErrorCode
		retryable bool
	}{
		{ErrCodeServiceUnavailable, true},
		{ErrCodeConnectionFailed, true},
		{ErrCodeTimeout, true},
		{ErrCodeRateLimited, true},
		{ErrCodeExternalService, true},
		{ErrCodeNotFound, false},
		{ErrCodeAlreadyExists, false},
		{ErrCodeConflict, false},
		{ErrCodeInvalidInput, false},
		{ErrCodeMissingField, false},
		{ErrCodeInvalidFormat, false},
		{ErrCodeUnauthorized, false},
		{ErrCodeForbidden, false},
		{ErrCodeTokenExpired, false},
		{ErrCodeInvalidToken, false},
		{ErrCodeInternal, false},
		{ErrCodeDatabaseError, false},
	}

	for _, tc := range tests {
		t.Run(string(tc.code), func(t *testing.T) {
			t.Parallel()
			got := IsRetryableCode(tc.code)
			if got != tc.retryable {
				t.Errorf("IsRetryableCode(%s) = %v, want %v", tc.code, got, tc.retryable)
			}
		})
	}
}

func TestIsRetryableCode_UnknownCode(t *testing.T) {
	t.Parallel()
	if IsRetryableCode(ErrorCode("UNKNOWN_CODE")) {
		t.Error("unknown code should not be retryable")
	}
}

// ---------------------------------------------------------------------------
// 3. RFC 9457 ProblemDetail
// ---------------------------------------------------------------------------

func TestToProblemDetail_AllCodes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		code        ErrorCode
		expectedURL string
		status      int
	}{
		{ErrCodeServiceUnavailable, "https://gokit.dev/errors/service-unavailable", http.StatusServiceUnavailable},
		{ErrCodeConnectionFailed, "https://gokit.dev/errors/connection-failed", http.StatusServiceUnavailable},
		{ErrCodeTimeout, "https://gokit.dev/errors/timeout", http.StatusGatewayTimeout},
		{ErrCodeRateLimited, "https://gokit.dev/errors/rate-limited", http.StatusTooManyRequests},
		{ErrCodeNotFound, "https://gokit.dev/errors/not-found", http.StatusNotFound},
		{ErrCodeAlreadyExists, "https://gokit.dev/errors/already-exists", http.StatusConflict},
		{ErrCodeConflict, "https://gokit.dev/errors/conflict", http.StatusConflict},
		{ErrCodeInvalidInput, "https://gokit.dev/errors/invalid-input", http.StatusUnprocessableEntity},
		{ErrCodeMissingField, "https://gokit.dev/errors/missing-field", http.StatusUnprocessableEntity},
		{ErrCodeInvalidFormat, "https://gokit.dev/errors/invalid-format", http.StatusUnprocessableEntity},
		{ErrCodeUnauthorized, "https://gokit.dev/errors/unauthorized", http.StatusUnauthorized},
		{ErrCodeForbidden, "https://gokit.dev/errors/forbidden", http.StatusForbidden},
		{ErrCodeTokenExpired, "https://gokit.dev/errors/token-expired", http.StatusUnauthorized},
		{ErrCodeInvalidToken, "https://gokit.dev/errors/invalid-token", http.StatusUnauthorized},
		{ErrCodeInternal, "https://gokit.dev/errors/internal-error", http.StatusInternalServerError},
		{ErrCodeDatabaseError, "https://gokit.dev/errors/database-error", http.StatusInternalServerError},
		{ErrCodeExternalService, "https://gokit.dev/errors/external-service-error", http.StatusBadGateway},
	}

	constructorForCode := map[ErrorCode]func() *AppError{
		ErrCodeServiceUnavailable: func() *AppError { return ServiceUnavailable("svc") },
		ErrCodeConnectionFailed:   func() *AppError { return ConnectionFailed("db") },
		ErrCodeTimeout:            func() *AppError { return Timeout("op") },
		ErrCodeRateLimited:        RateLimited,
		ErrCodeNotFound:           func() *AppError { return NotFound("res", "1") },
		ErrCodeAlreadyExists:      func() *AppError { return AlreadyExists("res") },
		ErrCodeConflict:           func() *AppError { return Conflict("reason") },
		ErrCodeInvalidInput:       func() *AppError { return InvalidInput("f", "r") },
		ErrCodeMissingField:       func() *AppError { return MissingField("f") },
		ErrCodeInvalidFormat:      func() *AppError { return InvalidFormat("f", "fmt") },
		ErrCodeUnauthorized:       func() *AppError { return Unauthorized("") },
		ErrCodeForbidden:          func() *AppError { return Forbidden("") },
		ErrCodeTokenExpired:       TokenExpired,
		ErrCodeInvalidToken:       InvalidToken,
		ErrCodeInternal:           func() *AppError { return Internal(nil) },
		ErrCodeDatabaseError:      func() *AppError { return DatabaseError(nil) },
		ErrCodeExternalService:    func() *AppError { return ExternalServiceError("ext", nil) },
	}

	for _, tc := range tests {
		t.Run(string(tc.code), func(t *testing.T) {
			t.Parallel()
			err := constructorForCode[tc.code]()
			pd := err.ToProblemDetail()

			if pd.Type != tc.expectedURL {
				t.Errorf("Type = %q, want %q", pd.Type, tc.expectedURL)
			}
			if pd.Status != tc.status {
				t.Errorf("Status = %d, want %d", pd.Status, tc.status)
			}
			if pd.Detail == "" {
				t.Error("Detail should not be empty")
			}
			if pd.Code != tc.code {
				t.Errorf("Code = %q, want %q", pd.Code, tc.code)
			}
		})
	}
}

func TestToProblemDetail_Detail(t *testing.T) {
	t.Parallel()
	err := NotFound("user", "42")
	pd := err.ToProblemDetail()
	if pd.Detail != err.Message {
		t.Errorf("Detail = %q, want %q", pd.Detail, err.Message)
	}
}

func TestToProblemDetail_EmptyError(t *testing.T) {
	t.Parallel()
	err := &AppError{}
	pd := err.ToProblemDetail()
	if pd.Type != "https://gokit.dev/errors/" {
		t.Errorf("Type = %q, want base URL with empty code", pd.Type)
	}
	if pd.Status != 0 {
		t.Errorf("Status = %d, want 0 for zero-value AppError", pd.Status)
	}
	if pd.Detail != "" {
		t.Errorf("Detail = %q, want empty", pd.Detail)
	}
}

func TestToProblemDetail_KebabCaseConversion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		code     ErrorCode
		expected string
	}{
		{ErrorCode("SINGLE"), "single"},
		{ErrorCode("TWO_WORDS"), "two-words"},
		{ErrorCode("THREE_LONG_WORDS"), "three-long-words"},
		{ErrorCode("ALREADY_HAS"), "already-has"},
	}
	for _, tc := range tests {
		t.Run(string(tc.code), func(t *testing.T) {
			t.Parallel()
			err := &AppError{Code: tc.code}
			pd := err.ToProblemDetail()
			wantURL := "https://gokit.dev/errors/" + tc.expected
			if pd.Type != wantURL {
				t.Errorf("Type = %q, want %q", pd.Type, wantURL)
			}
		})
	}
}

func TestToProblemDetail_TitleCase(t *testing.T) {
	t.Parallel()
	tests := []struct {
		code  ErrorCode
		title string
	}{
		{ErrCodeNotFound, "Not Found"},
		{ErrCodeInternal, "Internal Error"},
		{ErrCodeServiceUnavailable, "Service Unavailable"},
	}
	for _, tc := range tests {
		t.Run(string(tc.code), func(t *testing.T) {
			t.Parallel()
			err := &AppError{Code: tc.code}
			pd := err.ToProblemDetail()
			if pd.Title != tc.title {
				t.Errorf("Title = %q, want %q", pd.Title, tc.title)
			}
		})
	}
}

func TestSetTypeBaseURI(t *testing.T) {
	t.Parallel()
	original := GetTypeBaseURI()
	defer SetTypeBaseURI(original)

	SetTypeBaseURI("https://example.com/problems")
	if GetTypeBaseURI() != "https://example.com/problems/" {
		t.Errorf("GetTypeBaseURI() = %q, want trailing slash normalised", GetTypeBaseURI())
	}

	err := NotFound("item", "1")
	pd := err.ToProblemDetail()
	if !strings.HasPrefix(pd.Type, "https://example.com/problems/") {
		t.Errorf("Type = %q, want prefix https://example.com/problems/", pd.Type)
	}
}

func TestSetTypeBaseURI_AlreadyHasSlash(t *testing.T) {
	t.Parallel()
	original := GetTypeBaseURI()
	defer SetTypeBaseURI(original)

	SetTypeBaseURI("https://example.com/p/")
	if GetTypeBaseURI() != "https://example.com/p/" {
		t.Errorf("GetTypeBaseURI() = %q, should not double-slash", GetTypeBaseURI())
	}
}

// ---------------------------------------------------------------------------
// 4. Constructor detail verification
// ---------------------------------------------------------------------------

func TestConstructor_NotFound_Details(t *testing.T) {
	t.Parallel()
	err := NotFound("user", "123")
	if err.Details["resource"] != "user" {
		t.Errorf("resource = %v, want user", err.Details["resource"])
	}
	if err.Details["id"] != "123" {
		t.Errorf("id = %v, want 123", err.Details["id"])
	}
}

func TestConstructor_ServiceUnavailable_Details(t *testing.T) {
	t.Parallel()
	err := ServiceUnavailable("db")
	if err.Details["service"] != "db" {
		t.Errorf("service = %v, want db", err.Details["service"])
	}
}

func TestConstructor_ConnectionFailed_Details(t *testing.T) {
	t.Parallel()
	err := ConnectionFailed("redis")
	if err.Details["service"] != "redis" {
		t.Errorf("service = %v, want redis", err.Details["service"])
	}
}

func TestConstructor_Timeout_Details(t *testing.T) {
	t.Parallel()
	err := Timeout("query")
	if err.Details["operation"] != "query" {
		t.Errorf("operation = %v, want query", err.Details["operation"])
	}
}

func TestConstructor_AlreadyExists_Details(t *testing.T) {
	t.Parallel()
	err := AlreadyExists("account")
	if err.Details["resource"] != "account" {
		t.Errorf("resource = %v, want account", err.Details["resource"])
	}
}

func TestConstructor_MissingField_Details(t *testing.T) {
	t.Parallel()
	err := MissingField("email")
	if err.Details["field"] != "email" {
		t.Errorf("field = %v, want email", err.Details["field"])
	}
	if !strings.Contains(err.Message, "email") {
		t.Errorf("message should mention field name, got %q", err.Message)
	}
}

func TestConstructor_InvalidFormat_Details(t *testing.T) {
	t.Parallel()
	err := InvalidFormat("date", "RFC3339")
	if err.Details["field"] != "date" {
		t.Errorf("field = %v, want date", err.Details["field"])
	}
	if err.Details["expected_format"] != "RFC3339" {
		t.Errorf("expected_format = %v, want RFC3339", err.Details["expected_format"])
	}
}

func TestConstructor_InvalidInput_Details(t *testing.T) {
	t.Parallel()
	err := InvalidInput("email", "must be valid")
	if err.Details["field"] != "email" {
		t.Errorf("field = %v, want email", err.Details["field"])
	}
	if !strings.Contains(err.Message, "must be valid") {
		t.Errorf("message should contain reason, got %q", err.Message)
	}
}

func TestConstructor_InvalidInput_EmptyField(t *testing.T) {
	t.Parallel()
	err := InvalidInput("", "some reason")
	if _, ok := err.Details["field"]; ok {
		t.Error("empty field should not be present in details")
	}
}

func TestConstructor_ExternalServiceError_Details(t *testing.T) {
	t.Parallel()
	cause := fmt.Errorf("connection refused")
	err := ExternalServiceError("stripe", cause)
	if err.Details["service"] != "stripe" {
		t.Errorf("service = %v, want stripe", err.Details["service"])
	}
	if !errors.Is(err.Cause, cause) {
		t.Error("cause should be set")
	}
}

func TestConstructor_Conflict_NoDetails(t *testing.T) {
	t.Parallel()
	err := Conflict("version mismatch")
	if err.Message != "version mismatch" {
		t.Errorf("message = %q, want 'version mismatch'", err.Message)
	}
	if len(err.Details) != 0 {
		t.Errorf("Conflict should have no details, got %v", err.Details)
	}
}

func TestConstructor_RateLimited_NoDetails(t *testing.T) {
	t.Parallel()
	err := RateLimited()
	if len(err.Details) != 0 {
		t.Errorf("RateLimited should have no details, got %v", err.Details)
	}
}

func TestConstructor_Validation_NoDetails(t *testing.T) {
	t.Parallel()
	err := Validation("bad input")
	if err.Code != ErrCodeInvalidInput {
		t.Errorf("code = %s, want INVALID_INPUT", err.Code)
	}
	if err.Message != "bad input" {
		t.Errorf("message = %q, want 'bad input'", err.Message)
	}
	if len(err.Details) != 0 {
		t.Errorf("Validation should have no details, got %v", err.Details)
	}
}

func TestConstructor_Unauthorized_CustomMessage(t *testing.T) {
	t.Parallel()
	err := Unauthorized("invalid credentials")
	if err.Message != "invalid credentials" {
		t.Errorf("message = %q, want 'invalid credentials'", err.Message)
	}
}

func TestConstructor_Forbidden_CustomMessage(t *testing.T) {
	t.Parallel()
	err := Forbidden("admin only")
	if err.Message != "admin only" {
		t.Errorf("message = %q, want 'admin only'", err.Message)
	}
}

func TestConstructor_Internal_NilCause(t *testing.T) {
	t.Parallel()
	err := Internal(nil)
	if err.Cause != nil {
		t.Error("cause should be nil")
	}
	if !strings.Contains(err.Message, "unexpected error") {
		t.Errorf("message should mention unexpected error, got %q", err.Message)
	}
}

func TestConstructor_DatabaseError_NilCause(t *testing.T) {
	t.Parallel()
	err := DatabaseError(nil)
	if err.Cause != nil {
		t.Error("cause should be nil")
	}
	if !strings.Contains(err.Message, "database error") {
		t.Errorf("message should mention database, got %q", err.Message)
	}
}

// ---------------------------------------------------------------------------
// 5. Builder chain tests
// ---------------------------------------------------------------------------

func TestBuilderChain_WithCause_PreservesOriginal(t *testing.T) {
	t.Parallel()
	cause := fmt.Errorf("original cause")
	err := NotFound("item", "1").WithCause(cause)
	if err.Code != ErrCodeNotFound {
		t.Errorf("code changed to %s", err.Code)
	}
	if err.Details["resource"] != "item" {
		t.Error("details should be preserved")
	}
	if !errors.Is(err.Cause, cause) {
		t.Error("cause should be set")
	}
}

func TestBuilderChain_WithDetails_Merges(t *testing.T) {
	t.Parallel()
	err := NotFound("user", "1").
		WithDetails(map[string]any{"extra1": "val1"}).
		WithDetails(map[string]any{"extra2": "val2"})

	if err.Details["resource"] != "user" {
		t.Error("original 'resource' detail should be preserved")
	}
	if err.Details["extra1"] != "val1" {
		t.Error("extra1 should be present")
	}
	if err.Details["extra2"] != "val2" {
		t.Error("extra2 should be present")
	}
}

func TestBuilderChain_WithDetail_Multiple(t *testing.T) {
	t.Parallel()
	err := Internal(nil).
		WithDetail("key1", "val1").
		WithDetail("key2", "val2").
		WithDetail("key3", 42)

	if err.Details["key1"] != "val1" {
		t.Error("key1 missing")
	}
	if err.Details["key2"] != "val2" {
		t.Error("key2 missing")
	}
	if err.Details["key3"] != 42 {
		t.Error("key3 missing")
	}
}

func TestBuilderChain_FullChain(t *testing.T) {
	t.Parallel()
	cause := fmt.Errorf("root")
	err := NotFound("item", "1").
		WithCause(cause).
		WithDetails(map[string]any{"attempt": 3}).
		WithDetail("trace_id", "abc-123")

	if err.Code != ErrCodeNotFound {
		t.Error("code should be preserved")
	}
	if !errors.Is(err.Cause, cause) {
		t.Error("cause should be set")
	}
	if err.Details["attempt"] != 3 {
		t.Error("attempt detail should be set")
	}
	if err.Details["trace_id"] != "abc-123" {
		t.Error("trace_id detail should be set")
	}
	if err.Details["resource"] != "item" {
		t.Error("original resource detail should be preserved")
	}
}

func TestBuilderChain_WithDetails_OverwritesExisting(t *testing.T) {
	t.Parallel()
	err := NotFound("user", "1").
		WithDetails(map[string]any{"resource": "overwritten"})
	if err.Details["resource"] != "overwritten" {
		t.Errorf("resource = %v, want overwritten", err.Details["resource"])
	}
}

func TestBuilderChain_WithCause_ReplacesExisting(t *testing.T) {
	t.Parallel()
	cause1 := fmt.Errorf("first")
	cause2 := fmt.Errorf("second")
	err := Internal(cause1).WithCause(cause2)
	if !errors.Is(err.Cause, cause2) {
		t.Error("WithCause should replace existing cause")
	}
}

// ---------------------------------------------------------------------------
// 6. Edge cases
// ---------------------------------------------------------------------------

func TestEdgeCase_EmptyStringParams(t *testing.T) {
	t.Parallel()
	subtests := []struct {
		name string
		fn   func() *AppError
	}{
		{"NotFound_empty", func() *AppError { return NotFound("", "") }},
		{"ServiceUnavailable_empty", func() *AppError { return ServiceUnavailable("") }},
		{"ConnectionFailed_empty", func() *AppError { return ConnectionFailed("") }},
		{"Timeout_empty", func() *AppError { return Timeout("") }},
		{"AlreadyExists_empty", func() *AppError { return AlreadyExists("") }},
		{"Conflict_empty", func() *AppError { return Conflict("") }},
		{"InvalidInput_empty", func() *AppError { return InvalidInput("", "") }},
		{"MissingField_empty", func() *AppError { return MissingField("") }},
		{"InvalidFormat_empty", func() *AppError { return InvalidFormat("", "") }},
		{"Unauthorized_empty", func() *AppError { return Unauthorized("") }},
		{"Forbidden_empty", func() *AppError { return Forbidden("") }},
	}
	for _, tc := range subtests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.fn()
			if err == nil {
				t.Fatal("constructor should never return nil")
			}
			if err.Error() == "" {
				t.Error("Error() should never return empty string")
			}
		})
	}
}

func TestEdgeCase_VeryLongMessage(t *testing.T) {
	t.Parallel()
	longMsg := strings.Repeat("a", 10000)
	err := Conflict(longMsg)
	if err.Message != longMsg {
		t.Errorf("long message was truncated: len=%d, want %d", len(err.Message), len(longMsg))
	}
	if !strings.Contains(err.Error(), longMsg) {
		t.Error("Error() should include the full message")
	}
}

func TestEdgeCase_SpecialCharacters(t *testing.T) {
	t.Parallel()
	special := `<script>alert("xss")</script> & "quotes" 'single' \n\t`
	err := Conflict(special)
	if err.Message != special {
		t.Errorf("special chars not preserved: got %q", err.Message)
	}
}

func TestEdgeCase_UnicodeInDetails(t *testing.T) {
	t.Parallel()
	err := NotFound("用户", "日本語ID").
		WithDetail("emoji", "🚀✨").
		WithDetail("chinese", "你好世界")

	if err.Details["resource"] != "用户" {
		t.Error("unicode resource not preserved")
	}
	if err.Details["emoji"] != "🚀✨" {
		t.Error("emoji detail not preserved")
	}
	if err.Details["chinese"] != "你好世界" {
		t.Error("chinese detail not preserved")
	}
}

func TestEdgeCase_NilCauseInError(t *testing.T) {
	t.Parallel()
	err := Internal(nil)
	s := err.Error()
	if strings.Contains(s, "cause") {
		t.Errorf("Error() with nil cause should not mention 'cause', got %q", s)
	}
}

func TestEdgeCase_WithDetailsNilMap(t *testing.T) {
	t.Parallel()
	err := Conflict("test")
	err.WithDetails(nil)
	if err.Details == nil {
		t.Fatal("Details should be initialized even after nil merge")
	}
}

func TestEdgeCase_ZeroValueAppError(t *testing.T) {
	t.Parallel()
	err := &AppError{}
	s := err.Error()
	if s == "" {
		t.Error("Error() on zero-value should not panic or be empty")
	}
	if err.Unwrap() != nil {
		t.Error("Unwrap on zero-value should return nil")
	}
}

// ---------------------------------------------------------------------------
// 7. IsAppError / AsAppError helpers
// ---------------------------------------------------------------------------

func TestIsAppError_Nil(t *testing.T) {
	t.Parallel()
	if IsAppError(nil) {
		t.Error("IsAppError(nil) should return false")
	}
}

func TestAsAppError_Nil(t *testing.T) {
	t.Parallel()
	got, ok := AsAppError(nil)
	if ok {
		t.Error("AsAppError(nil) should return false")
	}
	if got != nil {
		t.Error("AsAppError(nil) should return nil AppError")
	}
}

func TestIsAppError_DoubleWrapped(t *testing.T) {
	t.Parallel()
	orig := NotFound("item", "1")
	wrapped := fmt.Errorf("layer1: %w", orig)
	doubleWrapped := fmt.Errorf("layer2: %w", wrapped)
	if !IsAppError(doubleWrapped) {
		t.Error("IsAppError should find AppError through multiple wraps")
	}
}

func TestAsAppError_DoubleWrapped(t *testing.T) {
	t.Parallel()
	orig := Timeout("op")
	doubleWrapped := fmt.Errorf("l2: %w", fmt.Errorf("l1: %w", orig))

	got, ok := AsAppError(doubleWrapped)
	if !ok {
		t.Fatal("AsAppError should succeed through multiple wraps")
	}
	if got.Code != ErrCodeTimeout {
		t.Errorf("code = %s, want TIMEOUT", got.Code)
	}
}

func TestErrorsIs_WithAppError(t *testing.T) {
	t.Parallel()
	cause := fmt.Errorf("sentinel")
	err := Internal(cause)
	if !errors.Is(err, cause) {
		t.Error("errors.Is should find the cause through Unwrap")
	}
}

func TestErrorsAs_WithAppError(t *testing.T) {
	t.Parallel()
	orig := Forbidden("nope")
	wrapped := fmt.Errorf("outer: %w", orig)

	var target *AppError
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As should find AppError through fmt.Errorf wrap")
	}
	if target.Code != ErrCodeForbidden {
		t.Errorf("code = %s, want FORBIDDEN", target.Code)
	}
}

// ---------------------------------------------------------------------------
// 8. Error interface compliance
// ---------------------------------------------------------------------------

func TestError_FormatWithCause(t *testing.T) {
	t.Parallel()
	cause := fmt.Errorf("disk full")
	err := Internal(cause)
	s := err.Error()
	if !strings.Contains(s, "INTERNAL_ERROR") {
		t.Errorf("should contain code, got %q", s)
	}
	if !strings.Contains(s, "disk full") {
		t.Errorf("should contain cause, got %q", s)
	}
	if !strings.Contains(s, "cause:") {
		t.Errorf("should contain 'cause:' prefix, got %q", s)
	}
}

func TestError_FormatWithoutCause(t *testing.T) {
	t.Parallel()
	err := NotFound("user", "5")
	s := err.Error()
	if strings.Contains(s, "cause") {
		t.Errorf("should not contain 'cause' when no cause, got %q", s)
	}
	expected := fmt.Sprintf("%s: %s", err.Code, err.Message)
	if s != expected {
		t.Errorf("Error() = %q, want %q", s, expected)
	}
}

func TestUnwrap_NilCause(t *testing.T) {
	t.Parallel()
	err := NotFound("x", "")
	if err.Unwrap() != nil {
		t.Error("Unwrap should return nil when no cause")
	}
}

func TestUnwrap_ChainedCause(t *testing.T) {
	t.Parallel()
	root := fmt.Errorf("root")
	mid := fmt.Errorf("mid: %w", root)
	err := Internal(mid)
	if !errors.Is(err.Unwrap(), mid) {
		t.Error("Unwrap should return the direct cause")
	}
	if !errors.Is(err, root) {
		t.Error("errors.Is should find the root cause through the chain")
	}
}

// ---------------------------------------------------------------------------
// 9. Serialization: ToProblemDetail and JSON round-trip
// ---------------------------------------------------------------------------

func TestToProblemDetail_AllFields(t *testing.T) {
	t.Parallel()
	err := NotFound("user", "42").WithDetail("trace", "xyz")
	pd := err.ToProblemDetail()

	if pd.Code != ErrCodeNotFound {
		t.Errorf("code = %s, want NOT_FOUND", pd.Code)
	}
	if pd.Detail != err.Message {
		t.Errorf("detail = %q, want %q", pd.Detail, err.Message)
	}
	if pd.Retryable != false {
		t.Error("retryable should be false")
	}
	if pd.Details["resource"] != "user" {
		t.Error("resource detail should be preserved")
	}
	if pd.Details["trace"] != "xyz" {
		t.Error("trace detail should be preserved")
	}
}

func TestToProblemDetail_NilDetails(t *testing.T) {
	t.Parallel()
	err := Conflict("test")
	pd := err.ToProblemDetail()
	if pd.Details != nil {
		t.Error("nil details should stay nil in problem detail")
	}
}

func TestToProblemDetail_Retryable(t *testing.T) {
	t.Parallel()
	err := ServiceUnavailable("api")
	pd := err.ToProblemDetail()
	if !pd.Retryable {
		t.Error("retryable should be true for ServiceUnavailable")
	}
}

func TestJSON_RoundTrip_ProblemDetail(t *testing.T) {
	t.Parallel()
	original := NotFound("user", "42").WithDetail("extra", "data")
	pd := original.ToProblemDetail()

	data, err := json.Marshal(pd)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded ProblemDetail
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if decoded.Code != pd.Code {
		t.Errorf("code = %s, want %s", decoded.Code, pd.Code)
	}
	if decoded.Detail != pd.Detail {
		t.Errorf("detail = %q, want %q", decoded.Detail, pd.Detail)
	}
	if decoded.Retryable != pd.Retryable {
		t.Errorf("retryable = %v, want %v", decoded.Retryable, pd.Retryable)
	}
	if decoded.Type != pd.Type {
		t.Errorf("type = %q, want %q", decoded.Type, pd.Type)
	}
	if decoded.Title != pd.Title {
		t.Errorf("title = %q, want %q", decoded.Title, pd.Title)
	}
	if decoded.Details["resource"] != "user" {
		t.Error("resource detail should survive round-trip")
	}
	if decoded.Details["extra"] != "data" {
		t.Error("extra detail should survive round-trip")
	}
}

func TestJSON_AppError_OmitsEmptyDetails(t *testing.T) {
	t.Parallel()
	err := TokenExpired()
	data, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		t.Fatalf("json.Marshal failed: %v", marshalErr)
	}
	if strings.Contains(string(data), `"details"`) {
		t.Errorf("empty details should be omitted from JSON, got %s", data)
	}
}

func TestJSON_AppError_IncludesDetails(t *testing.T) {
	t.Parallel()
	err := NotFound("user", "1")
	data, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		t.Fatalf("json.Marshal failed: %v", marshalErr)
	}
	if !strings.Contains(string(data), `"details"`) {
		t.Errorf("non-empty details should be in JSON, got %s", data)
	}
}

// ---------------------------------------------------------------------------
// 10. Wrap function
// ---------------------------------------------------------------------------

func TestWrap_WrappedAppError_Preserves(t *testing.T) {
	t.Parallel()
	orig := Timeout("op").WithDetail("key", "val")
	wrapped := fmt.Errorf("context: %w", orig)
	got := Wrap(wrapped)
	if got.Code != ErrCodeTimeout {
		t.Errorf("code = %s, want TIMEOUT", got.Code)
	}
	if got.Details["key"] != "val" {
		t.Error("details should be preserved through wrap")
	}
}

func TestWrap_PlainError_SetsInternal(t *testing.T) {
	t.Parallel()
	plain := fmt.Errorf("something went wrong")
	got := Wrap(plain)
	if got.Code != ErrCodeInternal {
		t.Errorf("code = %s, want INTERNAL_ERROR", got.Code)
	}
	if got.HTTPStatus != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", got.HTTPStatus)
	}
}

// ---------------------------------------------------------------------------
// 11. FormatResourceError additional tests
// ---------------------------------------------------------------------------

func TestFormatResourceError_IntegerTypes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		id   any
		want string
	}{
		{"int", 42, "42"},
		{"int64", int64(9999999999), "9999999999"},
		{"string", "abc-123", "abc-123"},
		{"float", 3.14, "3.14"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := FormatResourceError("res", tc.id)
			if err.Details["id"] != tc.want {
				t.Errorf("id = %v, want %s", err.Details["id"], tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 12. New() constructor with various codes
// ---------------------------------------------------------------------------

func TestNew_SetsRetryableFromCode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		code      ErrorCode
		retryable bool
	}{
		{ErrCodeTimeout, true},
		{ErrCodeServiceUnavailable, true},
		{ErrCodeConnectionFailed, true},
		{ErrCodeRateLimited, true},
		{ErrCodeExternalService, true},
		{ErrCodeNotFound, false},
		{ErrCodeInternal, false},
		{ErrCodeInvalidInput, false},
		{ErrorCode("CUSTOM_CODE"), false},
	}
	for _, tc := range tests {
		t.Run(string(tc.code), func(t *testing.T) {
			t.Parallel()
			err := New(tc.code, "msg", 500)
			if err.Retryable != tc.retryable {
				t.Errorf("New(%s).Retryable = %v, want %v", tc.code, err.Retryable, tc.retryable)
			}
		})
	}
}

func TestNew_CustomCode(t *testing.T) {
	t.Parallel()
	err := New(ErrorCode("CUSTOM"), "custom msg", 418)
	if err.Code != ErrorCode("CUSTOM") {
		t.Errorf("code = %s, want CUSTOM", err.Code)
	}
	if err.HTTPStatus != 418 {
		t.Errorf("status = %d, want 418", err.HTTPStatus)
	}
	if err.Message != "custom msg" {
		t.Errorf("message = %q, want 'custom msg'", err.Message)
	}
}
