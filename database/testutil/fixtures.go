package testutil

import (
	"fmt"
	"testing"

	"gorm.io/gorm"
)

// LoadFixture loads test data into a table.
// Data should be a slice of maps where each map represents a row.
func LoadFixture(db *gorm.DB, table string, data []map[string]interface{}) error {
	if len(data) == 0 {
		return nil
	}

	for _, row := range data {
		if err := db.Table(table).Create(row).Error; err != nil {
			return fmt.Errorf("failed to insert fixture row into %s: %w", table, err)
		}
	}

	return nil
}

// MustLoadFixture loads test data and fails the test on error.
func MustLoadFixture(t *testing.T, db *gorm.DB, table string, data []map[string]interface{}) {
	t.Helper()
	if err := LoadFixture(db, table, data); err != nil {
		t.Fatalf("LoadFixture failed: %v", err)
	}
}

// TruncateTable removes all rows from a table.
func TruncateTable(db *gorm.DB, table string) error {
	return db.Exec(fmt.Sprintf("DELETE FROM %s", table)).Error
}

// TruncateAllTables removes all rows from all tables in the database.
func TruncateAllTables(db *gorm.DB) error {
	tables, err := GetTableNames(db)
	if err != nil {
		return err
	}

	for _, table := range tables {
		if err := TruncateTable(db, table); err != nil {
			return err
		}
	}

	return nil
}

// TableExists checks if a table exists in the database.
func TableExists(db *gorm.DB, table string) bool {
	return db.Migrator().HasTable(table)
}

// GetTableNames returns a list of all non-system tables.
func GetTableNames(db *gorm.DB) ([]string, error) {
	var tables []string
	err := db.Raw("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'").
		Scan(&tables).Error
	return tables, err
}

// CountRows returns the number of rows in a table.
func CountRows(db *gorm.DB, table string) (int64, error) {
	var count int64
	err := db.Raw(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count).Error
	return count, err
}

// AssertTableEmpty fails the test if the table is not empty.
func AssertTableEmpty(t *testing.T, db *gorm.DB, table string) {
	t.Helper()
	count, err := CountRows(db, table)
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
	count, err := CountRows(db, table)
	if err != nil {
		t.Fatalf("failed to count rows in %s: %v", table, err)
	}
	if count != expected {
		t.Errorf("table %s row count = %d, want %d", table, count, expected)
	}
}
