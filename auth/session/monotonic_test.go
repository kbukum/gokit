package session

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

type stalledStore struct {
	*memoryStore
	mu      sync.Mutex
	stall   bool
	entered chan struct{}
	release chan struct{}
}

func (s *stalledStore) Lookup(ctx context.Context, reference string) (Record, error) {
	s.mu.Lock()
	stall := s.stall
	s.mu.Unlock()
	if stall {
		select {
		case s.entered <- struct{}{}:
		default:
		}
		// Deliberately model a driver that does not honor cancellation.
		<-s.release
	}
	return s.memoryStore.Lookup(ctx, reference)
}

func TestMonotonicLeaseWithFrozenOrBackwardDomainClock(t *testing.T) {
	for _, backward := range []bool{false, true} {
		t.Run(map[bool]string{false: "frozen", true: "backward"}[backward], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				base := &memoryStore{rows: make(map[string]Record)}
				manager, domain := fixture(t, base)
				defer manager.Close(context.Background())
				store := &stalledStore{memoryStore: base, entered: make(chan struct{}, 1), release: make(chan struct{})}
				manager.store = store
				issued, err := manager.Create(context.Background(), caller())
				if err != nil {
					t.Fatal(err)
				}
				lifetime, release, err := manager.Acquire(context.Background(), issued.Principal.Reference)
				if err != nil {
					t.Fatal(err)
				}
				defer release()
				store.mu.Lock()
				store.stall = true
				store.mu.Unlock()
				manager.Nudge()
				defer close(store.release)
				<-store.entered
				if backward {
					domain.Advance(-24 * time.Hour)
				}
				start := time.Now()
				time.Sleep(3 * time.Second)
				synctest.Wait()
				select {
				case <-lifetime.Done():
				default:
					t.Fatal("domain clock defeated monotonic lease")
				}
				if time.Since(start) != 3*time.Second {
					t.Fatal("lease exceeded its read-start bound")
				}
				// The deferred release unblocks both polling owners before manager shutdown.
			})
		})
	}
}
