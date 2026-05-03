package database_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"gorm.io/gorm"

	. "github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/sqlite"
	"github.com/kbukum/gokit/logger"
)

// helper to create a DB instance with SQLite in-memory for testing.
func newTestDB(t *testing.T) *DB {
	t.Helper()
	cfg := Config{Enabled: true, DSN: ":memory:"}
	cfg.ApplyDefaults()
	log := logger.NewDefault("test")
	db, err := NewWithContext(context.Background(), sqlite.Open(cfg.DSN), cfg, log)
	if err != nil {
		t.Fatalf("newTestDB: %v", err)
	}
	return db
}

type testItem struct {
	ID   uint   `gorm:"primaryKey"`
	Name string `gorm:"size:255"`
}

// --- Transaction ---

func TestTransaction_CommitOnSuccess(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	db.AutoMigrate(&testItem{})

	err := db.Transaction(func(tx *gorm.DB) error {
		return tx.Create(&testItem{ID: 1, Name: "committed"}).Error
	})
	if err != nil {
		t.Fatalf("Transaction() error: %v", err)
	}

	var item testItem
	if err := db.GormDB.First(&item, 1).Error; err != nil {
		t.Fatalf("row not found after commit: %v", err)
	}
	if item.Name != "committed" {
		t.Errorf("Name = %q, want %q", item.Name, "committed")
	}
}

func TestTransaction_RollbackOnError(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	db.AutoMigrate(&testItem{})

	sentinelErr := errors.New("forced error")
	err := db.Transaction(func(tx *gorm.DB) error {
		tx.Create(&testItem{ID: 1, Name: "should-rollback"})
		return sentinelErr
	})
	if !errors.Is(err, sentinelErr) {
		t.Fatalf("expected sentinel error, got %v", err)
	}

	var count int64
	db.GormDB.Model(&testItem{}).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 rows after rollback, got %d", count)
	}
}

// --- WithTransaction ---

func TestWithTransaction_CommitOnSuccess(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	db.AutoMigrate(&testItem{})

	err := db.WithTransaction(context.Background(), func(tx *gorm.DB) error {
		return tx.Create(&testItem{ID: 1, Name: "via-with-tx"}).Error
	})
	if err != nil {
		t.Fatalf("WithTransaction() error: %v", err)
	}

	var item testItem
	if err := db.GormDB.First(&item, 1).Error; err != nil {
		t.Fatalf("row not found after WithTransaction commit: %v", err)
	}
	if item.Name != "via-with-tx" {
		t.Errorf("Name = %q, want %q", item.Name, "via-with-tx")
	}
}

func TestWithTransaction_RollbackOnError(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	db.AutoMigrate(&testItem{})

	sentinelErr := errors.New("tx-fail")
	err := db.WithTransaction(context.Background(), func(tx *gorm.DB) error {
		tx.Create(&testItem{ID: 1, Name: "should-vanish"})
		return sentinelErr
	})
	if !errors.Is(err, sentinelErr) {
		t.Fatalf("expected sentinel error, got %v", err)
	}

	var count int64
	db.GormDB.Model(&testItem{}).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 rows after rollback, got %d", count)
	}
}

func TestWithTransaction_PanicRecovery(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	db.AutoMigrate(&testItem{})

	// Insert a row first to verify rollback on panic
	db.GormDB.Create(&testItem{ID: 99, Name: "pre-existing"})

	recovered := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				recovered = true
			}
		}()
		_ = db.WithTransaction(context.Background(), func(tx *gorm.DB) error {
			tx.Create(&testItem{ID: 2, Name: "panic-row"})
			panic("test panic")
		})
	}()

	if !recovered {
		t.Fatal("expected panic to be re-raised")
	}

	// The panic-row should not be persisted
	var count int64
	db.GormDB.Model(&testItem{}).Where("name = ?", "panic-row").Count(&count)
	if count != 0 {
		t.Errorf("expected panic-row to be rolled back, but found %d rows", count)
	}

	// Pre-existing row should still be there
	var pre testItem
	if err := db.GormDB.First(&pre, 99).Error; err != nil {
		t.Errorf("pre-existing row should survive: %v", err)
	}
}

