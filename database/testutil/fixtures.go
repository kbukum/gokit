package testutil

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kbukum/gokit/codec"
	"github.com/kbukum/gokit/database/migration"
)

const (
	// MaxFixtureRows bounds rows per fixture and across an entire SQLite snapshot.
	MaxFixtureRows = 1000
	// MaxFixtureBytes bounds encoded fixture or snapshot data.
	MaxFixtureBytes = 1 << 20
	// MaxFixtureTables bounds application tables discovered by fixture operations.
	MaxFixtureTables = 32
)

func operationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 30*time.Second)
}

// validateFixture treats map values as opaque SQL column inputs.
func validateFixture(data []map[string]any) error {
	if len(data) > MaxFixtureRows {
		return fmt.Errorf("fixture exceeds %d rows", MaxFixtureRows)
	}
	encoded, err := codec.Encode(codec.CompactJSON(), data)
	if err != nil {
		return err
	}
	if len(encoded) > MaxFixtureBytes {
		return fmt.Errorf("fixture exceeds %d encoded bytes", MaxFixtureBytes)
	}
	return nil
}

// LoadFixture loads test data into a table.
// Data should be a slice of maps where each map represents a row.
func LoadFixture(ctx context.Context, db *gorm.DB, table string, data []map[string]any) error {
	ctx, cancel := operationContext(ctx)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	if migration.IsVersionTable(table) {
		return fmt.Errorf("migration metadata is not application fixture data")
	}
	if err := validateFixture(data); err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}

	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, row := range data {
			if err := namedTable(tx, table).Create(row).Error; err != nil {
				return fmt.Errorf("failed to insert fixture row into %s: %w", table, err)
			}
		}
		return nil
	})
}

// MustLoadFixture loads test data and fails the test on error.
func MustLoadFixture(t *testing.T, db *gorm.DB, table string, data []map[string]any) {
	t.Helper()
	if err := LoadFixture(t.Context(), db, table, data); err != nil {
		t.Fatalf("LoadFixture failed: %v", err)
	}
}

// TruncateTable removes all rows from a table.
func TruncateTable(ctx context.Context, db *gorm.DB, table string) error {
	ctx, cancel := operationContext(ctx)
	defer cancel()
	if migration.IsVersionTable(table) {
		return fmt.Errorf("cannot clear migration metadata")
	}
	return db.WithContext(ctx).Exec("DELETE FROM " + quoteTable(db, table)).Error
}

// TruncateAllTables removes all rows from all tables in the database.
func TruncateAllTables(ctx context.Context, db *gorm.DB) error {
	ctx, cancel := operationContext(ctx)
	defer cancel()
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tables, err := GetTableNames(ctx, tx)
		if err != nil {
			return err
		}
		return clearTables(tx, tables)
	})
}

func quoteTable(db *gorm.DB, name string) string {
	return db.Statement.Quote(clause.Table{Name: name})
}

func namedTable(db *gorm.DB, name string) *gorm.DB {
	return db.Scopes(func(scoped *gorm.DB) *gorm.DB {
		scoped.Statement.Table = name
		scoped.Statement.TableExpr = nil
		return scoped
	})
}

func clearTables(tx *gorm.DB, tables []string) error {
	if len(tables) == 0 {
		return nil
	}
	switch tx.Name() {
	case "sqlite":
		if err := tx.Exec("PRAGMA defer_foreign_keys=ON").Error; err != nil {
			return err
		}
		for _, table := range tables {
			if err := tx.Exec("DELETE FROM " + quoteTable(tx, table)).Error; err != nil {
				return err
			}
		}
		return nil
	case "postgres":
		quoted := make([]string, len(tables))
		for i, table := range tables {
			quoted[i] = quoteTable(tx, table)
		}
		return tx.Exec("TRUNCATE TABLE " + strings.Join(quoted, ", ") + " RESTART IDENTITY").Error
	default:
		return fmt.Errorf("fixture reset does not support dialect %q", tx.Name())
	}
}

// TableExists checks if a table exists in the database.
func TableExists(ctx context.Context, db *gorm.DB, table string) (bool, error) {
	tables, err := GetTableNames(ctx, db)
	if err != nil {
		return false, err
	}
	for _, existing := range tables {
		if existing == table {
			return true, nil
		}
	}
	return false, nil
}

// GetTableNames returns a list of all non-system tables.
func GetTableNames(ctx context.Context, db *gorm.DB) ([]string, error) {
	ctx, cancel := operationContext(ctx)
	defer cancel()
	var tables []string
	var query string
	switch db.Name() {
	case "sqlite":
		query = "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name <> 'schema_migrations' AND name NOT LIKE '%\\_schema\\_migrations' ESCAPE '\\' ORDER BY name LIMIT ?"
	case "postgres":
		query = "SELECT tablename FROM pg_tables WHERE schemaname=current_schema() AND tablename <> 'schema_migrations' AND tablename NOT LIKE '%\\_schema\\_migrations' ESCAPE '\\' ORDER BY tablename LIMIT ?"
	default:
		return nil, fmt.Errorf("fixture tables do not support dialect %q", db.Name())
	}
	err := db.WithContext(ctx).Raw(query, MaxFixtureTables+1).Scan(&tables).Error
	if err == nil && len(tables) > MaxFixtureTables {
		return nil, fmt.Errorf("fixture exceeds %d application tables", MaxFixtureTables)
	}
	return tables, err
}

// CountRows returns the number of rows in a table.
func CountRows(ctx context.Context, db *gorm.DB, table string) (int64, error) {
	ctx, cancel := operationContext(ctx)
	defer cancel()
	var count int64
	err := db.WithContext(ctx).Raw("SELECT COUNT(*) FROM " + quoteTable(db, table)).Scan(&count).Error
	return count, err
}

// AssertTableEmpty fails the test if the table is not empty.
func AssertTableEmpty(t *testing.T, db *gorm.DB, table string) {
	t.Helper()
	count, err := CountRows(t.Context(), db, table)
	if err != nil {
		t.Fatalf("failed to count rows in %s: %v", table, err)
	}
	if count != 0 {
		t.Errorf("table %s is not empty: has %d rows", table, count)
	}
}

// AssertRowCount fails the test if the table doesn't have the expected row count.
func AssertRowCount(t *testing.T, db *gorm.DB, table string, expected int64) {
	t.Helper()
	count, err := CountRows(t.Context(), db, table)
	if err != nil {
		t.Fatalf("failed to count rows in %s: %v", table, err)
	}
	if count != expected {
		t.Errorf("table %s row count = %d, want %d", table, count, expected)
	}
}
