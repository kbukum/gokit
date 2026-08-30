package stateful

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestManager_ConcurrentGetOrCreate(t *testing.T) {
	mgr := NewManager(
		func(key string) *Accumulator[int] {
			return NewAccumulator(NewMemoryStore[int](), Config[int]{})
		},
		5*time.Minute,
	)
	defer mgr.Close()

	const goroutines = 50
	results := make([]*Accumulator[int], goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx] = mgr.GetOrCreate("shared-key")
		}(i)
	}

	wg.Wait()

	// All goroutines must receive the same accumulator instance
	for i := 1; i < goroutines; i++ {
		if results[i] != results[0] {
			t.Fatalf("goroutine %d got different accumulator instance", i)
		}
	}
}

// ---------------------------------------------------------------------------
// Manager: Keys() accuracy during concurrent modifications
// ---------------------------------------------------------------------------

func TestManager_KeysDuringConcurrentMods(t *testing.T) {
	mgr := NewManager(
		func(key string) *Accumulator[int] {
			return NewAccumulator(NewMemoryStore[int](), Config[int]{})
		},
		5*time.Minute,
	)
	defer mgr.Close()

	ctx := context.Background()
	const n = 20
	var wg sync.WaitGroup

	// Concurrently create accumulators
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			_ = mgr.Append(ctx, "key-"+string(rune('A'+i)), i)
		}(i)
	}
	wg.Wait()

	keys := mgr.List()
	if len(keys) != n {
		t.Errorf("expected %d keys, got %d", n, len(keys))
	}
}

// ---------------------------------------------------------------------------
// Manager: Flush/Size/Measure on non-existent key
// ---------------------------------------------------------------------------

func TestManager_NonExistentKeyOps(t *testing.T) {
	mgr := NewManager(
		func(key string) *Accumulator[int] {
			return NewAccumulator(NewMemoryStore[int](), Config[int]{})
		},
		5*time.Minute,
	)
	defer mgr.Close()

	ctx := context.Background()

	// Flush non-existent → nil, nil
	vals, err := mgr.Flush(ctx, "ghost")
	if err != nil || vals != nil {
		t.Errorf("Flush non-existent: expected (nil, nil), got (%v, %v)", vals, err)
	}

	// Size non-existent → 0, nil
	size, err := mgr.Size(ctx, "ghost")
	if err != nil || size != 0 {
		t.Errorf("Size non-existent: expected (0, nil), got (%d, %v)", size, err)
	}

	// Measure non-existent → 0, nil
	m, err := mgr.Measure(ctx, "ghost")
	if err != nil || m != 0 {
		t.Errorf("Measure non-existent: expected (0, nil), got (%d, %v)", m, err)
	}
}

// ---------------------------------------------------------------------------
// Manager: Close is idempotent
// ---------------------------------------------------------------------------

