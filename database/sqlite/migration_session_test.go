package sqlite_test

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/golang-migrate/migrate/v4"

	"github.com/kbukum/gokit/database/migration"
	"github.com/kbukum/gokit/database/sqlite"
	apperrors "github.com/kbukum/gokit/errors"
)

func tableCount(t *testing.T, cfg migration.Config) int {
	t.Helper()
	var count int
	if err := cfg.DB.Raw("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name NOT LIKE '%schema_migrations'").Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func TestMigrationUpRejectsStoredVersionUnknownToSource(t *testing.T) {
	t.Parallel()
	db := newMigrationDB(t)
	cfg := migration.Config{DB: db, Driver: sqlite.MigrateDriver(), FS: migrationsFS, Path: "testdata/migrations"}
	if err := cfg.Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	cfg.Path, cfg.FS = ".", fstest.MapFS{
		"1_first.up.sql": {Data: []byte("CREATE TABLE first(id INTEGER)")},
		"3_later.up.sql": {Data: []byte("CREATE TABLE later(id INTEGER)")},
	}
	before := tableCount(t, cfg)
	for name, run := range map[string]func() error{
		"up":    func() error { return cfg.Up(t.Context()) },
		"apply": func() error { return cfg.Apply(t.Context(), 3) },
		"down":  func() error { return cfg.Down(t.Context()) },
	} {
		if err := run(); apperrors.Normalize(err).Code != apperrors.ErrCodeDatabaseError {
			t.Fatalf("%s accepted a version the source does not contain: %v", name, err)
		}
	}
	if tableCount(t, cfg) != before {
		t.Fatal("rejected plan ran source SQL")
	}
}

func TestMigrationRejectsDirtyAndUnreadablePendingFilesBeforeSQL(t *testing.T) {
	t.Parallel()
	db := newMigrationDB(t)
	cfg := migration.Config{DB: db, Driver: sqlite.MigrateDriver(), Path: ".", FS: fstest.MapFS{
		"1_first.up.sql":      {Data: []byte("CREATE TABLE first(id INTEGER)")},
		"2_large.up.sql":      {Data: []byte(strings.Repeat(" ", 1<<20+1))},
		"3_downonly.down.sql": {Data: []byte("SELECT 1")},
	}}
	if err := cfg.Apply(t.Context(), 2); err == nil || tableCount(t, cfg) != 0 {
		t.Fatalf("oversized pending file not rejected before SQL: %v", err)
	}
	if err := cfg.Apply(t.Context(), 3); err == nil || tableCount(t, cfg) != 0 {
		t.Fatalf("missing up file not rejected before SQL: %v", err)
	}
	if err := cfg.Apply(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE schema_migrations SET dirty = 1").Error; err != nil {
		t.Fatal(err)
	}
	var dirty migrate.ErrDirty
	if err := cfg.Up(t.Context()); !errors.As(err, &dirty) {
		t.Fatalf("dirty schema: %v", err)
	}
}

func TestMigrationMetadataMustBeOneRowOrdinaryTable(t *testing.T) {
	t.Parallel()
	db := newMigrationDB(t)
	cfg := migration.Config{DB: db, Driver: sqlite.MigrateDriver(), FS: migrationsFS, Path: "testdata/migrations"}
	if err := db.Exec("CREATE VIEW schema_migrations AS SELECT 2 AS version, 0 AS dirty").Error; err != nil {
		t.Fatal(err)
	}
	if err := cfg.Ready(t.Context(), 2); err == nil || !strings.Contains(err.Error(), "not an ordinary table") {
		t.Fatalf("view treated as metadata or as absent: %v", err)
	}
	if err := db.Exec("DROP VIEW schema_migrations").Error; err != nil {
		t.Fatal(err)
	}
	if err := cfg.Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO schema_migrations(version, dirty) VALUES (1, 0)").Error; err != nil {
		t.Fatal(err)
	}
	if err := cfg.Ready(t.Context(), 2); err == nil || !strings.Contains(err.Error(), "more than one version") {
		t.Fatalf("multi-row metadata accepted: %v", err)
	}
}

func TestSessionAppliesSetsOnOnePinnedConnection(t *testing.T) {
	t.Parallel()
	db := newMigrationDB(t)
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if pool.Stats().MaxOpenConnections != 1 {
		t.Fatal("the proof needs a one-connection pool")
	}
	first := migration.Set{FS: migrationsFS, Path: "testdata/migrations", Table: migration.Table{Name: "app_schema_migrations"}, ExpectedVersion: 2}
	second := migration.Set{Path: ".", Table: migration.Table{Name: "audit_schema_migrations"}, ExpectedVersion: 1, FS: fstest.MapFS{
		"1_audit.up.sql": {Data: []byte("CREATE TABLE audit(id INTEGER)")},
	}}
	invalid := second
	invalid.ExpectedVersion = 4

	session, err := migration.Begin(t.Context(), pool, sqlite.MigrationBackend())
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Apply(t.Context(), first, invalid); err == nil {
		t.Fatal("invalid second set accepted")
	}
	var count int
	if err := session.Conn().QueryRowContext(t.Context(), "SELECT count(*) FROM sqlite_master WHERE type = 'table'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("plan failure mutated the first set: %d %v", count, err)
	}
	if err := session.Apply(t.Context(), first, second); err != nil {
		t.Fatal(err)
	}
	if err := errors.Join(session.Close(), session.Close()); err != nil {
		t.Fatal(err)
	}
	if err := session.Apply(t.Context(), first); err == nil {
		t.Fatal("closed session applied a set")
	}
	for _, set := range []migration.Set{first, second} {
		cfg := migration.Config{DB: db, Driver: sqlite.MigrateDriver(), Table: set.Table}
		if err := cfg.Ready(t.Context(), set.ExpectedVersion); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMigrationReadyReportsUnmigratedSchemaAsNotReady(t *testing.T) {
	t.Parallel()
	cfg := migration.Config{DB: newMigrationDB(t), Driver: sqlite.MigrateDriver()}
	err := cfg.Ready(t.Context(), 1)
	if !errors.Is(err, migrate.ErrNilVersion) || apperrors.Normalize(err).Code != apperrors.ErrCodeDatabaseError {
		t.Fatalf("Ready on an unmigrated schema = %v", err)
	}
}
