package connect

import (
	"testing"

	connectrpc "connectrpc.com/connect"

	apperrors "github.com/kbukum/gokit/errors"
)

// ---------------------------------------------------------------------------
// Mapping helpers
// ---------------------------------------------------------------------------

func TestAppErrorCodeToConnect_AllCodes(t *testing.T) {
	cases := []struct {
		code     apperrors.ErrorCode
		expected connectrpc.Code
	}{
		{apperrors.ErrCodeNotFound, connectrpc.CodeNotFound},
		{apperrors.ErrCodeAlreadyExists, connectrpc.CodeAlreadyExists},
		{apperrors.ErrCodeInvalidInput, connectrpc.CodeInvalidArgument},
		{apperrors.ErrCodeMissingField, connectrpc.CodeInvalidArgument},
		{apperrors.ErrCodeInvalidFormat, connectrpc.CodeInvalidArgument},
		{apperrors.ErrCodeUnauthorized, connectrpc.CodeUnauthenticated},
		{apperrors.ErrCodeTokenExpired, connectrpc.CodeUnauthenticated},
		{apperrors.ErrCodeInvalidToken, connectrpc.CodeUnauthenticated},
		{apperrors.ErrCodeForbidden, connectrpc.CodePermissionDenied},
		{apperrors.ErrCodeConflict, connectrpc.CodeFailedPrecondition},
		{apperrors.ErrCodeTimeout, connectrpc.CodeDeadlineExceeded},
		{apperrors.ErrCodeRateLimited, connectrpc.CodeResourceExhausted},
		{apperrors.ErrCodeServiceUnavailable, connectrpc.CodeUnavailable},
		{apperrors.ErrCodeConnectionFailed, connectrpc.CodeUnavailable},
		{apperrors.ErrCodeInternal, connectrpc.CodeInternal},
		{apperrors.ErrCodeDatabaseError, connectrpc.CodeInternal},
		{apperrors.ErrCodeExternalService, connectrpc.CodeInternal},
	}

	for _, tc := range cases {
		got := appErrorCodeToConnect(tc.code)
		if got != tc.expected {
			t.Errorf("appErrorCodeToConnect(%v) = %v, want %v", tc.code, got, tc.expected)
		}
	}
}

func TestSanitizeMessage(t *testing.T) {
	if got := sanitizeMessage(""); got != "An error occurred." {
		t.Errorf("expected default message, got %q", got)
	}
	if got := sanitizeMessage("hello"); got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

// ---------------------------------------------------------------------------
// ToConnectError / FromConnectError
// ---------------------------------------------------------------------------

func TestToConnectError_Nil(t *testing.T) {
	if ToConnectError(nil) != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestToConnectError_NotFound(t *testing.T) {
	appErr := apperrors.NotFound("user", "123")
	cErr := ToConnectError(appErr)
	if cErr == nil {
		t.Fatal("expected non-nil connect error")
	}
	if cErr.Code() != connectrpc.CodeNotFound {
		t.Fatalf("expected NotFound, got %v", cErr.Code())
	}
}

func TestToConnectError_Internal(t *testing.T) {
	appErr := apperrors.Internal(nil)
	cErr := ToConnectError(appErr)
	if cErr.Code() != connectrpc.CodeInternal {
		t.Fatalf("expected Internal, got %v", cErr.Code())
	}
}

func TestFromConnectError_Nil(t *testing.T) {
	if FromConnectError(nil) != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestFromConnectError_NotFound(t *testing.T) {
	cErr := connectrpc.NewError(connectrpc.CodeNotFound, nil)
	appErr := FromConnectError(cErr)
	if appErr == nil {
		t.Fatal("expected non-nil app error")
	}
	if appErr.Code != apperrors.ErrCodeNotFound {
		t.Fatalf("expected NotFound, got %v", appErr.Code)
	}
}

func TestFromConnectError_NonConnectError(t *testing.T) {
	appErr := FromConnectError(apperrors.Internal(nil))
	if appErr == nil {
		t.Fatal("expected non-nil app error")
	}
	if appErr.Code != apperrors.ErrCodeInternal {
		t.Fatalf("expected Internal, got %v", appErr.Code)
	}
}

