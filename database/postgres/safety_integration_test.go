//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/migration"
	"github.com/kbukum/gokit/database/postgres"
	"github.com/kbukum/gokit/logging"
)

func TestPostgresSafeDefaultsAndMigrations(t *testing.T) {
	dsn := newDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := database.NewWithContext(ctx, postgres.Open(dsn), database.Config{}, logging.NewDefault("test"))
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
	conn, err := pool.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var statement, idle string
	if err := conn.QueryRowContext(ctx, "SHOW statement_timeout").Scan(&statement); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRowContext(ctx, "SHOW idle_in_transaction_session_timeout").Scan(&idle); err != nil {
		t.Fatal(err)
	}
	if statement != "30s" || idle != "10s" {
		t.Fatalf("timeouts: %s %s", statement, idle)
	}
	if _, err := conn.ExecContext(ctx, "SET statement_timeout = '50ms'"); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, "SELECT pg_sleep(1)"); err == nil {
		t.Fatal("statement timeout not enforced")
	}
	if _, err := conn.ExecContext(ctx, "SET statement_timeout = '30s'"); err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	cfg := migration.Config{DB: db.GormDB, FS: migrationsFS, Path: "testdata/migrations", Driver: postgres.MigrateDriver()}
	second, err := database.NewWithContext(ctx, postgres.Open(dsn), database.Config{}, logging.NewDefault("test"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := second.Close(); err != nil {
			t.Error(err)
		}
	})
	var wg sync.WaitGroup
	for _, instance := range []*database.DB{db, second} {
		wg.Go(func() {
			instanceConfig := cfg
			instanceConfig.DB = instance.GormDB
			if err := instanceConfig.Up(ctx); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if err := cfg.Ready(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if pool.Stats().InUse != 0 {
		t.Fatal("migrations retained connections")
	}
	locker, err := pool.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := locker.ExecContext(ctx, "SELECT pg_advisory_lock(718049637891)"); err != nil {
		t.Fatal(err)
	}
	waitCtx, waitCancel := context.WithTimeout(ctx, 50*time.Millisecond)
	start := time.Now()
	err = cfg.Up(waitCtx)
	waitCancel()
	if err == nil || time.Since(start) > time.Second {
		t.Fatalf("lock wait did not cancel promptly: %v", err)
	}
	if _, err := locker.ExecContext(ctx, "SELECT pg_advisory_unlock(718049637891)"); err != nil {
		t.Fatal(err)
	}
	if err := locker.Close(); err != nil {
		t.Fatal(err)
	}
	canceled, stop := context.WithCancel(ctx)
	stop()
	if err := cfg.Up(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	if err := cfg.Ready(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Down(ctx); err != nil {
		t.Fatal(err)
	}
	cfg.FS = fstest.MapFS{"1_slow.up.sql": {Data: []byte("SELECT pg_sleep(10)")}}
	cfg.Path = "."
	sqlCtx, sqlCancel := context.WithTimeout(ctx, 50*time.Millisecond)
	start = time.Now()
	err = cfg.Up(sqlCtx)
	sqlCancel()
	if err == nil || time.Since(start) > time.Second {
		t.Fatalf("running SQL did not cancel: %v", err)
	}
	version, dirty, err := cfg.Version(ctx)
	if err != nil || version != 1 || !dirty {
		t.Fatalf("canceled SQL lost dirty state: %d %t %v", version, dirty, err)
	}
}

func TestPostgresResetWithForeignKeys(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	db, err := database.NewWithContext(ctx, postgres.Open(newDSN(t)), database.Config{}, logging.NewDefault("test"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	cfg := migration.Config{DB: db.GormDB, Path: ".", Driver: postgres.MigrateDriver(), FS: fstest.MapFS{
		"1_tables.up.sql": {Data: []byte(`
			CREATE TABLE a_parent(id INTEGER PRIMARY KEY);
			CREATE TABLE z_child(id INTEGER PRIMARY KEY, parent_id INTEGER REFERENCES a_parent(id));
			INSERT INTO a_parent VALUES (1);
			INSERT INTO z_child VALUES (1, 1);
			CREATE TABLE a_events(id INTEGER) PARTITION BY RANGE(id);
			CREATE TABLE z_events_partition PARTITION OF a_events FOR VALUES FROM (0) TO (100);
			INSERT INTO a_events VALUES (1);`)},
	}}
	if err := cfg.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.WithContext(ctx).Table("z_child").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("reset did not reapply schema: %d, %v", count, err)
	}
}
