package sqlite

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"

	"github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/migration"
	apperrors "github.com/kbukum/gokit/errors"
)

func TestPrepareAndOpenRejectInvalidInput(t *testing.T) {
	t.Parallel()
	var nilCtx context.Context
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	memory := database.ConnectionInput{DSN: ":memory:"}
	for name, input := range map[string]database.ConnectionInput{
		"structured parameters": {Params: database.ConnParams{Host: "localhost"}},
		"no input":              {},
		"malformed options":     {DSN: "file.db?mode=%zz"},
	} {
		if value, err := Prepare(t.Context(), input); value != nil || apperrors.Normalize(err).Code != apperrors.ErrCodeInvalidInput {
			t.Errorf("%s = %v", name, err)
		}
	}
	if _, err := Prepare(nilCtx, memory); apperrors.Normalize(err).Code != apperrors.ErrCodeInvalidInput {
		t.Fatalf("nil context = %v", err)
	}
	if _, err := Prepare(canceled, memory); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context = %v", err)
	}
	prepared, err := Prepare(t.Context(), memory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prepared.Open(nilCtx); apperrors.Normalize(err).Code != apperrors.ErrCodeInvalidInput {
		t.Fatalf("Open with nil context = %v", err)
	}
	if value, err := prepared.Open(canceled); value != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("Open with canceled context = %v", err)
	}
}

func TestDialectorRejectsInvalidConnectionBeforeOpening(t *testing.T) {
	t.Parallel()
	for name, dsn := range map[string]string{"empty": "", "malformed options": "file.db?mode=%zz"} {
		if err := (&dialector{dsn: dsn}).Initialize(&gorm.DB{Config: &gorm.Config{}}); apperrors.Normalize(err).Code != apperrors.ErrCodeInvalidInput {
			t.Errorf("%s = %v", name, err)
		}
	}
}

func TestMigrationBackendAcceptsOnlyTheMainSchema(t *testing.T) {
	t.Parallel()
	backend := MigrationBackend()
	if err := backend.ValidateTable(migration.Table{Schema: "main", Name: "schema_migrations"}); err != nil {
		t.Fatal(err)
	}
	if err := backend.ValidateTable(migration.Table{Schema: "aux", Name: "schema_migrations"}); apperrors.Normalize(err).Code != apperrors.ErrCodeInvalidInput {
		t.Fatalf("attached schema = %v", err)
	}
}
