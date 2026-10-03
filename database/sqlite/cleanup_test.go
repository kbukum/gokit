package sqlite_test

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kbukum/gokit/database/cleanup"
	"github.com/kbukum/gokit/util"
	"github.com/kbukum/gokit/worker"
)

type expiringRow struct {
	ID        uint `gorm:"primaryKey"`
	ExpiresAt time.Time
}

func TestScheduledExpiryCleanupStops(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		db := newTestDB(t)
		t.Cleanup(func() {
			if err := db.Close(); err != nil {
				t.Error(err)
			}
		})
		if err := db.AutoMigrate(ctx, &expiringRow{}); err != nil {
			t.Fatal(err)
		}
		clock := util.NewFakeClock(time.Now())
		for range 25 {
			if err := db.WithContext(ctx).Create(&expiringRow{ExpiresAt: clock.Now().Add(-time.Hour)}).Error; err != nil {
				t.Fatal(err)
			}
		}
		job := worker.NewTickerWorker("expiry", time.Second, func(ctx context.Context) error {
			_, err := cleanup.DeleteExpired[expiringRow](ctx, db.GormDB, cleanup.Config{ExpiryField: "expires_at", BatchSize: 10, Clock: clock})
			return err
		})
		if err := job.Start(ctx); err != nil {
			t.Fatal(err)
		}
		timer := time.NewTimer(3 * time.Second)
		<-timer.C
		synctest.Wait()
		if err := job.Stop(ctx); err != nil {
			t.Fatal(err)
		}
		if job.RunCount() != 3 || job.FailCount() != 0 {
			t.Fatalf("runs=%d failures=%d", job.RunCount(), job.FailCount())
		}
		var count int64
		if err := db.WithContext(ctx).Model(&expiringRow{}).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("remaining=%d err=%v", count, err)
		}
		timer.Reset(time.Hour)
		<-timer.C
		synctest.Wait()
		if job.RunCount() != 3 {
			t.Fatal("cleanup ran after shutdown")
		}
	})
}

func TestExpiryCleanupRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	if err := db.AutoMigrate(context.Background(), &expiringRow{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	for _, cfg := range []cleanup.Config{{BatchSize: -1}, {BatchSize: 1001}, {ExpiryField: "missing"}, {ExpiryField: "id"}} {
		if _, err := cleanup.DeleteExpired[expiringRow](context.Background(), db.GormDB, cfg); err == nil {
			t.Fatal("expected invalid configuration")
		}
	}
}

func TestExpiryCleanupIsBounded(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := newTestDB(t)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := db.AutoMigrate(ctx, &expiringRow{}); err != nil {
		t.Fatal(err)
	}
	clock := util.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	for i := range 26 {
		stamp := clock.Now().Add(-time.Hour)
		if i == 25 {
			stamp = clock.Now().Add(time.Hour)
		}
		if err := db.WithContext(ctx).Create(&expiringRow{ExpiresAt: stamp}).Error; err != nil {
			t.Fatal(err)
		}
	}
	cfg := cleanup.Config{ExpiryField: "expires_at", BatchSize: 10, Clock: clock}
	for _, expected := range []int64{10, 10, 5, 0} {
		deleted, err := cleanup.DeleteExpired[expiringRow](ctx, db.GormDB, cfg)
		if err != nil || deleted != expected {
			t.Fatalf("delete=%d expected=%d err=%v", deleted, expected, err)
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := cleanup.DeleteExpired[expiringRow](canceled, db.GormDB, cfg); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	clock.Advance(2 * time.Hour)
	if deleted, err := cleanup.DeleteExpired[expiringRow](ctx, db.GormDB, cfg); err != nil || deleted != 1 {
		t.Fatalf("last row: %d %v", deleted, err)
	}
}
