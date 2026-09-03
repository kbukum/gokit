package sqlite_test

import (
	"testing"

	"github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/sqlite"
)

func TestRegisterAddsDialectToRegistry(t *testing.T) {
	reg := database.NewDialectRegistry()
	if err := sqlite.Register(reg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	d, ok := reg.Get(sqlite.Name)
	if !ok {
		t.Fatalf("dialect %q not found after Register", sqlite.Name)
	}
	if d.Open(":memory:") == nil {
		t.Fatal("registered dialect returned nil dialector")
	}
}

func TestRegisterRejectsDuplicate(t *testing.T) {
	reg := database.NewDialectRegistry()
	if err := sqlite.Register(reg); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := sqlite.Register(reg); err == nil {
		t.Fatal("expected duplicate Register to fail")
	}
}
