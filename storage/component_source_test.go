package storage

import (
	"context"
	"errors"
	"testing"
)

func TestComponentSourceIsFailClosed(t *testing.T) {
	t.Parallel()
	c := NewComponent(stubRegistry(t, &stubStorage{}), Config{Provider: ProviderLocal, Enabled: true}, nil)
	if _, err := c.Source(); err == nil {
		t.Fatal("unstarted source succeeded")
	}
	if err := c.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if s, err := c.Source(); err != nil || s == nil {
		t.Fatalf("started source: %v, %v", s, err)
	}
	if err := c.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Source(); err == nil {
		t.Fatal("stopped source succeeded")
	}
}

func TestComponentCanceledStartDoesNotPublish(t *testing.T) {
	t.Parallel()
	c := NewComponent(stubRegistry(t, &stubStorage{}), Config{Provider: ProviderLocal, Enabled: true}, nil)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := c.Start(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled startup: %v", err)
	}
	if _, err := c.Source(); err == nil {
		t.Fatal("canceled start published a source")
	}
}
