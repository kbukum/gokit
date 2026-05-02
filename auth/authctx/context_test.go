package authctx

import (
	"context"
	"errors"
	"testing"
)

type testClaims struct {
	Subject string
}

func TestSetAndGet(t *testing.T) {
	t.Parallel()

	ctx := Set(context.Background(), &testClaims{Subject: "user-1"})
	claims, ok := Get[*testClaims](ctx)
	if !ok || claims.Subject != "user-1" {
		t.Fatalf("unexpected claims: %+v ok=%v", claims, ok)
	}
}

func TestGet_MissingOrWrongType(t *testing.T) {
	t.Parallel()

	if _, ok := Get[*testClaims](context.Background()); ok {
		t.Fatal("expected missing claims to return ok=false")
	}

	ctx := Set(context.Background(), "not-claims")
	if _, ok := Get[*testClaims](ctx); ok {
		t.Fatal("expected wrong type to return ok=false")
	}
}

func TestGetOrError(t *testing.T) {
	t.Parallel()

	ctx := Set(context.Background(), &testClaims{Subject: "user-1"})
	claims, err := GetOrError[*testClaims](ctx)
	if err != nil || claims.Subject != "user-1" {
		t.Fatalf("unexpected result: %+v %v", claims, err)
	}

	_, err = GetOrError[*testClaims](context.Background())
	if !errors.Is(err, ErrNoClaims) {
		t.Fatalf("expected ErrNoClaims, got %v", err)
	}
}
