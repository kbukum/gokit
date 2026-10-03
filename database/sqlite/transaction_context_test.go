package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"
)

func TestAutoMigrateHonorsCancellation(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := db.AutoMigrate(ctx, &testItem{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled migration: %v", err)
	}
}

func TestTransactionCanceledBeforeCommit(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := db.AutoMigrate(context.Background(), &testItem{}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := db.WithTransaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Create(&testItem{Name: "canceled"}).Error; err != nil {
			return err
		}
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("transaction cancellation: %v", err)
	}
}