func TestManager_CloseIdempotent(t *testing.T) {
	mgr := NewManager(
		func(key string) *Accumulator[int] {
			return NewAccumulator(NewMemoryStore[int](), Config[int]{})
		},
		5*time.Minute,
	)

	ctx := context.Background()
	_ = mgr.Append(ctx, "k", 1)

	// Close twice should not panic
	if err := mgr.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := mgr.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Concurrent Append + Flush
// ---------------------------------------------------------------------------

func TestManager_TTL_KeepAlive_DuringActiveUse(t *testing.T) {
	mgr := NewManager(
		func(key string) *Accumulator[int] {
			return NewAccumulator(NewMemoryStore[int](), Config[int]{
				TTL:       80 * time.Millisecond,
				KeepAlive: true,
			})
		},
		80*time.Millisecond,
	)
	defer mgr.Close()

	ctx := context.Background()
	_ = mgr.Append(ctx, "active", 1)

	// Keep using it — should stay alive
	for i := 0; i < 5; i++ {
		time.Sleep(30 * time.Millisecond)
		_ = mgr.Append(ctx, "active", i+2)
	}

	// Should NOT be cleaned up because keep-alive resets TTL
	cleaned := mgr.Cleanup()
	if cleaned != 0 {
		t.Errorf("expected 0 cleanups (keep-alive active), got %d", cleaned)
	}

	acc := mgr.Get("active")
	if acc == nil {
		t.Fatal("accumulator should still exist")
	}

	size, _ := acc.Size(ctx)
	if size != 6 {
		t.Errorf("expected size 6, got %d", size)
	}
}

// ---------------------------------------------------------------------------
// Custom trigger that uses accumulator state
// ---------------------------------------------------------------------------

func TestManager_Cleanup_Expiration(t *testing.T) {
	expiredKeys := make(map[string]bool)
	var mu sync.Mutex

	mgr := NewManager(
		func(key string) *Accumulator[int] {
			return NewAccumulator(
				NewMemoryStore[int](),
				Config[int]{
					TTL:       50 * time.Millisecond,
					KeepAlive: false,
					OnExpire: func(ctx context.Context, k string) {
						mu.Lock()
						expiredKeys[k] = true
						mu.Unlock()
					},
				},
			)
		},
		50*time.Millisecond,
	)
	defer mgr.Close()

	ctx := context.Background()

	// Create accumulators
	mgr.Append(ctx, "user1", 1)
	mgr.Append(ctx, "user2", 2)

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Manually cleanup. Background cleanup may already have removed expired accumulators.
	count := mgr.Cleanup()
	if count > 2 {
		t.Errorf("expected cleanup count <= 2, got %d", count)
	}

	mu.Lock()
	if !expiredKeys["user1"] || !expiredKeys["user2"] {
		t.Errorf("expected both users expired, got %v", expiredKeys)
	}
	mu.Unlock()
}

// Test Manager operations
func TestManager_Operations(t *testing.T) {
	mgr := NewManager(
		func(key string) *Accumulator[int] {
			return NewAccumulator(
				NewMemoryStore[int](),
				Config[int]{},
			)
		},
		5*time.Minute,
	)
	defer mgr.Close()

	ctx := context.Background()

	// Get non-existent
	if acc := mgr.Get("nonexistent"); acc != nil {
		t.Error("expected nil for non-existent key")
	}

	// GetOrCreate
	acc1 := mgr.GetOrCreate("user1")
	if acc1 == nil {
		t.Fatal("expected accumulator")
	}

	// Get existing
	acc2 := mgr.Get("user1")
	if acc1 != acc2 {
		t.Error("expected same accumulator instance")
	}

	// Append
	if err := mgr.Append(ctx, "user1", 10); err != nil {
		t.Errorf("append failed: %v", err)
	}

	// Size
	size, err := mgr.Size(ctx, "user1")
	if err != nil || size != 1 {
		t.Errorf("expected size 1, got %d, err %v", size, err)
	}

	// Measure
	measured, err := mgr.Measure(ctx, "user1")
	if err != nil || measured != 1 {
		t.Errorf("expected measure 1, got %d, err %v", measured, err)
	}

	// List
	keys := mgr.List()
	if len(keys) != 1 || keys[0] != "user1" {
		t.Errorf("expected [user1], got %v", keys)
	}

	// Flush
	values, err := mgr.Flush(ctx, "user1")
	if err != nil || len(values) != 1 {
		t.Errorf("flush failed: %v, err %v", values, err)
	}

	// Delete
	if !mgr.Delete("user1") {
		t.Error("delete failed")
	}

	// Delete non-existent
	if mgr.Delete("user1") {
		t.Error("should return false for non-existent")
	}
}

// Test concurrent appends

// countingStore wraps MemoryStore to count Close calls, locking the
// close-once semantics of GetOrCreate losers, Delete, Cleanup, and Close.
type countingStore[V any] struct {
	*MemoryStore[V]
	closes *atomic.Int64
}

func (s *countingStore[V]) Close() error {
	s.closes.Add(1)
	return s.MemoryStore.Close()
}

func TestManager_AccumulatorsClosedExactlyOnce(t *testing.T) {
	var closes atomic.Int64
	mgr := NewManager(
		func(key string) *Accumulator[int] {
			return NewAccumulator[int](
				&countingStore[int]{MemoryStore: NewMemoryStore[int](), closes: &closes},
				Config[int]{},
			)
		},
		5*time.Minute,
	)

	const n = 8
	for i := 0; i < n; i++ {
		mgr.GetOrCreate("key-" + string(rune('A'+i)))
	}

	// Delete closes exactly one accumulator, once.
	if !mgr.Delete("key-A") {
		t.Fatal("Delete(key-A) = false, want true")
	}
	if got := closes.Load(); got != 1 {
		t.Fatalf("Delete closed %d times, want 1", got)
	}

	// Close drains the remaining accumulators, each closed exactly once.
	if err := mgr.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	if got := closes.Load(); got != int64(n) {
		t.Fatalf("total closes = %d, want %d (each accumulator once)", got, n)
	}

	// Close is idempotent — no double close.
	if err := mgr.Close(); err != nil {
		t.Fatalf("second Close() = %v", err)
	}
	if got := closes.Load(); got != int64(n) {
		t.Fatalf("closes after second Close = %d, want %d", got, n)
	}
}

// failingCloseStore wraps MemoryStore and returns a sentinel error from Close,
// while still counting closes, to verify Manager surfaces store close errors.
type failingCloseStore[V any] struct {
	*MemoryStore[V]
	err    error
	closes *atomic.Int64
}

func (s *failingCloseStore[V]) Close() error {
	if s.closes != nil {
		s.closes.Add(1)
	}
	_ = s.MemoryStore.Close()
	return s.err
}

// blockingCloseStore signals when its Close is entered and blocks there until
// released, letting a test hold an in-flight cleanup open to observe shutdown
// ordering deterministically instead of racing a sleep.
type blockingCloseStore[V any] struct {
	*MemoryStore[V]
	entered  chan struct{}
	release  chan struct{}
	signaler sync.Once
}

func (s *blockingCloseStore[V]) Close() error {
	s.signaler.Do(func() { close(s.entered) })
	<-s.release
	return s.MemoryStore.Close()
}

// Close blocks until an in-flight cleanup finishes rather than racing it; a
// post-Close Cleanup is then a no-op.
func TestManager_Close_WaitsForInFlightCleanup(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	mgr := NewManager(
		func(key string) *Accumulator[int] {
			return NewAccumulator[int](
				&blockingCloseStore[int]{
					MemoryStore: NewMemoryStore[int](),
					entered:     entered,
					release:     release,
				},
				Config[int]{TTL: 20 * time.Millisecond},
			)
		},
		20*time.Millisecond,
	)

	if err := mgr.Append(context.Background(), "k", 1); err != nil {
		t.Fatalf("Append() = %v", err)
	}

	// Wait until the cleanup goroutine has entered the accumulator's Close.
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup did not start within timeout")
	}

	// Close must not return while that cleanup is still in flight.
	closed := make(chan struct{})
	go func() {
		_ = mgr.Close()
		close(closed)
	}()

	select {
	case <-closed:
		t.Fatal("Close returned while cleanup was still in flight")
	case <-time.After(50 * time.Millisecond):
		// Expected: Close is blocked on the in-flight cleanup.
	}

	// Release the cleanup and confirm shutdown completes.
	close(release)
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return after cleanup was released")
	}

	// The cleanup goroutine has been drained; nothing remains to clean.
	if n := mgr.Cleanup(); n != 0 {
		t.Fatalf("Cleanup() after Close = %d, want 0", n)
	}
}

