package postgres_test

import (
	"testing"

	"gorm.io/gorm"

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
	opener, err := d.Prepare(t.Context(), database.ConnectionInput{Params: testParams()})
	if err != nil || opener == nil {
		t.Fatal("registered dialect returned nil opener")
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

	if preparedDialector(t, testParams()) == nil {
		t.Fatal("prepared opener returned nil dialector")
	}
}

func testParams() database.ConnParams {
	return database.ConnParams{Host: "localhost", User: "app", Database: "app", Options: map[string]string{"sslmode": "disable", "search_path": "app"}}
}

func preparedDialector(t *testing.T, params database.ConnParams) gorm.Dialector {
	t.Helper()
	opener, err := postgres.Prepare(t.Context(), database.ConnectionInput{Params: params})
	if err != nil {
		t.Fatal(err)
	}
	dialector, err := opener.Open(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	owner, ok := dialector.(interface{ Close() error })
	if !ok {
		t.Fatal("PostgreSQL dialector has no pool owner")
	}
	t.Cleanup(func() {
		if err := owner.Close(); err != nil {
			t.Error(err)
		}
	})
	return dialector
}

func TestMigrateDriverIsProvided(t *testing.T) {
	t.Parallel()

	if postgres.MigrateDriver() == nil {
		t.Fatal("MigrateDriver returned nil")
	}
}
