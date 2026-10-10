package sqlite_test

import (
	"errors"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestNestedTransactionSavepointRollback(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := db.AutoMigrate(t.Context(), &testItem{}); err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("rollback nested transaction")
	err := db.WithTransaction(t.Context(), func(tx *gorm.DB) error {
		if err := tx.Create(&testItem{Name: "outer"}).Error; err != nil {
			return err
		}
		err := tx.Transaction(func(nested *gorm.DB) error {
			if err := nested.Create(&testItem{Name: "inner"}).Error; err != nil {
				return err
			}
			return rollback
		})
		if !errors.Is(err, rollback) {
			t.Errorf("nested rollback: %v", err)
			return err
		}
		return tx.Create(&testItem{Name: "after"}).Error
	})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	if err := db.WithContext(t.Context()).Model(&testItem{}).Order("id").Pluck("name", &names).Error; err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "outer" || names[1] != "after" {
		t.Fatalf("savepoint rollback changed outer transaction: %v", names)
	}
}

func TestSQLiteTranslateError(t *testing.T) {
	t.Parallel()
	db, err := gorm.Open(preparedDialector(t, ":memory:"), &gorm.Config{TranslateError: true, Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Error(err)
		}
	})
	db = db.WithContext(t.Context())
	if err := db.AutoMigrate(&testItem{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&testItem{ID: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&testItem{ID: 1}).Error; !errors.Is(err, gorm.ErrDuplicatedKey) {
		t.Fatalf("duplicate key not translated: %v", err)
	}
}
