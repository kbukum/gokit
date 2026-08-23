package component

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestHealthConstructors(t *testing.T) {
	t.Parallel()
	h := Healthy("db")
	if h.Status != StatusHealthy || h.Name != "db" || h.Message != "" || !h.IsHealthy() {
		t.Fatalf("Healthy = %+v", h)
	}
	d := Degraded("db", "slow")
	if d.Status != StatusDegraded || d.Message != "slow" || d.IsHealthy() {
		t.Fatalf("Degraded = %+v", d)
	}
	u := Unhealthy("db", "down")
	if u.Status != StatusUnhealthy || u.Message != "down" || u.IsHealthy() {
		t.Fatalf("Unhealthy = %+v", u)
	}
}

func TestRegistryConfigDefaults(t *testing.T) {
	t.Parallel()
	cfg := RegistryConfig{}.withDefaults()
	if cfg.StartTimeout != DefaultStartTimeout || cfg.StopTimeout != DefaultStopTimeout {
		t.Fatalf("defaults not applied: %+v", cfg)
	}
	if got := DefaultRegistryConfig().Concurrency; got != 1 {
		t.Fatalf("default concurrency = %d, want 1", got)
	}
}

// blockingComponent blocks in Start until its context is canceled, recording whether
// Stop was invoked for start-timeout cleanup verification.
type blockingComponent struct {
	name     string
	stopped  atomic.Bool
	startErr error
}

func (b *blockingComponent) Name() string { return b.name }
func (b *blockingComponent) Start(ctx context.Context) error {
	if b.startErr != nil {
		return b.startErr
	}
	<-ctx.Done()
	return ctx.Err()
}
func (b *blockingComponent) Stop(context.Context) error { b.stopped.Store(true); return nil }
func (b *blockingComponent) Health(context.Context) Health {
	return Healthy(b.name)
}

func TestStartAllStartTimeoutCleansUp(t *testing.T) {
	t.Parallel()
	r := NewRegistryWithConfig(RegistryConfig{StartTimeout: 20 * time.Millisecond})
	bc := &blockingComponent{name: "stuck"}
	if err := r.Register(bc); err != nil {
		t.Fatal(err)
	}
	err := r.StartAll(context.Background())
	if err == nil {
		t.Fatal("expected start timeout error")
	}
	if !bc.stopped.Load() {
		t.Fatal("expected Stop cleanup after start timeout")
	}
	if state, _ := r.State("stuck"); state != StateFailed {
		t.Fatalf("state = %v, want failed", state)
	}
}

func TestStartAllConcurrentRunsInParallel(t *testing.T) {
	t.Parallel()
	names := []string{"a", "b", "c"}
	r := NewRegistryWithConfig(RegistryConfig{Concurrency: 0})
	// A barrier that only releases once every component has entered Start proves the
	// starts overlap: if StartAllConcurrent serialized them, the first Start would block
	// forever waiting for peers that never arrive, deterministically failing the test via
	// the context deadline rather than a timing-sensitive sample.
	barrier := newStartBarrier(len(names))
	for _, n := range names {
		if err := r.Register(&barrierComponent{name: n, barrier: barrier}); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.StartAllConcurrent(ctx); err != nil {
		t.Fatalf("StartAllConcurrent: %v", err)
	}
}

func TestStartAllConcurrentRollsBackOnFailure(t *testing.T) {
	t.Parallel()
	r := NewRegistryWithConfig(RegistryConfig{Concurrency: 2})
	good := &mockComponent{name: "good", health: Healthy("good")}
	bad := &blockingComponent{name: "bad", startErr: errors.New("boom")}
	if err := r.Register(good); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(bad); err != nil {
		t.Fatal(err)
	}
	if err := r.StartAllConcurrent(context.Background()); err == nil {
		t.Fatal("expected failure")
	}
	// good must have been rolled back to stopped.
	if state, _ := r.State("good"); state != StateStopped && state != StateFailed {
		t.Fatalf("good state = %v, want stopped/failed after rollback", state)
	}
}

// startBarrier blocks every caller of wait until target callers have arrived, so a test
// can assert that that many goroutines were in a critical section simultaneously.
type startBarrier struct {
	mu       sync.Mutex
	count    int
	target   int
	released chan struct{}
}

func newStartBarrier(target int) *startBarrier {
	return &startBarrier{target: target, released: make(chan struct{})}
}

func (b *startBarrier) wait(ctx context.Context) error {
	b.mu.Lock()
	b.count++
	if b.count >= b.target {
		close(b.released)
	}
	b.mu.Unlock()
	select {
	case <-b.released:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type barrierComponent struct {
	name    string
	barrier *startBarrier
}

func (b *barrierComponent) Name() string                    { return b.name }
func (b *barrierComponent) Start(ctx context.Context) error { return b.barrier.wait(ctx) }
func (b *barrierComponent) Stop(context.Context) error      { return nil }
func (b *barrierComponent) Health(context.Context) Health   { return Healthy(b.name) }

func TestLazyComponentDefersConstruction(t *testing.T) {
	t.Parallel()
	var built atomic.Int32
	lazy := NewLazyComponent("cache", func() Component {
		built.Add(1)
		return &mockComponent{name: "cache", health: Healthy("cache")}
	})
	if built.Load() != 0 {
		t.Fatal("factory ran before Start")
	}
	if h := lazy.Health(context.Background()); h.Status != StatusDegraded {
		t.Fatalf("pre-start health = %v, want degraded", h.Status)
	}
	if err := lazy.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if built.Load() != 1 {
		t.Fatalf("factory ran %d times, want 1", built.Load())
	}
	if err := lazy.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if built.Load() != 1 {
		t.Fatalf("factory rebuilt on second Start: %d", built.Load())
	}
	if h := lazy.Health(context.Background()); !h.IsHealthy() {
		t.Fatalf("post-start health = %v, want healthy", h.Status)
	}
	if err := lazy.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestLazyComponentStopWithoutStart(t *testing.T) {
	t.Parallel()
	lazy := NewLazyComponent("x", func() Component { return &mockComponent{name: "x"} })
	if err := lazy.Stop(context.Background()); err != nil {
		t.Fatalf("Stop before Start = %v, want nil", err)
	}
}
