package oidc

import (
	"context"
	"net/http"
	"net/http/httptest"
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
