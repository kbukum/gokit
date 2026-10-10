package database

import (
	"context"
	"testing"
	"testing/fstest"
	"time"

	"github.com/kbukum/gokit/auth"
	dbkit "github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/migration"
	"github.com/kbukum/gokit/database/sqlite"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/util"
)

func TestMigrationsComposeWithApplicationSchema(t *testing.T) {
	t.Parallel()
	for _, sessionFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "session_first", false: "application_first"}[sessionFirst], func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			db, err := dbkit.NewWithContext(ctx, sqlite.Dialect(), dbkit.Config{DSN: "file:" + t.Name() + "?mode=memory&cache=shared", LogLevel: "silent"}, logging.NewDefault("session-test"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := db.Close(); err != nil {
					t.Error(err)
				}
			})
			assertMigrationComposition(t, db, sqlite.MigrateDriver(), "", sessionFirst)
		})
	}
}

func TestMigrationsRejectInvalidVersionTable(t *testing.T) {
	t.Parallel()
	cfg := Migrations(nil, sqlite.MigrateDriver())
	if cfg.Table.Name != VersionTable || !migration.IsVersionTable(cfg.Table.Name) || cfg.Table.Name == migration.DefaultVersionTable {
		t.Fatalf("session migrations must use their own version table, got %q", cfg.Table.Name)
	}
}

// assertMigrationComposition proves the session schema and an application schema, each starting at version 1, both install and report ready on one database in either order.
func assertMigrationComposition(t *testing.T, db *dbkit.DB, driver migration.DriverFunc, schema string, sessionFirst bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	app := migration.Config{DB: db.GormDB, Driver: driver, Table: migration.Table{Schema: schema}, Path: "migrations", FS: fstest.MapFS{
		"migrations/1_items.up.sql":   {Data: []byte("CREATE TABLE app_items (id BIGINT PRIMARY KEY);")},
		"migrations/1_items.down.sql": {Data: []byte("DROP TABLE app_items;")},
	}}
	sessions := Migrations(db, driver)
	sessions.Table.Schema = schema
	order := []migration.Config{app, sessions}
	if sessionFirst {
		order = []migration.Config{sessions, app}
	}
	for _, cfg := range order {
		if err := cfg.Up(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := app.Ready(ctx, 1); err != nil {
		t.Fatalf("application schema: %v", err)
	}
	if err := sessions.Ready(ctx, SchemaVersion); err != nil {
		t.Fatalf("session schema: %v", err)
	}
	if err := db.GormDB.WithContext(ctx).Exec("INSERT INTO app_items (id) VALUES (1)").Error; err != nil {
		t.Fatalf("application table missing: %v", err)
	}
	clock := util.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	issued, err := instance(t, db, clock).Create(ctx, auth.Principal{Subject: "u", Kind: auth.User, Restrictions: auth.Restrictions{Mode: auth.Unrestricted}})
	if err != nil || issued.Token == "" {
		t.Fatalf("session tables missing: %v", err)
	}
}
