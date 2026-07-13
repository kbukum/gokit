package resilience

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBulkhead_AllowsRequestsWithinLimit(t *testing.T) {
	b := NewBulkhead(BulkheadConfig{
		Name:          "test",
		MaxConcurrent: 3,
	})

	var callCount int32

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := b.Execute(context.Background(), func() error {
				atomic.AddInt32(&callCount, 1)
				time.Sleep(10 * time.Millisecond)
				return nil
			})
			if err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		}()
	}

	wg.Wait()

	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestBulkhead_RejectsWhenFull(t *testing.T) {
	b := NewBulkhead(BulkheadConfig{
		Name:          "test",
		MaxConcurrent: 1,
		MaxWait:       0, // Fail immediately
	})

	// Block the single slot
	started := make(chan struct{})
	release := make(chan struct{})

	go func() {
		b.Execute(context.Background(), func() error {
			close(started)
			<-release
			return nil
		})
	}()

	<-started // Wait for first execution to start

	// Try another request - should be rejected
	err := b.Execute(context.Background(), func() error {
		return nil
	})

	if !errors.Is(err, ErrBulkheadFull) {
		t.Errorf("expected ErrBulkheadFull, got %v", err)
	}

	close(release)
}

func TestBulkhead_WaitsForSlot(t *testing.T) {
	b := NewBulkhead(BulkheadConfig{
		Name:          "test",
		MaxConcurrent: 1,
		MaxWait:       100 * time.Millisecond,
	})

	// Block the single slot briefly
	started := make(chan struct{})

	go func() {
		b.Execute(context.Background(), func() error {
			close(started)
			time.Sleep(20 * time.Millisecond)
			return nil
		})
	}()

	<-started // Wait for first execution to start

	// This should wait and eventually succeed
	start := time.Now()
	err := b.Execute(context.Background(), func() error {
		return nil
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Should have waited at least a bit
	if elapsed < 10*time.Millisecond {
		t.Errorf("expected some wait time, got %v", elapsed)
	}
}

func TestBulkhead_TimesOutWaiting(t *testing.T) {
	b := NewBulkhead(BulkheadConfig{
		Name:          "test",
		MaxConcurrent: 1,
		MaxWait:       10 * time.Millisecond,
	})

	// Block the single slot for longer than MaxWait
	started := make(chan struct{})
	release := make(chan struct{})

	go func() {
		b.Execute(context.Background(), func() error {
			close(started)
			<-release
			return nil
		})
	}()

	<-started

	err := b.Execute(context.Background(), func() error {
		return nil
	})

	if !errors.Is(err, ErrBulkheadTimeout) {
		t.Errorf("expected ErrBulkheadTimeout, got %v", err)
	}

	close(release)
}

func TestBulkhead_RespectsContext(t *testing.T) {
	b := NewBulkhead(BulkheadConfig{
		Name:          "test",
		MaxConcurrent: 1,
		MaxWait:       1 * time.Second,
	})

	// Block the single slot
	started := make(chan struct{})
	release := make(chan struct{})

	go func() {
		b.Execute(context.Background(), func() error {
			close(started)
			<-release
			return nil
		})
	}()

	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := b.Execute(ctx, func() error {
		return nil
	})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}

	close(release)
}

func TestBulkhead_Callbacks(t *testing.T) {
	var acquired, released, rejected int32

	b := NewBulkhead(BulkheadConfig{
		Name:          "test",
		MaxConcurrent: 1,
		MaxWait:       0,
		OnAcquire: func(name string) {
			atomic.AddInt32(&acquired, 1)
		},
		OnRelease: func(name string) {
			atomic.AddInt32(&released, 1)
		},
		OnReject: func(name string) {
			atomic.AddInt32(&rejected, 1)
		},
	})

	// Block the slot
	started := make(chan struct{})
	release := make(chan struct{})

	go func() {
		b.Execute(context.Background(), func() error {
			close(started)
			<-release
			return nil
		})
	}()

	<-started

	// This should be rejected
	_ = b.Execute(context.Background(), func() error {
		return nil
	})

	close(release)
	time.Sleep(10 * time.Millisecond) // Allow goroutine to finish

	if v := atomic.LoadInt32(&acquired); v != 1 {
		t.Errorf("expected 1 acquire callback, got %d", v)
	}
	if v := atomic.LoadInt32(&released); v != 1 {
		t.Errorf("expected 1 release callback, got %d", v)
	}
	if v := atomic.LoadInt32(&rejected); v != 1 {
		t.Errorf("expected 1 reject callback, got %d", v)
	}
}

func TestBulkhead_AvailableAndInUse(t *testing.T) {
	b := NewBulkhead(BulkheadConfig{
		Name:          "test",
		MaxConcurrent: 3,
	})

	if b.Available() != 3 {
		t.Errorf("expected 3 available, got %d", b.Available())
	}
	if b.InUse() != 0 {
		t.Errorf("expected 0 in use, got %d", b.InUse())
	}

	started := make(chan struct{})
	release := make(chan struct{})

	go func() {
		b.Execute(context.Background(), func() error {
			close(started)
			<-release
			return nil
		})
	}()

	<-started

	if b.Available() != 2 {
		t.Errorf("expected 2 available, got %d", b.Available())
	}
	if b.InUse() != 1 {
		t.Errorf("expected 1 in use, got %d", b.InUse())
	}

	close(release)
	time.Sleep(10 * time.Millisecond)

	if b.Available() != 3 {
		t.Errorf("expected 3 available after release, got %d", b.Available())
	}
}

func TestExecuteWithResult(t *testing.T) {
	b := NewBulkhead(DefaultBulkheadConfig("test"))

	result, err := ExecuteWithResult(context.Background(), b, func() (int, error) {
		return 42, nil
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}
}

func TestNewBulkhead_DefaultsMaxConcurrent(t *testing.T) {
	b := NewBulkhead(BulkheadConfig{Name: "t"})
	if b.Available() != 10 {
		t.Fatalf("expected default MaxConcurrent 10, got %d", b.Available())
	}
}

func TestBulkhead_ExactlyMaxConcurrentAllSucceed(t *testing.T) {
	const maxConcurrent = 5
	b := NewBulkhead(BulkheadConfig{
		Name:          "exact",
		MaxConcurrent: maxConcurrent,
	})

	var running int32
	var maxRunning int32
	var wg sync.WaitGroup

	for i := 0; i < maxConcurrent; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := b.Execute(context.Background(), func() error {
				cur := atomic.AddInt32(&running, 1)
				for {
					old := atomic.LoadInt32(&maxRunning)
					if cur <= old || atomic.CompareAndSwapInt32(&maxRunning, old, cur) {
						break
					}
				}
				time.Sleep(20 * time.Millisecond)
				atomic.AddInt32(&running, -1)
				return nil
			})
			if err != nil {
				t.Errorf("expected success, got %v", err)
			}
		}()
	}
	wg.Wait()

	if int(maxRunning) != maxConcurrent {
		t.Errorf("expected peak concurrency %d, got %d", maxConcurrent, maxRunning)
	}
}

