package password

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
)

// blockingHasher counts work and blocks it until released.
type blockingHasher struct {
	calls     atomic.Int32
	release   chan struct{}
	verifyErr error
}

func (h *blockingHasher) Hash(string) (string, error) {
	h.calls.Add(1)
	<-h.release
	return "hash", nil
}

func (h *blockingHasher) Verify(string, string) error {
	h.calls.Add(1)
	<-h.release
	return h.verifyErr
}

func TestNewPoolValidatesBounds(t *testing.T) {
	t.Parallel()
	h := &blockingHasher{}
	var typedNil *blockingHasher
	for name, tc := range map[string]struct {
		hasher Hasher
		cfg    PoolConfig
	}{
		"hasher":      {nil, PoolConfig{Workers: 1}},
		"typed nil":   {typedNil, PoolConfig{Workers: 1}},
		"workers":     {h, PoolConfig{}},
		"workers up":  {h, PoolConfig{Workers: MaxPoolWorkers + 1}},
		"queue":       {h, PoolConfig{Workers: 1, Queue: -1}},
		"queue up":    {h, PoolConfig{Workers: 1, Queue: MaxPoolQueue + 1}},
		"wait needed": {h, PoolConfig{Workers: 1, Queue: 1}},
		"wait up":     {h, PoolConfig{Workers: 1, Queue: 1, MaxWait: MaxPoolWait + 1}},
		"wait unused": {h, PoolConfig{Workers: 1, MaxWait: time.Second}},
	} {
		if _, err := NewPool(tc.hasher, tc.cfg); apperrors.Normalize(err).Code != apperrors.ErrCodeInvalidInput {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestPoolSaturationRefusesBeforeWork(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := &blockingHasher{release: make(chan struct{})}
		pool, err := NewPool(h, PoolConfig{Workers: 2, Queue: 1, MaxWait: time.Second})
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		for range 3 {
			wg.Go(func() {
				if err := pool.Verify(context.Background(), "password", "hash"); err != nil {
					t.Errorf("admitted Verify: %v", err)
				}
			})
		}
		synctest.Wait()
		if h.calls.Load() != 2 {
			t.Fatalf("running = %d, want 2", h.calls.Load())
		}
		requireError(t, pool.Verify(context.Background(), "password", "hash"), apperrors.ErrCodeRateLimited, ReasonBusy)
		_, err = pool.Hash(context.Background(), "password")
		requireError(t, err, apperrors.ErrCodeRateLimited, ReasonBusy)
		if h.calls.Load() != 2 {
			t.Fatal("rejected call ran hashing work")
		}
		close(h.release)
		wg.Wait()
		if h.calls.Load() != 3 {
			t.Fatalf("queued call did not run: %d", h.calls.Load())
		}
	})
}

func TestPoolQueueTimeoutAndCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := &blockingHasher{release: make(chan struct{})}
		pool, err := NewPool(h, PoolConfig{Workers: 1, Queue: 1, MaxWait: time.Second})
		if err != nil {
			t.Fatal(err)
		}
		go func() { _ = pool.Verify(context.Background(), "password", "hash") }()
		synctest.Wait()
		requireError(t, pool.Verify(context.Background(), "password", "hash"), apperrors.ErrCodeRateLimited, ReasonBusy)
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- pool.Verify(ctx, "password", "hash") }()
		synctest.Wait()
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled waiter: %v", err)
		}
		if h.calls.Load() != 1 {
			t.Fatalf("calls = %d, want 1", h.calls.Load())
		}
		close(h.release)
	})
}

func TestPoolRunsRealHasherAndSkipsCanceledWork(t *testing.T) {
	t.Parallel()
	pool, err := NewPool(newHasher(t, Config{MinLength: 4}), PoolConfig{Workers: 1})
	if err != nil {
		t.Fatal(err)
	}

	hash, err := pool.Hash(context.Background(), "password")
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.Verify(context.Background(), "password", hash); err != nil {
		t.Fatal(err)
	}
	requireError(t, pool.Verify(context.Background(), "wrong", hash), apperrors.ErrCodeUnauthorized, ReasonMismatch)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := pool.Verify(ctx, "password", hash); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Verify: %v", err)
	}
}

func TestPoolCanceledRunningWorkCannotReturnSuccess(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := &blockingHasher{release: make(chan struct{})}
		pool, err := NewPool(h, PoolConfig{Workers: 1})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- pool.Verify(ctx, "password", "hash") }()
		synctest.Wait()
		cancel()
		close(h.release)
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled running KDF returned %v", err)
		}
	})
}

func TestPoolCanceledFailingWorkIsNotCredentialDenial(t *testing.T) {
	for name, tc := range map[string]struct {
		end  func(context.Context) (context.Context, context.CancelFunc)
		code apperrors.ErrorCode
	}{
		"canceled": {context.WithCancel, apperrors.ErrCodeCanceled},
		"deadline": {func(ctx context.Context) (context.Context, context.CancelFunc) {
			return context.WithTimeout(ctx, time.Second)
		}, apperrors.ErrCodeTimeout},
	} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := &blockingHasher{release: make(chan struct{}), verifyErr: mismatch()}
				pool, err := NewPool(h, PoolConfig{Workers: 1})
				if err != nil {
					t.Fatal(err)
				}
				ctx, cancel := tc.end(context.Background())
				done := make(chan error, 1)
				go func() { done <- pool.Verify(ctx, "password", "hash") }()
				defer cancel()
				synctest.Wait()
				if tc.code == apperrors.ErrCodeCanceled {
					cancel()
				}
				time.Sleep(2 * time.Second)
				close(h.release)
				err = <-done
				if got := apperrors.Normalize(err); got.Code != tc.code || got.Reason == ReasonMismatch {
					t.Fatalf("abandoned failing KDF classified as %v (%v)", got.Code, err)
				}
			})
		})
	}
}

func TestPoolCloseRetainsOwnershipAfterTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := &blockingHasher{release: make(chan struct{})}
		pool, err := NewPool(h, PoolConfig{Workers: 1, Queue: 1, MaxWait: time.Minute / 2})
		if err != nil {
			t.Fatal(err)
		}
		running, waiting := make(chan error, 1), make(chan error, 1)
		go func() { running <- pool.Verify(context.Background(), "password", "hash") }()
		synctest.Wait()
		go func() { waiting <- pool.Verify(context.Background(), "password", "hash") }()
		synctest.Wait()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := pool.Close(ctx); !errors.Is(err, context.DeadlineExceeded) || apperrors.Normalize(err).Code != apperrors.ErrCodeTimeout {
			t.Fatalf("close lost running work or misclassified its deadline: %v", err)
		}
		requireError(t, <-waiting, apperrors.ErrCodeServiceUnavailable, ReasonClosed)
		requireError(t, pool.Verify(context.Background(), "password", "hash"), apperrors.ErrCodeServiceUnavailable, ReasonClosed)
		if h.calls.Load() != 1 {
			t.Fatal("queued work started after close")
		}
		close(h.release)
		if err := <-running; err != nil {
			t.Fatal(err)
		}
		if err := pool.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := pool.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
}
