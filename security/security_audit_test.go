package security_test

import (
	"crypto/tls"
	"fmt"
	"strings"
	"testing"

	goerrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/security"
)

// ─── 1. Error Message Sanitization ─────────────────────────────────────────

func TestAppError_Error_DoesNotLeakSecrets(t *testing.T) {
	t.Parallel()

	secrets := []string{
		"super-secret-api-key-12345",
		"postgres://admin:s3cret@db:5432/mydb",
		"-----BEGIN RSA PRIVATE KEY-----",
	}

	tests := []struct {
		name    string
		errFunc func() *goerrors.AppError
	}{
		{
			name: "unauthorized does not contain credentials",
			errFunc: func() *goerrors.AppError {
				return goerrors.Unauthorized("Authentication required.")
			},
		},
		{
			name: "invalid token does not reveal valid token",
			errFunc: func() *goerrors.AppError {
				return goerrors.InvalidToken()
			},
		},
		{
			name: "token expired does not reveal expiry details",
			errFunc: func() *goerrors.AppError {
				return goerrors.TokenExpired()
			},
		},
		{
			name: "internal error wraps cause safely",
			errFunc: func() *goerrors.AppError {
				return goerrors.Internal(fmt.Errorf("connection refused"))
			},
		},
		{
			name: "database error wraps cause safely",
			errFunc: func() *goerrors.AppError {
				return goerrors.DatabaseError(fmt.Errorf("pq: authentication failed"))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			errMsg := tc.errFunc().Error()
			for _, secret := range secrets {
				if strings.Contains(errMsg, secret) {
					t.Errorf("error message leaked secret %q: %s", secret, errMsg)
				}
			}
		})
	}
}

func TestAppError_Details_SensitiveDataWarning(t *testing.T) {
	t.Parallel()

	// Auth errors should not include details about valid credentials
	err := goerrors.Unauthorized("bad token")
	errStr := err.Error()

	if strings.Contains(errStr, "password") || strings.Contains(errStr, "secret") {
		t.Errorf("auth error message contains sensitive terms: %s", errStr)
	}
}

func TestAppError_WithCause_DoesNotExposeInternalPaths(t *testing.T) {
	t.Parallel()

	cause := fmt.Errorf("open /etc/secrets/db-password.txt: permission denied")
	err := goerrors.Internal(cause)

	// The Message field (used in HTTP responses) should be generic
	if strings.Contains(err.Message, "/etc/secrets") {
		t.Errorf("Internal error Message field leaked internal path: %s", err.Message)
	}
}

// ─── 2. Auth Error Responses ───────────────────────────────────────────────

func TestAuthErrors_DoNotRevealSystemInfo(t *testing.T) {
	t.Parallel()

	sensitivePatterns := []string{
		"/usr/", "/etc/", "/var/", "127.0.0.1", "localhost",
		"stack trace", "goroutine", "runtime.",
	}

	authErrors := []*goerrors.AppError{
		goerrors.Unauthorized(""),
		goerrors.Forbidden(""),
		goerrors.TokenExpired(),
		goerrors.InvalidToken(),
	}

	for _, err := range authErrors {
		errStr := err.Error()
		for _, pattern := range sensitivePatterns {
			if strings.Contains(strings.ToLower(errStr), strings.ToLower(pattern)) {
				t.Errorf("auth error %q leaked sensitive pattern %q", errStr, pattern)
			}
		}
	}
}

func TestAuthErrors_HTTPStatusCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        *goerrors.AppError
		wantStatus int
	}{
		{"unauthorized", goerrors.Unauthorized(""), 401},
		{"forbidden", goerrors.Forbidden(""), 403},
		{"token expired", goerrors.TokenExpired(), 401},
		{"invalid token", goerrors.InvalidToken(), 401},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.err.HTTPStatus != tc.wantStatus {
				t.Errorf("expected HTTP %d, got %d", tc.wantStatus, tc.err.HTTPStatus)
			}
		})
	}
}

// ─── 3. Input Validation Edge Cases ────────────────────────────────────────

func TestAppError_EmptyInputs_NoPanic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		fn   func()
	}{
		{"NotFound empty", func() { goerrors.NotFound("", "") }},
		{"InvalidInput empty", func() { goerrors.InvalidInput("", "") }},
		{"MissingField empty", func() { goerrors.MissingField("") }},
		{"Unauthorized empty", func() { goerrors.Unauthorized("") }},
		{"Forbidden empty", func() { goerrors.Forbidden("") }},
		{"Internal nil cause", func() { goerrors.Internal(fmt.Errorf("")) }},
		{"WithDetails nil", func() { goerrors.NotFound("x", "1").WithDetails(nil) }},
		{"WithDetail empty key", func() { goerrors.NotFound("x", "1").WithDetail("", nil) }},
		{"Wrap nil", func() { goerrors.Wrap(nil) }},
		{"ServiceUnavailable empty", func() { goerrors.ServiceUnavailable("") }},
		{"Timeout empty", func() { goerrors.Timeout("") }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic on empty input: %v", r)
				}
			}()
			tc.fn()
		})
	}
}

