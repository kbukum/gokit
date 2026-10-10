package session

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kbukum/gokit/auth/lease"
)

type cleanupFailureStore struct{ *memoryStore }

func (s *cleanupFailureStore) Cleanup(context.Context, time.Time, int) (int64, error) {
	return 0, errors.New("cleanup unavailable")
}

func TestIndependentCleanupCadenceFor1024DueRows(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &memoryStore{rows: make(map[string]Record)}
		manager, domain := fixture(t, store)
		defer manager.Close(context.Background())
		for range 1024 {
			if _, err := manager.Create(context.Background(), caller()); err != nil {
				t.Fatal(err)
			}
		}
		domain.Advance(Lifetime + Retention)
		time.Sleep(40 * time.Second)
		synctest.Wait()
		store.mu.Lock()
		remaining := len(store.rows)
		store.mu.Unlock()
		if remaining != 0 {
			t.Fatalf("independent four-tick cleanup left %d rows", remaining)
		}
	})
}

func TestCleanupFailureIsReportedAndKeepsLiveWatch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &memoryStore{rows: make(map[string]Record)}
		manager, _ := fixture(t, store)
		defer manager.Close(context.Background())
		failing := &cleanupFailureStore{memoryStore: store}
		manager.store = failing
		var reported atomic.Int32
		manager.report = func(context.Context, error) { reported.Add(1) }
		issued, err := manager.Create(context.Background(), caller())
		if err != nil {
			t.Fatal(err)
		}
		lifetime, release, err := manager.Acquire(context.Background(), issued.Principal.Reference)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		time.Sleep(25 * time.Second)
		synctest.Wait()
		if reported.Load() < 2 {
			t.Fatalf("cleanup failures reported %d times; want every failed tick", reported.Load())
		}
		select {
		case <-lifetime.Done():
			t.Fatalf("a cleanup failure ended a watch the store still confirms: %v", context.Cause(lifetime))
		default:
		}
		// When the store stops answering lookups too, the lease still ends the watch.
		store.mu.Lock()
		store.fail = errors.New("store failed")
		store.mu.Unlock()
		time.Sleep(watchLease)
		synctest.Wait()
		endedWith(t, lifetime, lease.ErrExpired)
	})
}
