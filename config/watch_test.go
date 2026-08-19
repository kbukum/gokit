package config

import (
	"context"
	"testing"
)

func drainKinds(t *testing.T, ch <-chan ConfigChange, n int) []ConfigChange {
	t.Helper()
	got := make([]ConfigChange, 0, n)
	for range n {
		select {
		case c, ok := <-ch:
			if !ok {
				t.Fatalf("channel closed early after %d of %d events", len(got), n)
			}
			got = append(got, c)
		case <-context.Background().Done():
		}
	}
	return got
}

func TestInMemoryConfigSinkSatisfiesConfigWatch(t *testing.T) {
	t.Parallel()
	var _ ConfigWatch = NewInMemoryConfigSink()
}

func TestConfigWatchEmitsSetAndRemove(t *testing.T) {
	t.Parallel()

	sink := NewInMemoryConfigSink()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := sink.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	if err := sink.Set("db.url", NewSecretString("secret")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := sink.Remove("db.url"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	events := drainKinds(t, ch, 2)
	if events[0].Kind != ChangeSet || events[0].Key != "db.url" {
		t.Errorf("event 0 = %+v, want Set db.url", events[0])
	}
	if events[1].Kind != ChangeRemoved || events[1].Key != "db.url" {
		t.Errorf("event 1 = %+v, want Removed db.url", events[1])
	}
}

func TestConfigWatchClosesOnCancel(t *testing.T) {
	t.Parallel()

	sink := NewInMemoryConfigSink()
	ctx, cancel := context.WithCancel(context.Background())

	ch, err := sink.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	cancel()
	for range ch { //nolint:revive // drain until the source closes the channel
	}
}
