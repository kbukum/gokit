package di_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/kbukum/gokit/di"
	apperr "github.com/kbukum/gokit/errors"
)

// Resolution failures are typed AppErrors: an unregistered key is a NotFound
// and a circular dependency is a Conflict, so callers branch on the code rather
// than parsing message text.

func TestResolveUnregisteredIsTypedNotFound(t *testing.T) {
	t.Parallel()

	c := di.NewContainer()
	_, err := di.Resolve[int](context.Background(), c)
	appErr, ok := apperr.AsAppError(err)
	if !ok {
		t.Fatalf("expected AppError, got %T", err)
	}
	if appErr.Code != apperr.ErrCodeNotFound {
		t.Fatalf("code = %q, want NOT_FOUND", appErr.Code)
	}
	if appErr.HTTPStatus != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", appErr.HTTPStatus)
	}
}

func TestResolveCircularIsTypedConflict(t *testing.T) {
	t.Parallel()

	c := di.NewContainer()
	_ = di.RegisterSingleton(c, func(ctx context.Context) (*svc, error) {
		return di.Resolve[*svc](ctx, c)
	})
	_, err := di.Resolve[*svc](context.Background(), c)
	appErr, ok := apperr.AsAppError(err)
	if !ok {
		t.Fatalf("expected AppError, got %T", err)
	}
	if appErr.Code != apperr.ErrCodeConflict {
		t.Fatalf("code = %q, want CONFLICT", appErr.Code)
	}
	if appErr.HTTPStatus != http.StatusConflict {
		t.Fatalf("status = %d, want 409", appErr.HTTPStatus)
	}
}

func TestRegisterNilContainerIsTypedInvalidInput(t *testing.T) {
	t.Parallel()

	err := di.Register[int](nil, 1)
	appErr, ok := apperr.AsAppError(err)
	if !ok {
		t.Fatalf("expected AppError, got %T", err)
	}
	if appErr.Code != apperr.ErrCodeInvalidInput {
		t.Fatalf("code = %q, want INVALID_INPUT", appErr.Code)
	}
	if appErr.HTTPStatus != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", appErr.HTTPStatus)
	}
}
