package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/mattn/go-sqlite3"

	"github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/migration"
	"github.com/kbukum/gokit/database/sqlite"
	"github.com/kbukum/gokit/logging"
)

func TestRealSQLiteMigrations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := newMigrationDB(t)
	cfg := migration.Config{DB: db, FS: migrationsFS, Path: "testdata/migrations", Driver: sqlite.MigrateDriver()}
	for range 3 {
		if err := cfg.Up(ctx); err != nil {
			t.Fatal(err)
		}

		if err := cfg.Ready(ctx, 2); err != nil {
			t.Fatal(err)
		}
	}
	if err := cfg.Ready(ctx, 3); err == nil {
		t.Fatal("wrong schema version reported ready")
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if pool.Stats().InUse != 0 {
		t.Fatal("migration retained its connection")
	}
	if err := cfg.Steps(ctx, -1); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Ready(ctx, 1); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := cfg.Up(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	cfg.FS = fstest.MapFS{
		"1_create.up.sql": {Data: []byte("CREATE TABLE broken(id INTEGER); SELECT no_such_function();")},
	}
	cfg.Path = "."
	cfg.DB = newMigrationDB(t)
	if err := cfg.Up(ctx); err == nil {
		t.Fatal("expected SQL failure")
	}
	if err := cfg.Ready(ctx, 1); err == nil {
		t.Fatal("dirty migration reported ready")
	}
	version, dirty, err := cfg.Version(ctx)
	if err != nil || version != 1 || !dirty {
		t.Fatalf("dirty state: %d %v %v", version, dirty, err)
	}
}

func TestSQLiteMigrationCancellationAndCleanup(t *testing.T) {
	t.Parallel()
	ctx, stop := context.WithTimeout(t.Context(), 10*time.Second)
	defer stop()
	db, err := database.NewWithContext(ctx, sqlite.Dialect(), database.Config{DSN: filepath.Join(t.TempDir(), "migration.db")}, logging.NewDefault("test"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	pool, err := db.GormDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	budget, cancel := context.WithCancel(ctx)
	defer cancel()
	conn, err := pool.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = errors.Join(conn.Raw(func(raw any) error {
		sqliteConn, ok := raw.(*sqlite3.SQLiteConn)
		if !ok {
			return errors.New("expected a SQLite connection")
		}
		return sqliteConn.RegisterFunc("cancel_migration", func() int {
			cancel()
			return 0
		}, false)
	}), conn.Close())
	if err != nil {
		t.Fatal(err)
	}
	cfg := migration.Config{DB: db.GormDB, Path: ".", Driver: sqlite.MigrateDriver(), FS: fstest.MapFS{
		"1_long.up.sql": {Data: []byte("WITH RECURSIVE counter(x) AS (VALUES(cancel_migration()) UNION ALL SELECT x+1 FROM counter WHERE x < 100000000) SELECT sum(x) FROM counter;")},
	}}
	err = cfg.Up(budget)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("running migration did not honor cancellation: %v", err)
	}
	version, dirty, err := cfg.Version(ctx)
	if err != nil || version != 1 || !dirty {
		t.Fatalf("canceled migration state: %d %t %v", version, dirty, err)
	}
	if err := cfg.Up(ctx); err == nil {
		t.Fatal("dirty migration was reapplied")
	}
	if pool.Stats().InUse != 0 {
		t.Fatal("canceled migration retained a connection")
	}
}

func TestSQLiteMigrationResetAndDriverLifecycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := newMigrationDB(t)
	cfg := migration.Config{DB: db, FS: migrationsFS, Path: "testdata/migrations", Driver: sqlite.MigrateDriver()}
	if err := cfg.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Ready(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Down(ctx); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Ready(ctx, 0); err == nil {
		t.Fatal("empty schema was ready")
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	driver, err := sqlite.MigrateDriver()(ctx, pool, migration.Table{Name: migration.DefaultVersionTable})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := driver.Open("unused"); err == nil {
		t.Fatal("context-free open accepted")
	}
	if err := driver.Lock(); err != nil {
		t.Fatal(err)
	}
	if err := driver.Lock(); err == nil {
		t.Fatal("second lock accepted")
	}
	if err := driver.Close(); err != nil {
		t.Fatal(err)
	}
	if err := driver.Close(); err != nil {
		t.Fatal(err)
	}
	if err := driver.Unlock(); err == nil {
		t.Fatal("unlock without lock accepted")
	}
	if err := pool.Close(); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Up(ctx); err == nil {
		t.Fatal("closed pool accepted")
	}
}

func TestSQLiteResetWithForeignKeys(t *testing.T) {
	t.Parallel()
	db := newMigrationDB(t)
	cfg := migration.Config{DB: db, Path: ".", Driver: sqlite.MigrateDriver(), FS: fstest.MapFS{
		"1_tables.up.sql": {Data: []byte(`
			CREATE TABLE a_parent(id INTEGER PRIMARY KEY);
			CREATE TABLE z_child(id INTEGER PRIMARY KEY, parent_id INTEGER REFERENCES a_parent(id) ON DELETE RESTRICT);
			INSERT INTO a_parent VALUES (1);
			INSERT INTO z_child VALUES (1, 1);`)},
	}}
	if err := cfg.Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Reset(t.Context()); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Table("z_child").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("reset did not reapply schema: %d, %v", count, err)
	}
	if err := db.Exec("INSERT INTO z_child VALUES (2, 99)").Error; err == nil {
		t.Fatal("reset disabled foreign key enforcement")
	}
}

func TestSQLiteVersionIsReadOnly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := newMigrationDB(t)
	cfg := migration.Config{DB: db, FS: migrationsFS, Path: "testdata/migrations", Driver: sqlite.MigrateDriver(), Table: migration.Table{Name: "app_schema_migrations"}}
	if _, _, err := cfg.Version(ctx); !errors.Is(err, migrate.ErrNilVersion) {
		t.Fatalf("version before migrating: %v", err)
	}
	if err := cfg.Ready(ctx, 2); err == nil {
		t.Fatal("unmigrated schema reported ready")
	}
	var tables int64
	if err := db.WithContext(ctx).Raw("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", cfg.Table.Name).Scan(&tables).Error; err != nil || tables != 0 {
		t.Fatalf("version inspection created the version table: %d, %v", tables, err)
	}
}
