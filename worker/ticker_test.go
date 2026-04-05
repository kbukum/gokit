package worker_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/worker"
)

func TestTickerWorkerRunsOnInterval(t *testing.T) {
	var count atomic.Int32
	tw := worker.NewTickerWorker("test-ticker", 20*time.Millisecond, func(ctx context.Context) error {
		count.Add(1)
		return nil
	})

	if err := tw.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Wait enough for ~3 ticks
	time.Sleep(80 * time.Millisecond)

	if err := tw.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}

	c := count.Load()
	if c < 2 {
		t.Errorf("expected at least 2 ticks, got %d", c)
	}
}

func TestTickerWorkerHealthy(t *testing.T) {
	tw := worker.NewTickerWorker("health-test", 10*time.Millisecond, func(ctx context.Context) error {
		return nil
	})

	// Before start: unhealthy
	h := tw.Health(context.Background())
	if h.Status != component.StatusUnhealthy {
		t.Errorf("expected unhealthy before start, got %s", h.Status)
	}

	_ = tw.Start(context.Background())
	time.Sleep(30 * time.Millisecond)

	h = tw.Health(context.Background())
	if h.Status != component.StatusHealthy {
		t.Errorf("expected healthy, got %s", h.Status)
	}

	_ = tw.Stop(context.Background())
}

func TestTickerWorkerDegradedOnError(t *testing.T) {
	tw := worker.NewTickerWorker("error-test", 10*time.Millisecond, func(ctx context.Context) error {
		return errors.New("boom")
	})

	_ = tw.Start(context.Background())
	time.Sleep(30 * time.Millisecond)

	h := tw.Health(context.Background())
	if h.Status != component.StatusDegraded {
		t.Errorf("expected degraded, got %s", h.Status)
	}
	if h.Message != "boom" {
		t.Errorf("expected message 'boom', got %q", h.Message)
	}

	_ = tw.Stop(context.Background())
}

func TestTickerWorkerStopIsIdempotent(t *testing.T) {
	tw := worker.NewTickerWorker("idempotent", 10*time.Millisecond, func(ctx context.Context) error {
		return nil
	})

	// Stop before start should be fine
	if err := tw.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}

	_ = tw.Start(context.Background())
	_ = tw.Stop(context.Background())

	// Double stop should be fine
	if err := tw.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestTickerWorkerName(t *testing.T) {
	tw := worker.NewTickerWorker("my-worker", time.Second, func(ctx context.Context) error {
		return nil
	})
	if tw.Name() != "my-worker" {
		t.Errorf("Name() = %q, want %q", tw.Name(), "my-worker")
	}
}

func TestTickerWorkerRunCount(t *testing.T) {
	tw := worker.NewTickerWorker("count-test", 10*time.Millisecond, func(ctx context.Context) error {
		return nil
	})

	_ = tw.Start(context.Background())
	time.Sleep(35 * time.Millisecond)
	_ = tw.Stop(context.Background())

	if tw.RunCount() < 2 {
		t.Errorf("expected at least 2 runs, got %d", tw.RunCount())
	}
}
