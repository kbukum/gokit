package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"gorm.io/gorm"

	"github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/migration"
	"github.com/kbukum/gokit/database/sqlite"
	dbtest "github.com/kbukum/gokit/database/testutil"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/testutil"
)

func TestOwnedFileDatabaseEnvironment(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "owned.db")
	var owned *database.DB
	var schema migration.Config
	c := dbtest.NewComponent().WithDatabase(func(ctx context.Context) (*database.DB, error) {
		db, err := database.NewWithContext(ctx, sqlite.Dialect(), database.Config{DSN: path, MaxRetries: 1}, logging.NewDefault("fixture"))
		owned = db
		return db, err
	}).WithInitializer(func(ctx context.Context, db *gorm.DB) error {
		schema = migration.Config{DB: db, Driver: sqlite.MigrateDriver(), Path: ".", FS: fstest.MapFS{
			"1_fixture.up.sql": {Data: []byte("CREATE TABLE fixture(id INTEGER PRIMARY KEY, value TEXT NOT NULL);")},
		}}
		return errors.Join(schema.Up(ctx), schema.Ready(ctx, 1))
	})
	ctx, cancel := context.WithCancel(t.Context())
	manager := testutil.NewManager(ctx)
	if err := manager.Add(c); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.Cleanup(); err != nil {
			t.Error(err)
		}
	})
	if err := manager.StartAll(); err != nil {
		t.Fatal(err)
	}
	if err := dbtest.LoadFixture(t.Context(), c.DB(), "fixture", []map[string]any{{"id": 1, "value": "persisted"}}); err != nil {
		t.Fatal(err)
	}
	if err := c.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if count, err := dbtest.CountRows(t.Context(), c.DB(), "fixture"); err != nil || count != 1 {
		t.Fatalf("durable restart: %d %v", count, err)
	}
	if err := c.Reset(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := schema.Ready(t.Context(), 1); err != nil {
		t.Fatalf("reset erased production migration metadata: %v", err)
	}
	pool, err := owned.GormDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	start := time.Now()
	if err := manager.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 2*time.Second || pool.Stats().OpenConnections != 0 || c.DB() != nil {
		t.Fatal("canceled environment retained owned database resources")
	}
	if err := manager.Cleanup(); err != nil {
		t.Fatal(err)
	}
}
