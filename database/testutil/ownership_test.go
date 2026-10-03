package testutil

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/kbukum/gokit/codec"
	"github.com/kbukum/gokit/testutil"
)

func TestFailedInitializationReleasesPartialDatabase(t *testing.T) {
	t.Parallel()
	failure := errors.New("initialization failed")
	var opened *gorm.DB
	c := NewComponent().WithInitializer(func(_ context.Context, db *gorm.DB) error {
		opened = db
		return failure
	})
	if err := c.Start(t.Context()); !errors.Is(err, failure) {
		t.Fatalf("startup failure: %v", err)
	}
	if c.DB() != nil {
		t.Fatal("failed start published a database")
	}
	pool, err := opened.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.PingContext(t.Context()); err == nil {
		t.Fatal("partial database remained open")
	}
	if err := c.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestResetPreservesMigrationMetadataAndForeignKeys(t *testing.T) {
	t.Parallel()
	c := NewComponent()
	testutil.T(t).Setup(c)
	db := c.DB().Session(&gorm.Session{Logger: logger.Discard})
	for _, sql := range []string{
		"PRAGMA foreign_keys=ON",
		"CREATE TABLE schema_migrations(version INTEGER, dirty BOOLEAN)",
		"INSERT INTO schema_migrations VALUES(1, false)",
		`CREATE TABLE "parent table"(id INTEGER PRIMARY KEY)`,
		`CREATE TABLE children(id INTEGER PRIMARY KEY, parent INTEGER REFERENCES "parent table"(id))`,
		`INSERT INTO "parent table" VALUES(1)`,
		"INSERT INTO children VALUES(1, 1)",
	} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Reset(t.Context()); err != nil {
		t.Fatal(err)
	}
	var version int
	if err := db.Raw("SELECT version FROM schema_migrations").Scan(&version).Error; err != nil || version != 1 {
		t.Fatalf("metadata erased: %d %v", version, err)
	}
	for _, table := range []string{"parent table", "children"} {
		count, err := CountRows(t.Context(), db, table)
		if err != nil || count != 0 {
			t.Fatalf("table %s: %d %v", table, count, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := c.Reset(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("reset ignored cancellation: %v", err)
	}
	if err := db.Exec("INSERT INTO children VALUES(2, 999)").Error; err == nil {
		t.Fatal("reset disabled foreign key enforcement")
	}
}

func TestFixtureLimitsAndAtomicFailure(t *testing.T) {
	t.Parallel()
	c := NewComponent()
	testutil.T(t).Setup(c)
	db := c.DB().Session(&gorm.Session{Logger: logger.Discard})
	if err := db.Exec("CREATE TABLE fixture(id INTEGER PRIMARY KEY, value TEXT)").Error; err != nil {
		t.Fatal(err)
	}
	rows := make([]map[string]any, 1000, 1001)
	for i := range rows {
		rows[i] = map[string]any{"id": i}
	}
	if err := LoadFixture(t.Context(), db, "fixture", rows); err != nil {
		t.Fatal(err)
	}
	if err := LoadFixture(t.Context(), db, "fixture", append(rows, map[string]any{"id": 1000})); err == nil {
		t.Fatal("accepted 1001 rows")
	}
	if err := TruncateTable(t.Context(), db, "fixture"); err != nil {
		t.Fatal(err)
	}
	if err := LoadFixture(t.Context(), db, "fixture", []map[string]any{{"id": 1}, {"id": 1}}); err == nil {
		t.Fatal("expected duplicate key failure")
	}
	count, err := CountRows(t.Context(), db, "fixture")
	if err != nil || count != 0 {
		t.Fatalf("failed fixture partially committed: %d %v", count, err)
	}
	if err := LoadFixture(t.Context(), db, "fixture", []map[string]any{{"value": strings.Repeat("a", 1<<20)}}); err == nil {
		t.Fatal("accepted encoded fixture over 1 MiB")
	}
}

func TestSnapshotExactByteAndRowLimits(t *testing.T) {
	t.Parallel()
	c := NewComponent()
	testutil.T(t).Setup(c)
	db := c.DB()
	if err := db.Exec("CREATE TABLE fixture(value TEXT)").Error; err != nil {
		t.Fatal(err)
	}
	encoded, err := codec.Encode(codec.CompactJSON(), map[string][]map[string]any{"fixture": {{"value": ""}}})
	if err != nil {
		t.Fatal(err)
	}
	value := strings.Repeat("a", MaxFixtureBytes-len(encoded))
	if err := db.Exec("INSERT INTO fixture(value) VALUES(?)", value).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := c.Snapshot(t.Context()); err != nil {
		t.Fatalf("rejected exact encoded byte limit: %v", err)
	}
	if err := db.Exec("UPDATE fixture SET value=?", value+"a").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := c.Snapshot(t.Context()); err == nil {
		t.Fatal("accepted one byte over snapshot limit")
	}
	if err := c.Reset(t.Context()); err != nil {
		t.Fatal(err)
	}
	rows := make([]map[string]any, MaxFixtureRows)
	for i := range rows {
		rows[i] = map[string]any{"value": "a"}
	}
	if err := LoadFixture(t.Context(), db, "fixture", rows); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Snapshot(t.Context()); err != nil {
		t.Fatalf("rejected exactly 1000 rows: %v", err)
	}
	if err := db.Exec("INSERT INTO fixture VALUES('extra')").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := c.Snapshot(t.Context()); err == nil {
		t.Fatal("accepted 1001 snapshot rows")
	}
}

func TestResetTableLimitsAndRestoreRollback(t *testing.T) {
	t.Parallel()
	c := NewComponent()
	testutil.T(t).Setup(c)
	db := c.DB()
	for i := range MaxFixtureTables {
		name := "table_" + strings.Repeat("x", i+1)
		if err := db.Exec("CREATE TABLE " + quoteTable(db, name) + "(id INTEGER)").Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Reset(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE extra(id INTEGER)").Error; err != nil {
		t.Fatal(err)
	}
	if err := c.Reset(t.Context()); err == nil {
		t.Fatal("accepted 33 application tables")
	}
	other := NewComponent()
	testutil.T(t).Setup(other)
	db = other.DB()
	if err := db.Exec("CREATE TABLE fixture(id INTEGER PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO fixture VALUES(1)").Error; err != nil {
		t.Fatal(err)
	}
	snapshot, err := other.Snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		"INSERT INTO fixture VALUES(2)",
		"CREATE TRIGGER reject_restore BEFORE INSERT ON fixture BEGIN SELECT RAISE(ABORT, 'rejected fixture'); END",
	} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := other.Restore(t.Context(), snapshot); err == nil {
		t.Fatal("expected restore failure")
	}
	if count, err := CountRows(t.Context(), db, "fixture"); err != nil || count != 2 {
		t.Fatalf("failed restore erased current state: %d %v", count, err)
	}
}
