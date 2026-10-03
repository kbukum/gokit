//go:build integration

package testutil_test

import (
	"context"
	"testing"
	"time"

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
				return database.NewWithContext(ctx, postgres.Open(fixture.DSN), database.Config{MaxRetries: 1}, logging.NewDefault("fixture"))
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
