//go:build integration

package database

import (
	"context"
	"testing"
	"time"

	dbkit "github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/postgres"
	pgtest "github.com/kbukum/gokit/database/postgres/testutil"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/util"
)

func TestPostgresTransactionalFamily(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	fixture, err := pgtest.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := fixture.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	db, err := dbkit.NewWithContext(ctx, postgres.Open(fixture.DSN), dbkit.Config{LogLevel: "silent"}, logging.NewDefault("session-test"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	cfg := Migrations(db, postgres.MigrateDriver())
	if err := cfg.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Ready(ctx, 1); err != nil {
		t.Fatal(err)
	}
	clock := util.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	s, err := NewStore(db, clock)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Create(ctx, row("old", "family", clock)); err != nil {
		t.Fatal(err)
	}
	next := row("new", "family", clock)
	next.Generation = 2
	if err := s.Rotate(ctx, "old", next); err != nil {
		t.Fatal(err)
	}
	relogin := row("relogin", "family", clock)
	relogin.Generation = 3
	relogin.Principal.Subject = "reauthenticated"
	if err := s.Relogin(ctx, "new", relogin); err != nil {
		t.Fatal(err)
	}
	if err := s.Relogin(ctx, "new", relogin); err == nil {
		t.Fatal("stale login generation recovered")
	}
	clock.Advance(time.Hour)
	recovered := row("recovered", "family", clock)
	recovered.Generation = 4
	if err := s.Relogin(ctx, "relogin", recovered); err != nil {
		t.Fatal("strong-auth expired recovery failed", err)
	}
	if _, err := s.Revoke(ctx, "old"); err != nil {
		t.Fatal(err)
	}
	record, err := s.Lookup(ctx, "recovered")
	if err != nil || !record.Revoked {
		t.Fatal("family revocation failed", err)
	}
	clock.Advance(time.Hour + 10*time.Minute)
	if n, err := s.Cleanup(ctx, clock.Now(), 256); err != nil || n != 5 {
		t.Fatal(n, err)
	}
}

func TestPostgresCrossInstanceRevocation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	fixture, err := pgtest.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := fixture.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	open := func() *dbkit.DB {
		db, err := dbkit.NewWithContext(ctx, postgres.Open(fixture.DSN), dbkit.Config{LogLevel: "silent"}, logging.NewDefault("session-test"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := db.Close(); err != nil {
				t.Error(err)
			}
		})
		return db
	}
	first, second := open(), open()
	cfg := Migrations(first, postgres.MigrateDriver())
	if err := cfg.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Ready(ctx, 1); err != nil {
		t.Fatal(err)
	}
	assertCrossInstanceRevocation(t, first, second, util.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)))
}

func TestPostgresMigrationsComposeWithApplicationSchema(t *testing.T) {
	for _, sessionFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "session_first", false: "application_first"}[sessionFirst], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			fixture, err := pgtest.Start(ctx)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := fixture.Close(context.Background()); err != nil {
					t.Error(err)
				}
			})
			db, err := dbkit.NewWithContext(ctx, postgres.Open(fixture.DSN), dbkit.Config{LogLevel: "silent"}, logging.NewDefault("session-test"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := db.Close(); err != nil {
					t.Error(err)
				}
			})
			assertMigrationComposition(t, db, postgres.MigrateDriver(), sessionFirst)
		})
	}
}
