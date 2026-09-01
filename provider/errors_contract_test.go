package provider_test

import (
	"context"
	"testing"

	goerrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/provider"
)

// Registration and selection failures are typed AppErrors carrying stable
// error codes, not untyped strings, so callers can branch on the code and map
// to an HTTP status without string matching.

func TestRegistryCreateUnregisteredIsTypedNotFound(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry[*testProvider]()
	_, err := reg.Create("missing", nil)
	appErr, ok := goerrors.AsAppError(err)
	if !ok {
		t.Fatalf("expected AppError, got %T", err)
	}
	if appErr.Code != goerrors.ErrCodeNotFound {
		t.Fatalf("code = %q, want NOT_FOUND", appErr.Code)
	}
	if appErr.HTTPStatus != 404 {
		t.Fatalf("status = %d, want 404", appErr.HTTPStatus)
	}
}

func TestManagerGetByNameMissingIsTypedNotFound(t *testing.T) {
	t.Parallel()

	mgr := provider.NewManager[*testProvider](
		provider.NewRegistry[*testProvider](),
		&provider.PrioritySelector[*testProvider]{},
	)
	_, err := mgr.GetByName("missing")
	appErr, ok := goerrors.AsAppError(err)
	if !ok {
		t.Fatalf("expected AppError, got %T", err)
	}
	if appErr.Code != goerrors.ErrCodeNotFound {
		t.Fatalf("code = %q, want NOT_FOUND", appErr.Code)
	}
}

func TestPrioritySelectorNoneAvailableIsTypedUnavailable(t *testing.T) {
	t.Parallel()

	sel := &provider.PrioritySelector[*testProvider]{Priority: []string{"a"}}
	_, err := sel.Select(context.Background(), map[string]*testProvider{
		"a": {name: "a", available: false},
	})
	appErr, ok := goerrors.AsAppError(err)
	if !ok {
		t.Fatalf("expected AppError, got %T", err)
	}
	if appErr.Code != goerrors.ErrCodeServiceUnavailable {
		t.Fatalf("code = %q, want SERVICE_UNAVAILABLE", appErr.Code)
	}
	if !appErr.Retryable {
		t.Fatal("service-unavailable selection failure should be retryable")
	}
}
