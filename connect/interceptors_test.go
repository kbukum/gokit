package connect

import (
	"context"
	"errors"
	"testing"

	connectrpc "connectrpc.com/connect"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
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

func TestErrorInterceptor(t *testing.T) {
	ctx := context.Background()
	req := connectrpc.NewRequest(&struct{}{})
	wantResp := connectrpc.NewResponse(&struct{}{})

	t.Run("success passes response", func(t *testing.T) {
		resp, err := ErrorInterceptor()(func(context.Context, connectrpc.AnyRequest) (connectrpc.AnyResponse, error) {
			return wantResp, nil
		})(ctx, req)
		if err != nil {
			t.Fatalf("ErrorInterceptor returned error: %v", err)
		}
		if resp != wantResp {
			t.Fatal("response was not passed through")
		}
	})

	t.Run("connect error passes through", func(t *testing.T) {
		wantErr := connectrpc.NewError(connectrpc.CodePermissionDenied, errors.New("denied"))
		resp, err := ErrorInterceptor()(func(context.Context, connectrpc.AnyRequest) (connectrpc.AnyResponse, error) {
			return wantResp, wantErr
		})(ctx, req)
		if resp != wantResp {
			t.Fatal("response was not passed through")
		}
		if !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want original connect error", err)
		}
	})

	t.Run("app error converts to connect error", func(t *testing.T) {
		resp, err := ErrorInterceptor()(func(context.Context, connectrpc.AnyRequest) (connectrpc.AnyResponse, error) {
			return wantResp, apperrors.Unauthorized("login required")
		})(ctx, req)
		if resp != wantResp {
			t.Fatal("response was not passed through")
		}
		if connectrpc.CodeOf(err) != connectrpc.CodeUnauthenticated {
			t.Fatalf("CodeOf(err) = %v, want unauthenticated", connectrpc.CodeOf(err))
		}
	})

	t.Run("plain error passes through", func(t *testing.T) {
		wantErr := errors.New("plain")
		_, err := ErrorInterceptor()(func(context.Context, connectrpc.AnyRequest) (connectrpc.AnyResponse, error) {
			return nil, wantErr
		})(ctx, req)
		if !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want original plain error", err)
		}
	})
}

func TestLoggingInterceptor(t *testing.T) {
	log := logging.New(&logging.Config{Level: "debug", Format: "json", Output: "stdout"}, "connect-test")
	req := connectrpc.NewRequest(&struct{}{})
	wantResp := connectrpc.NewResponse(&struct{}{})

	t.Run("success", func(t *testing.T) {
		resp, err := LoggingInterceptor(log)(func(context.Context, connectrpc.AnyRequest) (connectrpc.AnyResponse, error) {
			return wantResp, nil
		})(context.Background(), req)
		if err != nil {
			t.Fatalf("LoggingInterceptor returned error: %v", err)
		}
		if resp != wantResp {
			t.Fatal("response was not passed through")
		}
	})

	t.Run("connect error", func(t *testing.T) {
		wantErr := connectrpc.NewError(connectrpc.CodeUnavailable, errors.New("down"))
		resp, err := LoggingInterceptor(log)(func(context.Context, connectrpc.AnyRequest) (connectrpc.AnyResponse, error) {
			return wantResp, wantErr
		})(context.Background(), req)
		if resp != wantResp {
			t.Fatal("response was not passed through")
		}
		if !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want original connect error", err)
		}
	})

	t.Run("plain error", func(t *testing.T) {
		wantErr := errors.New("plain")
		_, err := LoggingInterceptor(log)(func(context.Context, connectrpc.AnyRequest) (connectrpc.AnyResponse, error) {
			return nil, wantErr
		})(context.Background(), req)
		if !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want original plain error", err)
		}
	})
}

func TestConnectCodeToAppError_AllCodes(t *testing.T) {
	cases := []struct {
		code     connectrpc.Code
		wantCode apperrors.ErrorCode
	}{
		{connectrpc.CodeNotFound, apperrors.ErrCodeNotFound},
		{connectrpc.CodeAlreadyExists, apperrors.ErrCodeAlreadyExists},
		{connectrpc.CodeInvalidArgument, apperrors.ErrCodeInvalidInput},
		{connectrpc.CodeUnauthenticated, apperrors.ErrCodeUnauthorized},
		{connectrpc.CodePermissionDenied, apperrors.ErrCodeForbidden},
		{connectrpc.CodeFailedPrecondition, apperrors.ErrCodeConflict},
		{connectrpc.CodeDeadlineExceeded, apperrors.ErrCodeTimeout},
		{connectrpc.CodeResourceExhausted, apperrors.ErrCodeRateLimited},
		{connectrpc.CodeUnavailable, apperrors.ErrCodeServiceUnavailable},
		{connectrpc.CodeCanceled, apperrors.ErrCodeInternal},
		{connectrpc.CodeInternal, apperrors.ErrCodeInternal},
	}

	for _, tc := range cases {
		appErr := connectCodeToAppError(tc.code, "message")
		if appErr == nil {
			t.Fatalf("connectCodeToAppError(%v) returned nil", tc.code)
		}
		if appErr.Code != tc.wantCode {
			t.Fatalf("connectCodeToAppError(%v).Code = %v, want %v", tc.code, appErr.Code, tc.wantCode)
		}
	}
}