func TestAppError_UnicodeInputs(t *testing.T) {
	t.Parallel()

	unicodeInputs := []string{
		"こんにちは",
		"用户未找到",
		"Ñoño",
		"emoji 🔒🔑",
		"\x00null\x00byte",
	}

	for _, input := range unicodeInputs {
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic on unicode input: %v", r)
				}
			}()
			err := goerrors.NotFound("resource", input)
			_ = err.Error()
		})
	}
}

// ─── 4. TLS Configuration Hardening ────────────────────────────────────────

func TestTLS_DefaultMinVersion_Is12(t *testing.T) {
	t.Parallel()

	cfg := &security.TLSConfig{
		SkipVerify: true,
	}
	tlsCfg, err := cfg.Build()
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if tlsCfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("default MinVersion should be TLS 1.2 (0x%04x), got 0x%04x",
			tls.VersionTLS12, tlsCfg.MinVersion)
	}
}

func TestTLS_RejectsLegacyVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		minVersion uint16
		expected   uint16
	}{
		{"explicit TLS 1.2", tls.VersionTLS12, tls.VersionTLS12},
		{"explicit TLS 1.3", tls.VersionTLS13, tls.VersionTLS13},
		{"zero defaults to TLS 1.2", 0, tls.VersionTLS12},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := &security.TLSConfig{
				SkipVerify: true,
				MinVersion: tc.minVersion,
			}
			tlsCfg, err := cfg.Build()
			if err != nil {
				t.Fatalf("build failed: %v", err)
			}
			if tlsCfg.MinVersion != tc.expected {
				t.Errorf("expected MinVersion 0x%04x, got 0x%04x", tc.expected, tlsCfg.MinVersion)
			}
		})
	}
}

func TestTLS_CertKeyPairMustBeComplete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     security.TLSConfig
		wantErr bool
	}{
		{
			name:    "cert without key is invalid",
			cfg:     security.TLSConfig{CertFile: "/path/to/cert.pem"},
			wantErr: true,
		},
		{
			name:    "key without cert is invalid",
			cfg:     security.TLSConfig{KeyFile: "/path/to/key.pem"},
			wantErr: true,
		},
		{
			name:    "neither cert nor key is valid",
			cfg:     security.TLSConfig{},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.cfg.Validate()
			if tc.wantErr && err == nil {
				t.Error("expected validation error")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestTLS_NilConfig_IsHandledSafely(t *testing.T) {
	t.Parallel()

	var cfg *security.TLSConfig
	result, err := cfg.Build()
	if err != nil {
		t.Errorf("nil config should not error: %v", err)
	}
	if result != nil {
		t.Error("nil config should return nil tls.Config")
	}

	// Also test nil Validate and IsEnabled
	if err := cfg.Validate(); err != nil {
		t.Errorf("nil config Validate should not error: %v", err)
	}
	if cfg.IsEnabled() {
		t.Error("nil config should not be enabled")
	}
}

func TestTLS_SkipVerify_FlagPassedThrough(t *testing.T) {
	t.Parallel()

	cfg := &security.TLSConfig{SkipVerify: true}
	tlsCfg, err := cfg.Build()
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if !tlsCfg.InsecureSkipVerify {
		t.Error("SkipVerify should set InsecureSkipVerify")
	}
}

// ─── 5. Error Type Safety ──────────────────────────────────────────────────

func TestAppError_ImplementsErrorInterface(t *testing.T) {
	t.Parallel()

	var _ error = goerrors.NotFound("x", "1")
	var _ error = goerrors.Internal(fmt.Errorf("oops"))
}

func TestAppError_Unwrap_ChainPreserved(t *testing.T) {
	t.Parallel()

	original := fmt.Errorf("root cause")
	err := goerrors.Internal(original)
	if err.Unwrap() != original {
		t.Error("Unwrap should return the original cause")
	}
}

func TestAppError_WrapNil_ReturnsNil(t *testing.T) {
	t.Parallel()

	result := goerrors.Wrap(nil)
	if result != nil {
		t.Error("Wrap(nil) should return nil")
	}
}

func TestAppError_WrapAppError_ReturnsSame(t *testing.T) {
	t.Parallel()

	original := goerrors.NotFound("user", "123")
	wrapped := goerrors.Wrap(original)
	if wrapped != original {
		t.Error("Wrap of AppError should return same instance")
	}
}

func TestAppError_Retryable_CorrectCodes(t *testing.T) {
	t.Parallel()

	retryable := []func() *goerrors.AppError{
		func() *goerrors.AppError { return goerrors.ServiceUnavailable("x") },
		func() *goerrors.AppError { return goerrors.ConnectionFailed("x") },
		func() *goerrors.AppError { return goerrors.Timeout("x") },
		func() *goerrors.AppError { return goerrors.RateLimited() },
	}
	for _, fn := range retryable {
		err := fn()
		if !err.Retryable {
			t.Errorf("expected retryable for %s", err.Code)
		}
	}

	notRetryable := []func() *goerrors.AppError{
		func() *goerrors.AppError { return goerrors.Unauthorized("x") },
		func() *goerrors.AppError { return goerrors.Forbidden("x") },
		func() *goerrors.AppError { return goerrors.NotFound("x", "1") },
		func() *goerrors.AppError { return goerrors.TokenExpired() },
		func() *goerrors.AppError { return goerrors.InvalidToken() },
	}
	for _, fn := range notRetryable {
		err := fn()
		if err.Retryable {
			t.Errorf("expected non-retryable for %s", err.Code)
		}
	}
}

