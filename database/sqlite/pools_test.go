package sqlite_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/sqlite"
	"github.com/kbukum/gokit/logging"
)

func TestSQLiteOwnedPools(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn := filepath.Join(t.TempDir(), "pool.db")
	db, err := database.NewWithContext(ctx, sqlite.Open(dsn), database.Config{}, logging.NewDefault("test"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := db.WithContext(ctx).Exec("CREATE TABLE items(id INTEGER PRIMARY KEY, value INTEGER)").Error; err != nil {
		t.Fatal(err)
	}
	writer, err := db.GormDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	reader, err := db.ReadOnly(ctx).DB()
	if err != nil {
		t.Fatal(err)
	}
	if writer == reader || writer.Stats().MaxOpenConnections != 1 || reader.Stats().MaxOpenConnections != 4 {
		t.Fatalf("pool limits: write=%+v read=%+v", writer.Stats(), reader.Stats())
	}
	if err := db.ReadOnly(ctx).Exec("INSERT INTO items(value) VALUES (1)").Error; err == nil {
		t.Fatal("read pool permitted a write")
	}
	var wg sync.WaitGroup
	for worker := range 12 {
		wg.Go(func() {
			for range 100 {
				var err error
				if worker < 4 {
					err = db.WithContext(ctx).Exec("INSERT INTO items(value) VALUES (1)").Error
				} else {
					var count int64
					err = db.ReadOnly(ctx).Table("items").Count(&count).Error
				}
				if err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	wg.Wait()
	var count int64
	if err := db.ReadOnly(ctx).Table("items").Count(&count).Error; err != nil || count != 400 {
		t.Fatalf("writes: %d %v", count, err)
	}
	connections := make([]interface{ Close() error }, 0, 4)
	for range 4 {
		conn, err := reader.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		connections = append(connections, conn)
		var mode string
		var busy, readOnly int
		if err := conn.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
			t.Fatal(err)
		}
		if err := conn.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busy); err != nil {
			t.Fatal(err)
		}
		if err := conn.QueryRowContext(ctx, "PRAGMA query_only").Scan(&readOnly); err != nil {
			t.Fatal(err)
		}
		if mode != "wal" || busy != 5000 || readOnly != 1 {
			t.Fatalf("per-connection settings: %s %d %d", mode, busy, readOnly)
		}
	}
	for _, conn := range connections {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if writer.Stats().OpenConnections != 0 || reader.Stats().OpenConnections != 0 {
		t.Fatal("database retained connections after close")
	}
}

func TestMemoryDatabaseRetainsItsConnection(t *testing.T) {
	t.Parallel()
	for _, dsn := range []string{":memory:", ":memory:?cache=shared", "file::memory:?cache=shared", "file:retained?mode=memory&cache=shared"} {
		t.Run(dsn, func(t *testing.T) {
			t.Parallel()
			db, err := database.NewWithContext(t.Context(), sqlite.Open(dsn), database.Config{
				ConnMaxLifetime: "1ns", ConnMaxIdleTime: "1ns",
			}, logging.NewDefault("test"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := db.Close(); err != nil {
					t.Error(err)
				}
			})
			if err := db.WithContext(t.Context()).Exec("CREATE TABLE retained(id INTEGER PRIMARY KEY)").Error; err != nil {
				t.Fatal(err)
			}
			if err := db.WithContext(t.Context()).Exec("INSERT INTO retained VALUES (1)").Error; err != nil {
				t.Fatalf("database lost its schema: %v", err)
			}
			writer, err := db.GormDB.DB()
			if err != nil {
				t.Fatal(err)
			}
			reader, err := db.ReadOnly(t.Context()).DB()
			if err != nil {
				t.Fatal(err)
			}
			if reader != writer || writer.Stats().MaxLifetimeClosed != 0 || writer.Stats().MaxIdleTimeClosed != 0 {
				t.Fatal("memory database replaced its only connection")
			}
		})
	}
}
