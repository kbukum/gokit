package storage

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kbukum/gokit/logging"
)

func TestComponentFactoryReceivesStartupDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		registry := NewFactoryRegistry()
		if err := registry.Register("waiting", func(ctx context.Context, _ Config, _ *logging.Logger) (Storage, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}); err != nil {
			t.Fatal(err)
		}
		component := NewComponent(registry, Config{Enabled: true, Provider: "waiting"}, nil)
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		if err := component.Start(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("startup did not preserve its deadline: %v", err)
		}
		if _, err := component.Source(); err == nil {
			t.Fatal("failed initialization published a backend")
		}
	})
}
