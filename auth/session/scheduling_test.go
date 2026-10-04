package session

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
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

func TestCleanupFailureCancelsIndependentWatch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &memoryStore{rows: make(map[string]Record)}
		manager, _ := fixture(t, store)
		defer manager.Close(context.Background())
		manager.store = &cleanupFailureStore{memoryStore: store}
		issued, err := manager.Create(context.Background(), caller())
		if err != nil {
			t.Fatal(err)
		}
		lifetime, release, err := manager.Acquire(context.Background(), issued.Principal.Reference)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		time.Sleep(10 * time.Second)
		synctest.Wait()
		select {
		case <-lifetime.Done():
		default:
			t.Fatal("cleanup failure retained watch")
		}
	})
}
