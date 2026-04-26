package worker

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestKeyedPool_SubmitOrAttach_Coalesces(t *testing.T) {
	t.Parallel()

	var execCount atomic.Int32
	gate := make(chan struct{})
	h := HandlerFunc[string, string](func(ctx context.Context, in string, emit func(Event[string])) error {
		execCount.Add(1)
		<-gate
		emit(PartialEvent(in))
		return nil
	})
	pool := NewPool(h, PoolConfig{Name: "kp-test", Size: 2, EventBuffer: 4})
	defer pool.Stop(context.Background())
	kp := NewKeyedPool[string, string, string](pool)

	h1, attached1, err := kp.SubmitOrAttach(context.Background(), "k", "in")
	if err != nil || attached1 {
		t.Fatalf("first submit: attached=%v err=%v", attached1, err)
	}
	h2, attached2, err := kp.SubmitOrAttach(context.Background(), "k", "in")
	if err != nil || !attached2 {
		t.Fatalf("second submit: attached=%v err=%v", attached2, err)
	}
	if h1 != h2 {
		t.Fatalf("expected same handle for in-flight key")
	}
	if got := kp.Active(); got != 1 {
		t.Fatalf("Active=%d want 1", got)
	}

	close(gate)
	if _, err := h1.Result(); err != nil {
		t.Fatalf("Result: %v", err)
	}
	// Wait briefly for eviction goroutine.
	deadline := time.Now().Add(time.Second)
	for kp.Active() != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := kp.Active(); got != 0 {
		t.Fatalf("Active after Done=%d want 0", got)
	}
	if got := execCount.Load(); got != 1 {
		t.Fatalf("handler ran %d times; want 1 (coalesced)", got)
	}
}

func TestKeyedPool_DistinctKeys_RunIndependently(t *testing.T) {
	t.Parallel()

	var execCount atomic.Int32
	h := HandlerFunc[string, string](func(ctx context.Context, in string, emit func(Event[string])) error {
		execCount.Add(1)
		emit(PartialEvent(in))
		return nil
	})
	pool := NewPool(h, PoolConfig{Name: "kp-distinct", Size: 4, EventBuffer: 4})
	defer pool.Stop(context.Background())
	kp := NewKeyedPool[string, string, string](pool)

	var wg sync.WaitGroup
	for _, k := range []string{"a", "b", "c"} {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			h, _, err := kp.SubmitOrAttach(context.Background(), key, key)
			if err != nil {
				t.Errorf("submit %s: %v", key, err)
				return
			}
			_, _ = h.Result()
		}(k)
	}
	wg.Wait()
	if got := execCount.Load(); got != 3 {
		t.Fatalf("execCount=%d want 3", got)
	}
}

func TestKeyedPool_Cancel(t *testing.T) {
	t.Parallel()

	gate := make(chan struct{})
	h := HandlerFunc[string, string](func(ctx context.Context, in string, emit func(Event[string])) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-gate:
			return nil
		}
	})
	pool := NewPool(h, PoolConfig{Name: "kp-cancel", Size: 1, EventBuffer: 2})
	defer pool.Stop(context.Background())
	kp := NewKeyedPool[string, string, string](pool)

	handle, _, err := kp.SubmitOrAttach(context.Background(), "k", "x")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if !kp.Cancel("k") {
		t.Fatalf("Cancel returned false for in-flight key")
	}
	if _, err := handle.Result(); err == nil {
		t.Fatalf("expected cancellation error")
	}
	close(gate)
	// Wait for eviction goroutine to drain the inflight map.
	deadline := time.Now().Add(time.Second)
	for kp.Active() != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if kp.Cancel("k") {
		t.Fatalf("Cancel after completion should return false")
	}
}

func TestKeyedPool_Get(t *testing.T) {
	t.Parallel()

	gate := make(chan struct{})
	h := HandlerFunc[string, string](func(ctx context.Context, in string, emit func(Event[string])) error {
		<-gate
		return nil
	})
	pool := NewPool(h, PoolConfig{Name: "kp-get", Size: 1, EventBuffer: 2})
	defer pool.Stop(context.Background())
	kp := NewKeyedPool[string, string, string](pool)

	ctx := context.Background()
	if _, ok, err := kp.Get(ctx, "k"); ok || err != nil {
		t.Fatalf("Get on empty key: ok=%v err=%v", ok, err)
	}
	h1, _, _ := kp.SubmitOrAttach(ctx, "k", "x")
	got, ok, err := kp.Get(ctx, "k")
	if !ok || err != nil || got != h1 {
		t.Fatalf("Get returned wrong result: ok=%v err=%v handle==h1=%v", ok, err, got == h1)
	}
	close(gate)
	_, _ = h1.Result()
}

// Verifies the Get(ctx) ctx-aware blocking path: a Get call against a key
// whose entry exists but has not yet published a handle must respect ctx
// cancellation rather than block forever or lie about state.
func TestKeyedPool_Get_RespectsCtxDuringReservation(t *testing.T) {
	t.Parallel()

	// Stall pool.Submit by filling the pool's submit queue. Easier: hand-
	// craft an entry directly to simulate the reservation window.
	pool := NewPool(
		HandlerFunc[string, string](func(ctx context.Context, in string, emit func(Event[string])) error { return nil }),
		PoolConfig{Name: "kp-get-ctx", Size: 1, EventBuffer: 1},
	)
	defer pool.Stop(context.Background())
	kp := NewKeyedPool[string, string, string](pool)

	// Inject a phantom entry that will never become ready.
	cancelSubmit := func() {}
	entry := &keyedEntry[string]{ready: make(chan struct{}), cancelSubmit: cancelSubmit}
	kp.mu.Lock()
	kp.inflight["phantom"] = entry
	kp.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already-canceled ctx
	if _, ok, err := kp.Get(ctx, "phantom"); ok || err == nil {
		t.Fatalf("Get with canceled ctx: ok=%v err=%v; want false + ctx.Err", ok, err)
	}

	// Cleanup so other tests aren't affected.
	kp.mu.Lock()
	delete(kp.inflight, "phantom")
	kp.mu.Unlock()
	close(entry.ready)
}
