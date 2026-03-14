package provider

import (
	"context"
	"errors"
	"testing"

	goerrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/resilience"
)

func TestWrapResilienceError_Nil(t *testing.T) {
	if wrapResilienceError(nil) != nil {
		t.Error("nil should return nil")
	}
}

func TestWrapResilienceError_AlreadyAppError(t *testing.T) {
	original := goerrors.NotFound("user", "123")
	err := wrapResilienceError(original)
	if err != original {
		t.Error("existing AppErrors should pass through unchanged")
	}
}

func TestWrapResilienceError_CircuitOpen(t *testing.T) {
	err := wrapResilienceError(resilience.ErrCircuitOpen)
	appErr, ok := goerrors.AsAppError(err)
	if !ok {
		t.Fatal("expected AppError")
	}
	if appErr.HTTPStatus != 503 {
		t.Errorf("expected 503, got %d", appErr.HTTPStatus)
	}
}

func TestWrapResilienceError_RateLimited(t *testing.T) {
	err := wrapResilienceError(resilience.ErrRateLimited)
	appErr, ok := goerrors.AsAppError(err)
	if !ok {
		t.Fatal("expected AppError")
	}
	if appErr.HTTPStatus != 429 {
		t.Errorf("expected 429, got %d", appErr.HTTPStatus)
	}
}

func TestWrapResilienceError_BulkheadFull(t *testing.T) {
	err := wrapResilienceError(resilience.ErrBulkheadFull)
	appErr, ok := goerrors.AsAppError(err)
	if !ok {
		t.Fatal("expected AppError")
	}
	if appErr.HTTPStatus != 503 {
		t.Errorf("expected 503, got %d", appErr.HTTPStatus)
	}
}

func TestWrapResilienceError_BulkheadTimeout(t *testing.T) {
	err := wrapResilienceError(resilience.ErrBulkheadTimeout)
	appErr, ok := goerrors.AsAppError(err)
	if !ok {
		t.Fatal("expected AppError")
	}
	if appErr.HTTPStatus != 503 {
		t.Errorf("expected 503, got %d", appErr.HTTPStatus)
	}
}

func TestWrapResilienceError_ContextCanceled(t *testing.T) {
	err := wrapResilienceError(context.Canceled)
	appErr, ok := goerrors.AsAppError(err)
	if !ok {
		t.Fatal("expected AppError")
	}
	if appErr.HTTPStatus != 504 {
		t.Errorf("expected 504 (gateway timeout), got %d", appErr.HTTPStatus)
	}
}

func TestWrapResilienceError_DeadlineExceeded(t *testing.T) {
	err := wrapResilienceError(context.DeadlineExceeded)
	appErr, ok := goerrors.AsAppError(err)
	if !ok {
		t.Fatal("expected AppError")
	}
	if appErr.HTTPStatus != 504 {
		t.Errorf("expected 504 (gateway timeout), got %d", appErr.HTTPStatus)
	}
}

func TestWrapResilienceError_Unknown(t *testing.T) {
	original := errors.New("some unknown error")
	err := wrapResilienceError(original)
	if err != original {
		t.Error("unknown errors should be returned as-is")
	}
}
