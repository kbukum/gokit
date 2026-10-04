package database

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestIndexed1024DueRowsFourBoundedTicks(t *testing.T) {
	s, db, clock := fixture(t)
	ctx := context.Background()
	// 512 families plus their generation tombstones are 1,024 due physical rows.
	for i := range 512 {
		if err := s.Create(ctx, row(fmt.Sprintf("ref-%d", i), fmt.Sprintf("family-%d", i), clock)); err != nil {
			t.Fatal(err)
		}
	}
	clock.Advance(time.Hour + 10*time.Minute - time.Nanosecond)
	if n, err := s.Cleanup(ctx, clock.Now(), 256); err != nil || n != 0 {
		t.Fatal("early cleanup", n, err)
	}
	clock.Advance(time.Nanosecond)
	var total int64
	for range 4 {
		clock.Advance(10 * time.Second)
		start := time.Now()
		n, err := s.Cleanup(ctx, clock.Now(), 256)
		if err != nil || n > 256 || time.Since(start) > 500*time.Millisecond {
			t.Fatal("cleanup bound", n, err)
		}
		total += n
	}
	if total != 1024 {
		t.Fatalf("four ticks deleted %d rows", total)
	}
	var families, generations int64
	if err := db.WithContext(ctx).Model(&family{}).Count(&families).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Model(&generation{}).Count(&generations).Error; err != nil {
		t.Fatal(err)
	}
	if families != 0 || generations != 0 {
		t.Fatal("remaining due rows")
	}
}