func TestBulkhead_MaxConcurrentPlusOneRejected(t *testing.T) {
	const maxConcurrent = 2
	b := NewBulkhead(BulkheadConfig{
		Name:          "plus-one",
		MaxConcurrent: maxConcurrent,
		MaxWait:       0,
	})

	started := make(chan struct{}, maxConcurrent)
	release := make(chan struct{})
	var wg sync.WaitGroup

	// Fill all slots.
	for i := 0; i < maxConcurrent; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = b.Execute(context.Background(), func() error {
				started <- struct{}{}
				<-release
				return nil
			})
		}()
	}
	for i := 0; i < maxConcurrent; i++ {
		<-started
	}

	// The (maxConcurrent+1)th call should be rejected.
	err := b.Execute(context.Background(), func() error { return nil })
	if !errors.Is(err, ErrBulkheadFull) {
		t.Errorf("expected ErrBulkheadFull, got %v", err)
	}

	close(release)
	wg.Wait()
}

func TestBulkhead_MaxWaitTimeoutPrecision(t *testing.T) {
	const maxWait = 50 * time.Millisecond
	b := NewBulkhead(BulkheadConfig{
		Name:          "precision",
		MaxConcurrent: 1,
		MaxWait:       maxWait,
	})

	started := make(chan struct{})
	release := make(chan struct{})
	go func() {
		b.Execute(context.Background(), func() error {
			close(started)
			<-release
			return nil
		})
	}()
	<-started

	start := time.Now()
	err := b.Execute(context.Background(), func() error { return nil })
	elapsed := time.Since(start)

	if !errors.Is(err, ErrBulkheadTimeout) {
		t.Errorf("expected ErrBulkheadTimeout, got %v", err)
	}

	const tolerance = 50 * time.Millisecond
	if elapsed < maxWait-tolerance || elapsed > maxWait+tolerance {
		t.Errorf("timeout precision: expected ~%v, got %v", maxWait, elapsed)
	}

	close(release)
}

