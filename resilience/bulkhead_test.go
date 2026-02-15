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

	if acquired != 1 {
		t.Errorf("expected 1 acquire callback, got %d", acquired)
	}
	if released != 1 {
		t.Errorf("expected 1 release callback, got %d", released)
	}
	if rejected != 1 {
		t.Errorf("expected 1 reject callback, got %d", rejected)
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

	result, err := ExecuteWithResult(b, context.Background(), func() (int, error) {
		return 42, nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}
}
