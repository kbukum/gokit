package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kbukum/gokit/logging"
)

func TestSourceDoesNotWaitForProviderInitialization(t *testing.T) {
	t.Parallel()
	entered := make(chan struct{})
	registry := NewFactoryRegistry()
	if err := registry.Register("waiting", func(ctx context.Context, _ Config, _ *logging.Logger) (Storage, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}); err != nil {
		t.Fatal(err)
	}
	c := NewComponent(registry, Config{Enabled: true, Provider: "waiting"}, nil)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := make(chan error, 1)
	go func() { started <- c.Start(ctx) }()
	<-entered
	source := make(chan error, 1)
	go func() {
		_, err := c.Source()
		source <- err
	}()
	blocked := false
	select {
	case err := <-source:
		if err == nil {
			t.Error("source appeared ready during initialization")
		}
	case <-time.After(2 * time.Second):
		blocked = true
	}
	cancel()
	if err := <-started; !errors.Is(err, context.Canceled) {
		t.Fatalf("startup: %v", err)
	}
	if blocked {
		<-source
		t.Fatal("source waited for the provider despite being unavailable")
	}
}
