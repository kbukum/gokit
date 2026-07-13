package httpclient

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
)

func TestErrorCode_String(t *testing.T) {
	tests := []struct {
		code ErrorCode
		want string
	}{
		{ErrCodeTimeout, "timeout"},
		{ErrCodeConnection, "connection"},
		{ErrCodeAuth, "auth"},
		{ErrCodeNotFound, "not_found"},
		{ErrCodeRateLimit, "rate_limit"},
		{ErrCodeValidation, "validation"},
		{ErrCodeServer, "server"},
		{ErrorCode(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.code.String(); got != tt.want {
			t.Errorf("ErrorCode(%d).String() = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestError_Error(t *testing.T) {
	e := &Error{StatusCode: 404, Code: ErrCodeNotFound, Message: "HTTP 404"}
	want := "httpclient: not_found (HTTP 404): HTTP 404"
	if got := e.Error(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	e2 := &Error{Code: ErrCodeConnection, Message: "connection refused"}
	want2 := "httpclient: connection: connection refused"
	if got := e2.Error(); got != want2 {
		t.Errorf("got %q, want %q", got, want2)
	}
}

func TestError_Unwrap(t *testing.T) {
	inner := NewValidationError("bad input")
	outer := &Error{Code: ErrCodeServer, Message: "wrapped", Err: inner}
	if !errors.Is(outer.Unwrap(), inner) {
		t.Error("Unwrap did not return inner error")
	}
}

func TestClassifyStatusCode(t *testing.T) {
	tests := []struct {
		code    int
		wantNil bool
		errCode ErrorCode
		retry   bool
	}{
		{200, true, 0, false},
		{201, true, 0, false},
		{204, true, 0, false},
		{400, false, ErrCodeValidation, false},
		{401, false, ErrCodeAuth, false},
		{403, false, ErrCodeAuth, false},
		{404, false, ErrCodeNotFound, false},
		{429, false, ErrCodeRateLimit, true},
		{500, false, ErrCodeServer, true},
		{502, false, ErrCodeServer, true},
		{503, false, ErrCodeServer, true},
	}
	for _, tt := range tests {
		e := ClassifyStatusCode(tt.code, nil)
		if tt.wantNil {
			if e != nil {
				t.Errorf("ClassifyStatusCode(%d): expected nil, got %v", tt.code, e)
			}
			continue
		}
		if e == nil {
			t.Errorf("ClassifyStatusCode(%d): expected error, got nil", tt.code)
			continue
		}
		if e.Code != tt.errCode {
			t.Errorf("ClassifyStatusCode(%d): code = %v, want %v", tt.code, e.Code, tt.errCode)
		}
		if e.Retryable != tt.retry {
			t.Errorf("ClassifyStatusCode(%d): retryable = %v, want %v", tt.code, e.Retryable, tt.retry)
		}
	}
}

func TestIsHelpers(t *testing.T) {
	timeout := NewTimeoutError(fmt.Errorf("timed out"))
	conn := NewConnectionError(fmt.Errorf("connection refused"))
	auth := NewAuthError(401, nil)
	notFound := NewNotFoundError(nil)
	rateLimit := NewRateLimitError(nil)
	server := NewServerError(500, nil)
	validation := NewValidationError("bad")

	if !IsTimeout(timeout) {
		t.Error("IsTimeout should match timeout error")
	}
	if !IsConnection(conn) {
		t.Error("IsConnection should match connection error")
	}
	if !IsAuth(auth) {
		t.Error("IsAuth should match auth error")
	}
	if !IsNotFound(notFound) {
		t.Error("IsNotFound should match not-found error")
	}
	if !IsRateLimit(rateLimit) {
		t.Error("IsRateLimit should match rate-limit error")
	}
	if !IsServerError(server) {
		t.Error("IsServerError should match server error")
	}
	if !IsRetryable(timeout) {
		t.Error("timeout should be retryable")
	}
	if !IsRetryable(conn) {
		t.Error("connection should be retryable")
	}
	if !IsRetryable(server) {
		t.Error("server error should be retryable")
	}
	if IsRetryable(auth) {
		t.Error("auth should not be retryable")
	}
	if IsRetryable(validation) {
		t.Error("validation should not be retryable")
	}
}

func TestClassifyStatusCode_NonStandardCodes(t *testing.T) {
	tests := []struct {
		code      int
		wantCode  ErrorCode
		retryable bool
	}{
		{418, ErrCodeValidation, false}, // I'm a teapot
		{451, ErrCodeValidation, false}, // Unavailable For Legal Reasons
		{502, ErrCodeServer, true},      // Bad Gateway
		{599, ErrCodeServer, true},      // custom 5xx
		{100, ErrCodeServer, false},     // Informational - falls to default
		{301, ErrCodeServer, false},     // Redirect - falls to default
	}
	for _, tt := range tests {
		e := ClassifyStatusCode(tt.code, []byte("body"))
		if e == nil {
			t.Errorf("code %d: expected error, got nil", tt.code)
			continue
		}
		if e.Code != tt.wantCode {
			t.Errorf("code %d: got Code=%v, want %v", tt.code, e.Code, tt.wantCode)
		}
		if e.Retryable != tt.retryable {
			t.Errorf("code %d: got Retryable=%v, want %v", tt.code, e.Retryable, tt.retryable)
		}
	}
}

func TestClassifyStatusCode_2xx_ReturnsNil(t *testing.T) {
	for code := 200; code < 300; code++ {
		if e := ClassifyStatusCode(code, nil); e != nil {
			t.Errorf("code %d: expected nil, got %v", code, e)
		}
	}
}

func TestClassifyStatusCode_BodyPreserved(t *testing.T) {
	body := []byte(`{"error":"detail"}`)
	e := ClassifyStatusCode(500, body)
	if e == nil {
		t.Fatal("expected error")
	}
	if !bytes.Equal(e.Body, body) {
		t.Error("body not preserved in error")
	}
}

func TestError_Unwrap_Chaining(t *testing.T) {
	inner := fmt.Errorf("connection refused")
	e := NewConnectionError(inner)
	if !errors.Is(e.Unwrap(), inner) {
		t.Error("Unwrap did not return inner error")
	}
}

func TestIsCheckers_NonHttpError(t *testing.T) {
	plain := fmt.Errorf("some error")
	if IsTimeout(plain) || IsConnection(plain) || IsAuth(plain) ||
		IsNotFound(plain) || IsRateLimit(plain) || IsServerError(plain) || IsRetryable(plain) {
		t.Error("Is* helpers should return false for non-httpclient errors")
	}
}

func TestError_ErrorFormat_WithStatusCode(t *testing.T) {
	e := NewServerError(503, nil)
	want := "httpclient: server (HTTP 503): HTTP 503"
	if e.Error() != want {
		t.Errorf("Error() = %q, want %q", e.Error(), want)
	}
}

func TestError_ErrorFormat_WithoutStatusCode(t *testing.T) {
	e := NewTimeoutError(fmt.Errorf("dial timeout"))
	want := "httpclient: timeout: dial timeout"
	if e.Error() != want {
		t.Errorf("Error() = %q, want %q", e.Error(), want)
	}
}
