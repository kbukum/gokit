package apikey

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/auth"
	"github.com/kbukum/gokit/util"
)

func TestIndexedAuthenticatorClockAndCeiling(t *testing.T) {
	clock := util.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	m := NewManager(newMemStore(), testHasher(t), WithClock(clock))
	expires := clock.Now().Add(time.Minute)
	issued, _, err := m.IssueKey(context.Background(), IssueRequest{KeyID: "id", OwnerID: "u", Prefix: "key", Kind: auth.Service, RestrictionMode: auth.Restricted, Resources: []string{"one"}, Scopes: []string{"read"}, ExpiresAt: &expires})
	if err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest("POST", "/", http.NoBody)
	r.Header.Set("X-API-Key", issued.PlainKey)
	p, err := m.Authenticate(r)
	if err != nil || p.Kind != auth.Service || p.Reference == issued.PlainKey || !p.Allows("one", "read") || p.Allows("two", "read") {
		t.Fatal(err, p)
	}
	clock.Advance(time.Minute)
	if _, err := m.Authenticate(r); err == nil {
		t.Fatal("expiry boundary")
	}
	r.Header.Set("X-API-Key", strings.Repeat("a", 513))
	if _, err := m.Authenticate(r); err == nil {
		t.Fatal("oversized key")
	}
}

func TestMetadataCloneAndPrefixBound(t *testing.T) {
	hasher := testHasher(t)
	if _, err := hasher.GenerateKey(strings.Repeat("x", 513)); err == nil {
		t.Fatal("oversized generated key")
	}
	expires := time.Now().Add(time.Hour)
	key := &Key{ExpiresAt: &expires, GraceEndsAt: &expires, LastUsedAt: &expires, Scopes: []string{"read"}, Resources: []string{"one"}}
	cloned := key.Clone()
	*cloned.ExpiresAt = cloned.ExpiresAt.Add(time.Hour)
	cloned.Scopes[0] = "write"
	cloned.Resources[0] = "two"
	if !key.ExpiresAt.Equal(expires) || key.Scopes[0] != "read" || key.Resources[0] != "one" {
		t.Fatal("clone alias")
	}
	if (*Key)(nil).Clone() != nil {
		t.Fatal("nil clone")
	}
}
