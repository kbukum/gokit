package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/kbukum/gokit/component"
)

type checkedStorage struct {
	closingStorage
	checkErr error
}

func (s *checkedStorage) Check(context.Context) error { return s.checkErr }

func TestComponentVerifiesBackendBeforePublishingAndDuringHealth(t *testing.T) {
	t.Parallel()
	want := errors.New("bucket unavailable")
	backend := &checkedStorage{checkErr: want}
	c := NewComponent(stubRegistry(t, backend), Config{Provider: ProviderLocal, Enabled: true}, nil)
	if err := c.Start(t.Context()); !errors.Is(err, want) {
		t.Fatalf("unready backend published: %v", err)
	}
	if _, err := c.Source(); err == nil || backend.closed.Load() != 1 {
		t.Fatal("unready backend remained owned or available")
	}
	backend.checkErr = nil
	if err := c.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	backend.checkErr = want
	if h := c.Health(t.Context()); h.Status != component.StatusUnhealthy {
		t.Fatalf("backend outage looked healthy: %+v", h)
	}
	if err := c.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
}
