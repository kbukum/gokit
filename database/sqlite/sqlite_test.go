package sqlite_test

import (
	"testing"

	"gorm.io/gorm"

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
	opener, err := d.Prepare(t.Context(), database.ConnectionInput{DSN: ":memory:"})
	if err != nil || opener == nil {
		t.Fatal("registered dialect did not prepare a connection")
	}
}

func preparedDialector(t *testing.T, dsn string) gorm.Dialector {
	t.Helper()
	opener, err := sqlite.Prepare(t.Context(), database.ConnectionInput{DSN: dsn})
	if err != nil {
		t.Fatal(err)
	}
	dialector, err := opener.Open(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return dialector
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