// Close invoked reentrantly from within an OnExpire callback (which runs on the
// cleanup goroutine) must not self-deadlock joining that goroutine.
func TestManager_Close_ReentrantFromOnExpire(t *testing.T) {
	done := make(chan struct{})
	var mgr *Manager[string, int]
	mgr = NewManager(
		func(key string) *Accumulator[int] {
			return NewAccumulator(NewMemoryStore[int](), Config[int]{
				TTL: 20 * time.Millisecond,
				OnExpire: func(_ context.Context, _ string) {
					_ = mgr.Close()
					close(done)
				},
			})
		},
		20*time.Millisecond,
	)

	if err := mgr.Append(context.Background(), "k", 1); err != nil {
		t.Fatalf("Append() = %v", err)
	}

	select {
	case <-done:
		// Reentrant Close returned without deadlocking.
	case <-time.After(2 * time.Second):
		t.Fatal("reentrant Close from OnExpire deadlocked")
	}

	// A subsequent external Close remains safe and idempotent.
	if err := mgr.Close(); err != nil {
		t.Fatalf("external Close() = %v", err)
	}
}

// GetOrCreate racing Close must never orphan an unclosed accumulator. The
// factory blocks mid-flight so a GetOrCreate is provably in progress when Close
// runs; on release it must close its just-created accumulator rather than
// storing it, without relying on a second Close to sweep it.
func TestManager_GetOrCreateRacingClose_NoOrphan(t *testing.T) {
	var closes, created atomic.Int64
	entered := make(chan struct{})
	release := make(chan struct{})
	mgr := NewManager(
		func(key string) *Accumulator[int] {
			created.Add(1)
			close(entered)
			<-release
			return NewAccumulator[int](
				&countingStore[int]{MemoryStore: NewMemoryStore[int](), closes: &closes},
				Config[int]{},
			)
		},
		5*time.Minute,
	)

	// Drive a single GetOrCreate and wait until it is blocked inside the factory.
	got := make(chan *Accumulator[int], 1)
	go func() { got <- mgr.GetOrCreate("k") }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("factory was not entered")
	}

	// Close while the create is in flight, then let the factory finish.
	if err := mgr.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	close(release)
	<-got

	if cr, c := created.Load(), closes.Load(); cr != 1 || c != 1 {
		t.Fatalf("created = %d, closes = %d; want 1 and 1 (created accumulator closed, not orphaned)", cr, c)
	}
	if acc := mgr.Get("k"); acc != nil {
		t.Fatalf("Get(k) = %v, want nil (nothing stored after Close)", acc)
	}
}

