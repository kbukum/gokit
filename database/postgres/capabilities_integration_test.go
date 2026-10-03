//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/postgres"
	"github.com/kbukum/gokit/logging"
)

func TestPostgresTimestampTimezone(t *testing.T) {
	connection, err := url.Parse(newDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	connection.RawQuery += "&timezone=America/New_York"
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	db, err := database.NewWithContext(ctx, postgres.Open(connection.String()), database.Config{}, logging.NewDefault("test"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	var stamp time.Time
	if err := db.WithContext(ctx).Raw("SELECT TIMESTAMP '2026-01-15 12:00:00'").Row().Scan(&stamp); err != nil {
		t.Fatal(err)
	}
	expected := time.Date(2026, time.January, 15, 12, 0, 0, 0, time.FixedZone("EST", -5*60*60))
	if !stamp.Equal(expected) {
		t.Fatalf("timestamp timezone lost: got %s, want %s", stamp, expected)
	}
}

func TestPostgresNestedTransactionsAndTranslation(t *testing.T) {
	dsn := newDSN(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	db, err := database.NewWithContext(ctx, postgres.Open(dsn), database.Config{}, logging.NewDefault("test"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	db.GormDB.TranslateError = true
	if err := db.AutoMigrate(ctx, &widget{}); err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("rollback nested transaction")
	err = db.WithTransaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Create(&widget{ID: 1, Name: "outer"}).Error; err != nil {
			return err
		}
		err := tx.Transaction(func(nested *gorm.DB) error {
			if err := nested.Create(&widget{ID: 2, Name: "inner"}).Error; err != nil {
				return err
			}
			return rollback
		})
		if !errors.Is(err, rollback) {
			t.Errorf("nested rollback: %v", err)
			return err
		}
		return tx.Create(&widget{ID: 3, Name: "after"}).Error
	})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	if err := db.WithContext(ctx).Model(&widget{}).Order("id").Pluck("name", &names).Error; err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "outer" || names[1] != "after" {
		t.Fatalf("savepoint rollback changed outer transaction: %v", names)
	}
	if err := db.WithContext(ctx).Create(&widget{ID: 1}).Error; !errors.Is(err, gorm.ErrDuplicatedKey) {
		t.Fatalf("duplicate key not translated: %v", err)
	}
}