// --- WithReadOnlyTransaction ---

func TestWithReadOnlyTransaction_AlwaysRollsBack(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	db.AutoMigrate(&testItem{})

	err := db.WithReadOnlyTransaction(context.Background(), func(tx *gorm.DB) error {
		return tx.Create(&testItem{ID: 1, Name: "read-only-row"}).Error
	})
	if err != nil {
		t.Fatalf("WithReadOnlyTransaction() error: %v", err)
	}

	// Even though fn returned nil, the row should not persist
	var count int64
	db.GormDB.Model(&testItem{}).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 rows (always rollback), got %d", count)
	}
}

func TestWithReadOnlyTransaction_PropagatesFnError(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	sentinelErr := errors.New("read-only-fail")
	err := db.WithReadOnlyTransaction(context.Background(), func(tx *gorm.DB) error {
		return sentinelErr
	})
	if !errors.Is(err, sentinelErr) {
		t.Errorf("expected sentinel error, got %v", err)
	}
}

// --- Close ---

func TestClose_Idempotent(t *testing.T) {
	db := newTestDB(t)

	if err := db.Close(); err != nil {
		t.Fatalf("first Close() error: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("second Close() should be no-op, got: %v", err)
	}
}

// --- AutoMigrate ---

func TestAutoMigrate_CreatesTable(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	type Product struct {
		ID    uint
		Title string
	}

	if err := db.AutoMigrate(&Product{}); err != nil {
		t.Fatalf("AutoMigrate() error: %v", err)
	}

	if !db.GormDB.Migrator().HasTable(&Product{}) {
		t.Error("expected Product table to exist after AutoMigrate")
	}
}

func TestAutoMigrate_MultipleModels(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	type Alpha struct {
		ID uint
	}
	type Beta struct {
		ID uint
	}

	if err := db.AutoMigrate(&Alpha{}, &Beta{}); err != nil {
		t.Fatalf("AutoMigrate() error: %v", err)
	}

	if !db.GormDB.Migrator().HasTable(&Alpha{}) {
		t.Error("expected Alpha table to exist")
	}
	if !db.GormDB.Migrator().HasTable(&Beta{}) {
		t.Error("expected Beta table to exist")
	}
}

// --- Ping ---

func TestPing_Success(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Errorf("Ping() error: %v", err)
	}
}

// --- Concurrent queries ---

func TestConcurrentQueries(t *testing.T) {
	// Use shared cache for concurrent goroutine access
	cfg := Config{Enabled: true, DSN: "file::memory:?cache=shared"}
	cfg.ApplyDefaults()
	log := logger.NewDefault("test")
	db, err := NewWithContext(context.Background(), sqlite.Open(cfg.DSN), cfg, log)
	if err != nil {
		t.Fatalf("newTestDB: %v", err)
	}
	defer db.Close()

	if err := db.AutoMigrate(&testItem{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	for i := 1; i <= 10; i++ {
		if err := db.GormDB.Create(&testItem{ID: uint(i), Name: "item"}).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	var wg sync.WaitGroup
	errs := make(chan error, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var count int64
			if err := db.GormDB.Model(&testItem{}).Count(&count).Error; err != nil {
				errs <- err
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent query error: %v", err)
	}
}

// --- WithContext ---

func TestWithContext(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	type ctxKey string
	ctx := context.WithValue(context.Background(), ctxKey("key"), "val")
	gormDB := db.WithContext(ctx)
	if gormDB == nil {
		t.Error("WithContext() returned nil")
	}
}

// --- PingContext ---

func TestPingContext_Success(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	if err := db.PingContext(context.Background()); err != nil {
		t.Errorf("PingContext() error: %v", err)
	}
}
