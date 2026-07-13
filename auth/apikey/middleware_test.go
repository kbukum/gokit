package apikey

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidatorFuncAdapter(t *testing.T) {
	t.Parallel()
	want := &Key{ID: "k1"}
	var fn Validator = ValidatorFunc(func(_ context.Context, plain string, scopes ...string) (*Key, error) {
		if plain != "secret" {
			t.Fatalf("plain = %q, want secret", plain)
		}
		return want, nil
	})
	got, err := fn.ValidateKey(context.Background(), "secret")
	if err != nil {
		t.Fatalf("ValidateKey: %v", err)
	}
	if got != want {
		t.Fatal("ValidatorFunc did not return underlying key")
	}
}

func TestMiddlewareWithHeaderAndContext(t *testing.T) {
	t.Parallel()
	stored := &Key{ID: "k1", OwnerID: "owner"}
	v := ValidatorFunc(func(context.Context, string, ...string) (*Key, error) { return stored, nil })

	var fromCtx *Key
	handler := Middleware(v, WithHeader("Authorization"))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fromCtx = FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("Authorization", "some-key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if fromCtx != stored {
		t.Fatal("validated key not stored in context")
	}
}

func TestMiddlewareSkipsConfiguredPaths(t *testing.T) {
	t.Parallel()
	v := ValidatorFunc(func(context.Context, string, ...string) (*Key, error) {
		t.Fatal("validator should not run on skipped path")
		return nil, errors.New("unreachable")
	})
	handler := Middleware(v, WithSkipPaths("/health"))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", http.NoBody)
	req.Header.Set("X-API-Key", "irrelevant")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
}

func TestFromContextReturnsNilWhenAbsent(t *testing.T) {
	t.Parallel()
	if key := FromContext(context.Background()); key != nil {
		t.Fatalf("FromContext = %v, want nil", key)
	}
}
