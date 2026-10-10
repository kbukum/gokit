//go:build integration

package testutil_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/postgres"
	pgtest "github.com/kbukum/gokit/database/postgres/testutil"
	dbtest "github.com/kbukum/gokit/database/testutil"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/testutil"
)

func TestOwnedPostgresFixtureIsolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	for i := range 2 {
		t.Run(string(rune('a'+i)), func(t *testing.T) {
			fixture, err := pgtest.Start(ctx)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := fixture.Close(context.Background()); err != nil {
					t.Error(err)
				}
			})
			c := dbtest.NewComponent().WithDatabase(func(ctx context.Context) (*database.DB, error) {
				return database.NewWithContext(ctx, postgres.Dialect(), database.Config{Params: fixture.Params, MaxRetries: 1}, logging.NewDefault("fixture"))
			})
			testutil.T(t).Setup(c)
			db := c.DB()
			for _, sql := range []string{
				"CREATE TABLE schema_migrations(version BIGINT NOT NULL PRIMARY KEY, dirty BOOLEAN)",
				"INSERT INTO schema_migrations VALUES(1, false)",
				"CREATE TABLE parent(id INTEGER PRIMARY KEY)",
				"CREATE TABLE child(id INTEGER PRIMARY KEY, parent INTEGER REFERENCES parent(id))",
				"INSERT INTO parent VALUES(1)",
				"INSERT INTO child VALUES(1, 1)",
			} {
				if err := db.WithContext(ctx).Exec(sql).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := c.Reset(ctx); err != nil {
				t.Fatal(err)
			}
			if count, err := dbtest.CountRows(ctx, db, "schema_migrations"); err != nil || count != 1 {
				t.Fatalf("metadata reset: %d %v", count, err)
			}
			if count, err := dbtest.CountRows(ctx, db, "child"); err != nil || count != 0 {
				t.Fatalf("fixture reset: %d %v", count, err)
			}
			if err := c.Stop(ctx); err != nil {
				t.Fatal(err)
			}
			canceled, stop := context.WithCancel(ctx)
			stop()
			if err := fixture.Close(canceled); err != nil {
				t.Fatal(err)
			}
			if err := fixture.Close(canceled); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFixtureDatabasesAndRolePairsAreIsolatedAndCleaned(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
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
	first, err := fixture.NewDatabase(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := fixture.NewDatabase(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pair, err := first.NewRolePair(ctx, pgtest.RolePairConfig{Schema: "app"})
	if err != nil {
		t.Fatal(err)
	}
	connect := func(params database.ConnParams) (*database.DB, error) {
		return database.NewWithContext(ctx, postgres.Dialect(), database.Config{Params: params, MaxRetries: 1, LogLevel: "silent"}, logging.NewDefault("fixture"))
	}
	runtime, err := connect(pair.Runtime)
	if err != nil {
		t.Fatal(err)
	}
	var attributes bool
	if err := runtime.WithContext(ctx).Raw(`SELECT rolsuper OR rolcreatedb OR rolcreaterole OR rolbypassrls
		OR has_database_privilege(current_user, current_database(), 'CREATE, TEMPORARY')
		OR NOT starts_with((SELECT rolpassword FROM pg_authid WHERE rolname = current_user), 'SCRAM-SHA-256$')
		FROM pg_roles WHERE rolname = current_user`).Scan(&attributes).Error; err == nil {
		t.Fatal("runtime role can read pg_authid")
	}
	if err := runtime.WithContext(ctx).Raw(`SELECT rolsuper OR rolcreatedb OR rolcreaterole OR rolbypassrls
		OR has_database_privilege(current_user, current_database(), 'CREATE, TEMPORARY')
		FROM pg_roles WHERE rolname = current_user`).Scan(&attributes).Error; err != nil || attributes {
		t.Fatalf("runtime role is privileged: %v", err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	var verifier bool
	admin := first.AdminConfig()
	conn, err := pgx.ConnectConfig(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(ctx, "SELECT bool_and(starts_with(rolpassword, 'SCRAM-SHA-256$4096:')) FROM pg_authid WHERE rolname = ANY($1)",
		[]string{pair.Owner.User, pair.Runtime.User}).Scan(&verifier); err != nil || !verifier {
		t.Fatalf("role passwords are not client-computed SCRAM verifiers: %v", err)
	}
	if err := conn.Close(ctx); err != nil {
		t.Fatal(err)
	}
	foreign := pair.Runtime
	foreign.Database = second.Name()
	if db, err := connect(foreign); err == nil {
		t.Error("role pair connected to another fixture database")
		_ = db.Close()
	}
	if err := errors.Join(first.Close(ctx), first.Close(ctx)); err != nil {
		t.Fatal(err)
	}
	if _, err := first.NewRolePair(ctx, pgtest.RolePairConfig{Schema: "app"}); err == nil {
		t.Fatal("closed database created roles")
	}
	if _, err := connect(pair.Owner); err == nil {
		t.Fatal("owner role survived database cleanup")
	}
	admin = second.AdminConfig()
	conn, err = pgx.ConnectConfig(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	var roles int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM pg_roles WHERE rolname = ANY($1)", []string{pair.Owner.User, pair.Runtime.User}).Scan(&roles); err != nil || roles != 0 {
		t.Fatalf("roles survived cleanup: %d %v", roles, err)
	}
	if err := conn.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := fixture.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := second.Close(ctx); err != nil {
		t.Fatalf("fixture Close did not close its open database: %v", err)
	}
	if _, err := fixture.NewDatabase(ctx); err == nil {
		t.Fatal("closed fixture created a database")
	}
}
