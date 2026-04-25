package llm

import (
	"sort"
	"strings"
	"testing"
)

func TestDialectRegistry_RegisterAndGet(t *testing.T) {
	t.Parallel()

	reg := NewDialectRegistry()
	mock := &mockDialect{name: "test-provider"}

	if err := reg.Register("test-provider", mock); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, err := reg.Get("test-provider")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name() != "test-provider" {
		t.Errorf("Name() = %q, want %q", got.Name(), "test-provider")
	}
}

func TestDialectRegistry_GetUnknown(t *testing.T) {
	t.Parallel()

	reg := NewDialectRegistry()
	_, err := reg.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown dialect")
	}
}

func TestDialectRegistry_Names(t *testing.T) {
	t.Parallel()

	reg := NewDialectRegistry()
	reg.MustRegister("alpha", &mockDialect{name: "alpha"})
	reg.MustRegister("beta", &mockDialect{name: "beta"})

	names := reg.Names()
	sort.Strings(names)
	if len(names) != 2 || names[0] != "alpha" || names[1] != "beta" {
		t.Errorf("Names() = %v, want [alpha beta]", names)
	}
}

func TestDialectRegistry_DuplicateRegistration(t *testing.T) {
	t.Parallel()

	reg := NewDialectRegistry()
	if err := reg.Register("dup", &mockDialect{name: "first"}); err != nil {
		t.Fatalf("first register: %v", err)
	}
	err := reg.Register("dup", &mockDialect{name: "second"})
	if err == nil {
		t.Fatal("expected error on duplicate registration")
	}
	if !strings.Contains(err.Error(), "already registered") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDialectRegistry_RejectNilOrEmpty(t *testing.T) {
	t.Parallel()

	reg := NewDialectRegistry()

	if err := reg.Register("", &mockDialect{}); err == nil {
		t.Error("expected error for empty name")
	}
	if err := reg.Register("nil-d", nil); err == nil {
		t.Error("expected error for nil dialect")
	}
}

func TestDialectRegistry_MustRegisterPanicsOnError(t *testing.T) {
	t.Parallel()

	reg := NewDialectRegistry()
	reg.MustRegister("ok", &mockDialect{name: "ok"})

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate MustRegister")
		}
	}()
	reg.MustRegister("ok", &mockDialect{name: "ok"})
}
