package sqlite_test

import (
	"testing"

	"github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/sqlite"
)

func TestRegisterAddsDriverToRegistry(t *testing.T) {
	reg := database.NewDriverRegistry()
	if err := sqlite.Register(reg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	driver, ok := reg.Get(sqlite.Name)
	if !ok {
		t.Fatalf("driver %q not found after Register", sqlite.Name)
	}
	if driver(":memory:") == nil {
		t.Fatal("registered driver returned nil dialector")
	}
}

func TestRegisterRejectsDuplicate(t *testing.T) {
	reg := database.NewDriverRegistry()
	if err := sqlite.Register(reg); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := sqlite.Register(reg); err == nil {
		t.Fatal("expected duplicate Register to fail")
	}
}
