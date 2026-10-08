package testutil

import (
	"context"
	"errors"
	"testing"
)

func TestComponentOwnsLifecycleAndStatelessSnapshots(t *testing.T) {
	c := NewComponent()
	if c.Server() == nil || c.Server().GinEngine() != c.GinEngine() {
		t.Fatal("component does not expose its owned server")
	}
	if err := c.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := c.Reset(t.Context()); err == nil {
		t.Fatal("Reset accepted a stopped component")
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := c.Start(canceled); !errors.Is(err, context.Canceled) || c.BaseURL() != "" {
		t.Fatalf("canceled startup = %v, origin = %q", err, c.BaseURL())
	}
	if err := c.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.Stop(context.Background()); err != nil {
			t.Error(err)
		}
	})
	if err := c.Start(t.Context()); err == nil {
		t.Fatal("duplicate startup succeeded")
	}
	if err := c.Reset(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Reset = %v", err)
	}
	snapshot, err := c.Snapshot(t.Context())
	if err != nil || snapshot != nil {
		t.Fatalf("stateless snapshot = %v, %v", snapshot, err)
	}
	if err := c.Restore(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
}