// Close aggregates and returns store close errors via errors.Join,
// while still closing every accumulator.
func TestManager_Close_ReturnsJoinedCloseErrors(t *testing.T) {
	errClose := errors.New("store close failed")
	var closes atomic.Int64
	mgr := NewManager(
		func(key string) *Accumulator[int] {
			return NewAccumulator[int](
				&failingCloseStore[int]{MemoryStore: NewMemoryStore[int](), err: errClose, closes: &closes},
				Config[int]{},
			)
		},
		5*time.Minute,
	)

	const n = 4
	for i := 0; i < n; i++ {
		mgr.GetOrCreate("key-" + string(rune('A'+i)))
	}

	err := mgr.Close()
	if !errors.Is(err, errClose) {
		t.Fatalf("Close() = %v, want it to surface errClose", err)
	}
	if got := closes.Load(); got != int64(n) {
		t.Fatalf("closes = %d, want %d (every accumulator closed despite errors)", got, n)
	}
}

// Delete's accumulator close error is captured and surfaced through Close.
func TestManager_Delete_CloseErrorSurfacedByClose(t *testing.T) {
	errClose := errors.New("store close failed")
	mgr := NewManager(
		func(key string) *Accumulator[int] {
			return NewAccumulator[int](
				&failingCloseStore[int]{MemoryStore: NewMemoryStore[int](), err: errClose},
				Config[int]{},
			)
		},
		5*time.Minute,
	)

	mgr.GetOrCreate("doomed")
	if !mgr.Delete("doomed") {
		t.Fatal("Delete(doomed) = false, want true")
	}

	if err := mgr.Close(); !errors.Is(err, errClose) {
		t.Fatalf("Close() = %v, want it to surface the Delete close error", err)
	}
}

// Happy-path Close returns nil.
func TestManager_Close_NilOnCleanClose(t *testing.T) {
	mgr := NewManager(
		func(key string) *Accumulator[int] {
			return NewAccumulator(NewMemoryStore[int](), Config[int]{})
		},
		5*time.Minute,
	)
	mgr.GetOrCreate("a")
	mgr.GetOrCreate("b")
	if err := mgr.Close(); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}
}

// After Close, Append must fail with ErrAccumulatorClosed rather than silently
// succeeding into an untracked accumulator whose store Close only released
// resources without rejecting later writes.
func TestManager_Append_AfterCloseFails(t *testing.T) {
	mgr := NewManager(
		func(key string) *Accumulator[int] {
			return NewAccumulator(NewMemoryStore[int](), Config[int]{})
		},
		5*time.Minute,
	)
	if err := mgr.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}

	if err := mgr.Append(context.Background(), "k", 1); !errors.Is(err, ErrAccumulatorClosed) {
		t.Fatalf("Append() after Close = %v, want ErrAccumulatorClosed", err)
	}
	if acc := mgr.Get("k"); acc != nil {
		t.Fatalf("Get(k) after Close = %v, want nil (nothing orphaned)", acc)
	}
}
