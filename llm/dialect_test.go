package llm

import (
	"sort"
	"testing"
)

func TestRegisterDialect_And_GetDialect(t *testing.T) {
	// Clean state
	dialectsMu.Lock()
	original := dialects
	dialects = map[string]Dialect{}
	dialectsMu.Unlock()
	defer func() {
		dialectsMu.Lock()
		dialects = original
		dialectsMu.Unlock()
	}()

	mock := &mockDialect{name: "test-provider"}
	RegisterDialect("test-provider", mock)

	got, err := GetDialect("test-provider")
	if err != nil {
		t.Fatalf("GetDialect() error: %v", err)
	}
	if got.Name() != "test-provider" {
		t.Errorf("Name() = %q, want %q", got.Name(), "test-provider")
	}
}

func TestGetDialect_Unknown(t *testing.T) {
	_, err := GetDialect("nonexistent-dialect-xyz")
	if err == nil {
		t.Fatal("expected error for unknown dialect")
	}
}

func TestDialects_ListsRegistered(t *testing.T) {
	dialectsMu.Lock()
	original := dialects
	dialects = map[string]Dialect{}
	dialectsMu.Unlock()
	defer func() {
		dialectsMu.Lock()
		dialects = original
		dialectsMu.Unlock()
	}()

	RegisterDialect("alpha", &mockDialect{name: "alpha"})
	RegisterDialect("beta", &mockDialect{name: "beta"})

	names := Dialects()
	sort.Strings(names)
	if len(names) != 2 || names[0] != "alpha" || names[1] != "beta" {
		t.Errorf("Dialects() = %v, want [alpha beta]", names)
	}
}

func TestRegisterDialect_Overwrites(t *testing.T) {
	dialectsMu.Lock()
	original := dialects
	dialects = map[string]Dialect{}
	dialectsMu.Unlock()
	defer func() {
		dialectsMu.Lock()
		dialects = original
		dialectsMu.Unlock()
	}()

	RegisterDialect("dup", &mockDialect{name: "first"})
	RegisterDialect("dup", &mockDialect{name: "second"})

	got, _ := GetDialect("dup")
	if got.Name() != "second" {
		t.Errorf("expected overwritten dialect, got %q", got.Name())
	}
}
