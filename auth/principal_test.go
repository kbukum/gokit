package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kbukum/gokit/auth/authctx"
)

func TestPrincipalCeilingAndIsolation(t *testing.T) {
	p := Principal{Subject: "caller", Kind: User, Credential: APIKey, Reference: "digest", Restrictions: Restrictions{Mode: Restricted, Resources: []string{"one"}, Scopes: []string{"read"}}}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	if !p.Allows("one", "read") || p.Allows("two", "read") || p.Allows("one", "write") {
		t.Fatal("invalid ceiling")
	}
	clone := p.Clone()
	clone.Restrictions.Resources[0] = "two"
	if !p.Allows("one", "read") {
		t.Fatal("aliasing")
	}
	ctx := authctx.Set(context.Background(), p)
	ctx = authctx.Set(ctx, "independent")
	if got, ok := authctx.Get[Principal](ctx); !ok || got.Subject != p.Subject {
		t.Fatal("type context collision")
	}
	p.Restrictions.Mode = ""
	if p.Allows("one", "read") || p.Validate() == nil {
		t.Fatal("invalid restrictions accepted")
	}
}

func TestChainAmbiguityAndAbsence(t *testing.T) {
	calls := 0
	a := RequestAuthenticatorFunc(func(*http.Request) (Principal, error) {
		calls++
		return Principal{Subject: "u", Kind: User, Credential: Session, Reference: "ref", ExpiresAt: time.Now().Add(time.Hour), Restrictions: Restrictions{Mode: Unrestricted}}, nil
	})
	for _, headers := range []http.Header{
		{"X-Api-Key": {""}},
		{"X-Api-Key": {"a", "b"}},
		{"Authorization": {""}},
		{"Cookie": {"__Host-session=a; __Host-session=b"}},
		{"Cookie": {"__Host-session="}},
		{"Cookie": {"__Host-session=a"}, "X-Api-Key": {"b"}},
	} {
		r := httptest.NewRequest("GET", "/", http.NoBody)
		r.Header = headers
		if _, present, err := NewChain(a, a).Authenticate(r); err == nil || !present {
			t.Fatalf("accepted %#v", headers)
		}
	}
	if calls != 0 {
		t.Fatal("ambiguous input reached validator")
	}
	p, present, err := NewChain(a, a).Authenticate(httptest.NewRequest("GET", "/", http.NoBody))
	if err != nil || present || p.Subject != "" {
		t.Fatal("absence must be reported without an identity", p, present, err)
	}
	r := httptest.NewRequest("GET", "/", http.NoBody)
	r.Header.Set("X-API-Key", "a")
	if p, present, err := NewChain(a, a).Authenticate(r); err != nil || !present || p.Subject != "u" || calls != 1 {
		t.Fatal(err)
	}
	if _, present, err := NewChain(nil, nil).Authenticate(r); err == nil || !present {
		t.Fatal("unconfigured validator accepted a presented credential")
	}
}

func FuzzCredentials(f *testing.F) {
	f.Add("__Host-session=x", "key", "")
	f.Fuzz(func(t *testing.T, cookie, key, authorization string) {
		r := httptest.NewRequest("GET", "/", http.NoBody)
		r.Header.Set("Cookie", cookie)
		r.Header.Set("X-API-Key", key)
		r.Header.Set("Authorization", authorization)
		_, _ = ParseCredentials(r)
	})
}
