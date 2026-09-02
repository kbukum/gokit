package postgres_test

import (
	"testing"

	"github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/postgres"
)

func TestRegisterAddsDialectToRegistry(t *testing.T) {
	t.Parallel()

	reg := database.NewDialectRegistry()
	if err := postgres.Register(reg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	d, ok := reg.Get(postgres.Name)
	if !ok {
		t.Fatalf("dialect %q not found after Register", postgres.Name)
	}
	if d.Open("host=localhost user=app dbname=app sslmode=disable") == nil {
		t.Fatal("registered dialect returned nil dialector")
	}
}

func TestRegisterRejectsDuplicate(t *testing.T) {
	t.Parallel()

	reg := database.NewDialectRegistry()
	if err := postgres.Register(reg); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := postgres.Register(reg); err == nil {
		t.Fatal("expected duplicate Register to fail")
	}
}

func TestOpenReturnsDialector(t *testing.T) {
	t.Parallel()

	if postgres.Open("host=localhost user=app dbname=app sslmode=disable") == nil {
		t.Fatal("Open returned nil dialector")
	}
}

func TestMigrateDriverIsProvided(t *testing.T) {
	t.Parallel()

	if postgres.MigrateDriver() == nil {
		t.Fatal("MigrateDriver returned nil")
	}
}
