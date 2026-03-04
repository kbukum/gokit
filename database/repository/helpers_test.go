package repository

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// testModel is a simple model used across all repository tests.
type testModel struct {
	ID   string `gorm:"primaryKey"`
	Name string
	Age  int
}

func (testModel) TableName() string { return "test_models" }

// setupTestDB creates an in-memory SQLite database with the test schema.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Discard,
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(&testModel{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// seedModel inserts a test record.
func seedModel(t *testing.T, db *gorm.DB, id, name string, age int) {
	t.Helper()
	m := testModel{ID: id, Name: name, Age: age}
	if err := db.Create(&m).Error; err != nil {
		t.Fatalf("failed to seed: %v", err)
	}
}