func TestBulkhead_CallbackOrdering(t *testing.T) {
	var events []string
	var mu sync.Mutex

	b := NewBulkhead(BulkheadConfig{
		Name:          "order",
		MaxConcurrent: 1,
		OnAcquire: func(_ string) {
			mu.Lock()
			events = append(events, "acquire")
			mu.Unlock()
		},
		OnRelease: func(_ string) {
			mu.Lock()
			events = append(events, "release")
			mu.Unlock()
		},
	})

	_ = b.Execute(context.Background(), func() error {
		mu.Lock()
		events = append(events, "fn")
		mu.Unlock()
		return nil
	})

	mu.Lock()
	defer mu.Unlock()

	expected := []string{"acquire", "fn", "release"}
	if len(events) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, events)
	}
	for i, e := range expected {
		if events[i] != e {
			t.Errorf("event[%d] = %q, want %q", i, events[i], e)
		}
	}
}

func TestBulkhead_OnRejectCalled(t *testing.T) {
	t.Parallel()
	var rejectCount int32

	b := NewBulkhead(BulkheadConfig{
		Name:          "reject-cb",
		MaxConcurrent: 1,
		MaxWait:       0,
		OnReject: func(_ string) {
			atomic.AddInt32(&rejectCount, 1)
		},
	})

	started := make(chan struct{})
	release := make(chan struct{})
	go func() {
		b.Execute(context.Background(), func() error {
			close(started)
			<-release
			return nil
		})
	}()
	<-started

	_ = b.Execute(context.Background(), func() error { return nil })
	_ = b.Execute(context.Background(), func() error { return nil })

	close(release)
	time.Sleep(5 * time.Millisecond)

	if atomic.LoadInt32(&rejectCount) != 2 {
		t.Errorf("expected 2 rejections, got %d", rejectCount)
	}
}

func TestBulkhead_PanicReleasesSlot(t *testing.T) {
	b := NewBulkhead(BulkheadConfig{
		Name:          "panic-slot",
		MaxConcurrent: 1,
		MaxWait:       0,
	})

	// Execute with a panic. The bulkhead uses defer for release, so the
	// channel-based semaphore should be freed if fn() panics because the
	// deferred release in Execute runs. But fn() panics before Execute's
	// defer completes normally — let's verify.
	func() {
		defer func() { _ = recover() }()
		_ = b.Execute(context.Background(), func() error { panic("kaboom") })
	}()

	// Slot should be available (defer releases even on panic).
	if b.Available() != 1 {
		t.Errorf("expected 1 available slot after panic, got %d", b.Available())
	}

	// Should be able to execute again.
	err := b.Execute(context.Background(), func() error { return nil })
	if err != nil {
		t.Errorf("expected success after panic recovery, got %v", err)
	}
}

func TestBulkhead_ConcurrentReleaseAcquireRace(t *testing.T) {
	t.Parallel()
	const maxConcurrent = 3
	b := NewBulkhead(BulkheadConfig{
		Name:          "race",
		MaxConcurrent: maxConcurrent,
		MaxWait:       100 * time.Millisecond,
	})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = b.Execute(context.Background(), func() error {
				time.Sleep(time.Millisecond)
				return nil
			})
		}()
	}
	wg.Wait()

	if b.Available() != maxConcurrent {
		t.Errorf("expected %d available after all done, got %d", maxConcurrent, b.Available())
	}
}

func TestBulkhead_AvailableAccuracyDuringExecution(t *testing.T) {
	const maxConcurrent = 5
	b := NewBulkhead(BulkheadConfig{
		Name:          "avail",
		MaxConcurrent: maxConcurrent,
	})

	started := make(chan struct{})
	release := make(chan struct{})

	// Occupy 3 slots.
	for i := 0; i < 3; i++ {
		go func() {
			b.Execute(context.Background(), func() error {
				started <- struct{}{}
				<-release
				return nil
			})
		}()
	}
	for i := 0; i < 3; i++ {
		<-started
	}

	if got := b.Available(); got != 2 {
		t.Errorf("expected 2 available, got %d", got)
	}
	if got := b.InUse(); got != 3 {
		t.Errorf("expected 3 in use, got %d", got)
	}
	if got := b.MaxConcurrent(); got != maxConcurrent {
		t.Errorf("expected MaxConcurrent=%d, got %d", maxConcurrent, got)
	}

	close(release)
	time.Sleep(10 * time.Millisecond)

	if got := b.Available(); got != maxConcurrent {
		t.Errorf("expected %d available after release, got %d", maxConcurrent, got)
	}
}
