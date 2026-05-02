package oidc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestJWKSRefreshFailureDoesNotStartForcedCooldown(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Error(w, "temporary failure", http.StatusInternalServerError)
	}))
	defer srv.Close()

	cache := &jwksCache{
		jwksURI:  srv.URL,
		cacheTTL: time.Hour,
		keys:     map[string]*jwk{"existing": {Kid: "existing", Kty: "RSA"}},
	}

	client := srv.Client()
	if err := cache.refresh(context.Background(), client, true); err == nil {
		t.Fatal("expected forced refresh to fail")
	}
	if !cache.lastForcedRefreshAt.IsZero() {
		t.Fatal("failed refresh should not update forced refresh cooldown")
	}
	if err := cache.refresh(context.Background(), client, true); err == nil {
		t.Fatal("expected second forced refresh to fail")
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("expected second forced refresh attempt without cooldown, got %d hits", got)
	}
}

func TestJWKSForcedRefreshIsSerialized(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		time.Sleep(25 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"keys":[{"kid":"rsa-1","kty":"RSA","n":"AQAB","e":"AQAB","use":"sig"}]}`))
	}))
	defer srv.Close()

	cache := &jwksCache{
		jwksURI:  srv.URL,
		cacheTTL: time.Hour,
		keys:     map[string]*jwk{"existing": {Kid: "existing", Kty: "RSA"}},
	}

	const callers = 4
	errCh := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- cache.refresh(context.Background(), srv.Client(), true)
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("forced refresh failed: %v", err)
		}
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("expected exactly one forced refresh request, got %d", got)
	}
}
