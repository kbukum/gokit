package sqlite_test

import (
	"errors"
	"testing"
	"testing/fstest"

	"github.com/golang-migrate/migrate/v4"

	"github.com/kbukum/gokit/database/migration"
	"github.com/kbukum/gokit/database/sqlite"
)

func TestMigrationInspectionNeedsNoSource(t *testing.T) {
	t.Parallel()
	db := newMigrationDB(t)
	cfg := migration.Config{DB: db, Driver: sqlite.MigrateDriver()}
	if _, _, err := cfg.Version(t.Context()); !errors.Is(err, migrate.ErrNilVersion) {
		t.Fatalf("source-free empty inspection: %v", err)
	}

	cfg.FS, cfg.Path = migrationsFS, "testdata/migrations"
	if err := cfg.Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	cfg.FS, cfg.Path = nil, ""
	if err := cfg.Ready(t.Context(), 2); err != nil {
		t.Fatalf("source-free readiness: %v", err)
	}
}

func TestMigrationApplyStopsAtTarget(t *testing.T) {
	t.Parallel()
	db := newMigrationDB(t)
	cfg := migration.Config{DB: db, Driver: sqlite.MigrateDriver(), FS: migrationsFS, Path: "testdata/migrations"}
	if err := cfg.Apply(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Ready(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Apply(t.Context(), 2); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Ready(t.Context(), 2); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Apply(t.Context(), 1); err == nil {
		t.Fatal("future database version accepted")
	}
}

func TestMigrationApplyRejectsInvalidPlanBeforeMutation(t *testing.T) {
	t.Parallel()
	for _, target := range []uint{0, 2, 4} {
		t.Run(string(rune('0'+target)), func(t *testing.T) {
			t.Parallel()
			db := newMigrationDB(t)
			cfg := migration.Config{DB: db, Driver: sqlite.MigrateDriver(), Path: ".", FS: fstest.MapFS{
				"1_first.up.sql": {Data: []byte("CREATE TABLE first(id INTEGER)")},
				"3_later.up.sql": {Data: []byte("CREATE TABLE later(id INTEGER)")},
			}}
			if err := cfg.Apply(t.Context(), target); err == nil {
				t.Fatal("absent or invalid target accepted")
			}
			var count int
			if err := db.Raw("SELECT count(*) FROM sqlite_master WHERE type = 'table'").Scan(&count).Error; err != nil || count != 0 {
				t.Fatalf("rejected plan mutated schema: %d %v", count, err)
			}
		})
	}
}

func TestMigrationLockDoesNotCreateMetadata(t *testing.T) {
	t.Parallel()
	db := newMigrationDB(t)
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	d, err := sqlite.MigrateDriver()(t.Context(), pool, migration.Table{Name: migration.DefaultVersionTable})
	if err != nil {
		t.Fatal(err)
	}
	lockErr := d.Lock()
	if err := errors.Join(lockErr, d.Close()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.Raw("SELECT count(*) FROM sqlite_master WHERE type = 'table'").Scan(&count).Error; err != nil || count != 0 {
		t.Fatalf("lock performed DDL: tables=%d error=%v", count, err)
	}
}
