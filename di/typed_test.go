package di_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kbukum/gokit/di"
)

type (
	svc   struct{ N int }
	other struct{ N int }
)

var (
	svcKey   = di.NameKey[*svc]("svc")
	otherKey = di.NameKey[*other]("svc") // same name, different type — must not collide
	intKey   = di.NameKey[int]("count")
)

func TestProvide_ResolveKey(t *testing.T) {
	c := di.NewContainer()
	if err := di.Provide(c, svcKey, func() (*svc, error) {
		return &svc{N: 42}, nil
	}); err != nil {
		t.Fatalf("Provide: %v", err)
	}
	got, err := di.ResolveKey(c, svcKey)
	if err != nil {
		t.Fatalf("ResolveKey: %v", err)
	}
	if got.N != 42 {
		t.Fatalf("got.N = %d want 42", got.N)
	}
}

func TestProvide_NoErrorReturn(t *testing.T) {
	c := di.NewContainer()
	if err := di.Provide(c, svcKey, func() *svc { return &svc{N: 7} }); err != nil {
		t.Fatalf("Provide: %v", err)
	}
	got, err := di.ResolveKey(c, svcKey)
	if err != nil {
		t.Fatalf("ResolveKey: %v", err)
	}
	if got.N != 7 {
		t.Fatalf("got.N = %d want 7", got.N)
	}
}

func TestProvide_KeysWithSameNameDistinctTypes(t *testing.T) {
	c := di.NewContainer()
	if err := di.Provide(c, svcKey, func() *svc { return &svc{N: 1} }); err != nil {
		t.Fatalf("Provide svc: %v", err)
	}
	if err := di.Provide(c, otherKey, func() *other { return &other{N: 2} }); err != nil {
		t.Fatalf("Provide other (same name, different T): %v", err)
	}
	s, err := di.ResolveKey(c, svcKey)
	if err != nil || s.N != 1 {
		t.Fatalf("svc = %+v, err=%v", s, err)
	}
	o, err := di.ResolveKey(c, otherKey)
	if err != nil || o.N != 2 {
		t.Fatalf("other = %+v, err=%v", o, err)
	}
}

func TestProvideSingleton(t *testing.T) {
	c := di.NewContainer()
	if err := di.ProvideSingleton(c, intKey, 99); err != nil {
		t.Fatalf("ProvideSingleton: %v", err)
	}
	v, err := di.ResolveKey(c, intKey)
	if err != nil || v != 99 {
		t.Fatalf("got %d,%v want 99,nil", v, err)
	}
}

func TestProvide_RejectsBadCtor(t *testing.T) {
	c := di.NewContainer()
	cases := []struct {
		name string
		ctor any
	}{
		{"not a function", "hello"},
		{"wrong return type", func() *other { return &other{} }},
		{"too many returns", func() (*svc, error, int) { return nil, nil, 0 }}, //nolint:nilnil,staticcheck // test case intentionally exercises invalid ctor shape
		{"second not error", func() (*svc, *svc) { return nil, nil }},
		{"nil ctor", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := di.Provide(c, svcKey, tc.ctor); err == nil {
				t.Fatalf("expected error for %q", tc.name)
			}
		})
	}
}

func TestResolveKey_NotRegistered(t *testing.T) {
	c := di.NewContainer()
	if _, err := di.ResolveKey(c, svcKey); err == nil {
		t.Fatal("expected error for unregistered key")
	} else if !strings.Contains(err.Error(), "svc") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveKey_PropagatesCtorError(t *testing.T) {
	c := di.NewContainer()
	want := errors.New("boom")
	if err := di.Provide(c, svcKey, func() (*svc, error) { return nil, want }); err != nil {
		t.Fatalf("Provide: %v", err)
	}
	_, err := di.ResolveKey(c, svcKey)
	if err == nil {
		t.Fatal("expected ResolveKey to return ctor error")
	}
}

func TestProvide_NilContainer(t *testing.T) {
	if err := di.Provide(nil, svcKey, func() *svc { return nil }); err == nil {
		t.Fatal("expected error for nil container")
	}
	if _, err := di.ResolveKey[*svc](nil, svcKey); err == nil {
		t.Fatal("expected error for nil container")
	}
}

func TestKey_Name(t *testing.T) {
	if svcKey.Name() != "svc" {
		t.Fatalf("Name() = %q want svc", svcKey.Name())
	}
}

// Verify that the typed surface coexists with the legacy string-keyed API:
// resolution through Provide does not pollute the string namespace.
func TestProvide_NoStringNamespaceCollision(t *testing.T) {
	c := di.NewContainer()
	if err := di.Provide(c, svcKey, func() *svc { return &svc{N: 1} }); err != nil {
		t.Fatalf("Provide: %v", err)
	}
	// The bare name "svc" must not be resolvable — the typed key qualifies it.
	if _, err := c.Resolve("svc"); err == nil {
		t.Fatal("expected raw string \"svc\" to be unregistered (typed key qualifies it)")
	}
}

func TestMustResolveKey(t *testing.T) {
	c := di.NewContainer()
	if err := di.Provide(c, svcKey, func() *svc { return &svc{N: 5} }); err != nil {
		t.Fatalf("Provide: %v", err)
	}
	got := di.MustResolveKey(c, svcKey)
	if got.N != 5 {
		t.Fatalf("MustResolveKey = %+v", got)
	}
}

// Sanity: context.Context is a tricky generic type — ensure Key[context.Context]
// works (pure compile-time check + a noop resolve).
func TestKey_ContextType(t *testing.T) {
	ctxKey := di.NameKey[context.Context]("ctx")
	c := di.NewContainer()
	if err := di.ProvideSingleton(c, ctxKey, context.Background()); err != nil {
		t.Fatalf("ProvideSingleton: %v", err)
	}
	if _, err := di.ResolveKey(c, ctxKey); err != nil {
		t.Fatalf("ResolveKey: %v", err)
	}
}
