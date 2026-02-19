package testutil

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/testutil"
)

func TestComponent_Interfaces(t *testing.T) {
	comp := NewComponent()
	var _ component.Component = comp
	var _ testutil.TestComponent = comp
}

func TestComponent_Lifecycle(t *testing.T) {
	comp := NewComponent()
	ctx := context.Background()

	if comp.Client() != nil {
		t.Error("Client() should be nil before Start")
	}

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	if comp.Client() == nil {
		t.Error("Client() should not be nil after Start")
	}

	health := comp.Health(ctx)
	if health.Status != component.StatusHealthy {
		t.Errorf("Health Status = %q, want %q", health.Status, component.StatusHealthy)
	}

	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}
}

func TestComponent_SetGetReset(t *testing.T) {
	comp := NewComponent()
	ctx := context.Background()

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer comp.Stop(ctx)

	// Set a value
	if err := comp.Client().Set(ctx, "key1", "value1", 0).Err(); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	val, err := comp.Client().Get(ctx, "key1").Result()
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if val != "value1" {
		t.Errorf("Get = %q, want %q", val, "value1")
	}

	// Reset should flush
	if err := comp.Reset(ctx); err != nil {
		t.Fatalf("Reset() failed: %v", err)
	}

	_, err = comp.Client().Get(ctx, "key1").Result()
	if err == nil {
		t.Error("Get after Reset should fail")
	}
}

func TestComponent_SnapshotRestore(t *testing.T) {
	comp := NewComponent()
	ctx := context.Background()

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer comp.Stop(ctx)

	// Set some data
	comp.Client().Set(ctx, "a", "1", 0)
	comp.Client().Set(ctx, "b", "2", 0)

	// Snapshot
	snap, err := comp.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot() failed: %v", err)
	}

	// Modify data
	comp.Client().Set(ctx, "c", "3", 0)
	comp.Client().Del(ctx, "a")

	// Restore
	if err := comp.Restore(ctx, snap); err != nil {
		t.Fatalf("Restore() failed: %v", err)
	}

	// Verify restored state
	val, _ := comp.Client().Get(ctx, "a").Result()
	if val != "1" {
		t.Errorf("key 'a' = %q, want %q", val, "1")
	}
	val, _ = comp.Client().Get(ctx, "b").Result()
	if val != "2" {
		t.Errorf("key 'b' = %q, want %q", val, "2")
	}
	_, err = comp.Client().Get(ctx, "c").Result()
	if err == nil {
		t.Error("key 'c' should not exist after Restore")
	}
}
