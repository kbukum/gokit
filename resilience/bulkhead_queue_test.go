package resilience

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

func TestBulkhead_MaxQueueRejectsBeyondWaiters(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := newBulkhead(t, BulkheadConfig{Name: "q", MaxConcurrent: 1, MaxWait: time.Minute, MaxQueue: 2})
		hold := make(chan struct{})
		var wg sync.WaitGroup
		errs := make(chan error, 3)
		for range 3 {
			wg.Go(func() { errs <- b.Execute(context.Background(), func() error { <-hold; return nil }) })
		}
		synctest.Wait()
		if b.InUse() != 1 || b.Waiting() != 2 {
			t.Fatalf("in use %d, waiting %d", b.InUse(), b.Waiting())
		}
		ran := false
		if err := b.Execute(context.Background(), func() error { ran = true; return nil }); !errors.Is(err, ErrBulkheadFull) || ran {
			t.Fatalf("fourth caller: err %v, ran %v", err, ran)
		}
		close(hold)
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("admitted caller: %v", err)
			}
		}
		if b.Waiting() != 0 {
			t.Fatalf("waiting = %d after drain", b.Waiting())
		}
	})
}

func TestBulkhead_WaitersLeaveQueueOnTimeoutAndCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := newBulkhead(t, BulkheadConfig{MaxConcurrent: 1, MaxWait: time.Second, MaxQueue: 1})
		hold := make(chan struct{})
		go func() { _ = b.Execute(context.Background(), func() error { <-hold; return nil }) }()
		synctest.Wait()
		if err := b.Execute(context.Background(), func() error { return nil }); !errors.Is(err, ErrBulkheadTimeout) {
			t.Fatalf("timed-out waiter: %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- b.Execute(ctx, func() error { return nil }) }()
		synctest.Wait()
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled waiter: %v", err)
		}
		if b.Waiting() != 0 {
			t.Fatalf("waiting = %d", b.Waiting())
		}
		close(hold)
	})
}

func TestBulkheadRejectsInvalidQueue(t *testing.T) {
	for _, cfg := range []BulkheadConfig{
		{MaxConcurrent: 1, MaxQueue: -1},
		{MaxConcurrent: 1, MaxWait: -1},
		{MaxConcurrent: 1, MaxQueue: 1},
	} {
		if b, err := NewBulkhead(cfg); err == nil || b != nil {
			t.Fatalf("invalid queue accepted: %+v", cfg)
		}
	}
}

func TestBulkheadZeroQueueNeverWaits(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := newBulkhead(t, BulkheadConfig{MaxConcurrent: 1, MaxWait: time.Second})
		hold := make(chan struct{})
		go func() { _ = b.Execute(context.Background(), func() error { <-hold; return nil }) }()
		synctest.Wait()
		start := time.Now()
		err := b.Execute(context.Background(), func() error { return nil })
		close(hold)
		if !errors.Is(err, ErrBulkheadFull) || time.Since(start) != 0 || b.Waiting() != 0 {
			t.Fatalf("zero queue waited: error=%v delay=%s waiting=%d", err, time.Since(start), b.Waiting())
		}
	})
}
