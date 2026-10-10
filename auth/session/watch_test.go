package session

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kbukum/gokit/auth/lease"
)

type blockingStore struct {
	*memoryStore
	mu           sync.Mutex
	block        bool
	entered      chan struct{}
	continueRead chan struct{}
}

func (s *blockingStore) Lookup(ctx context.Context, ref string) (Record, error) {
	s.mu.Lock()
	block := s.block
	s.mu.Unlock()
	if block {
		select {
		case s.entered <- struct{}{}:
		default:
		}
		select {
		case <-ctx.Done():
			return Record{}, ctx.Err()
		case <-s.continueRead:
		}
	}
	return s.memoryStore.Lookup(ctx, ref)
}

func endedWith(t *testing.T, life context.Context, want error) {
	t.Helper()
	synctest.Wait()
	if life.Err() == nil {
		t.Fatal("lifetime still alive")
	}
	if got := context.Cause(life); !errors.Is(got, want) {
		t.Fatalf("cause = %v, want %v", got, want)
	}
}

func TestLeaseEndsWhileReadBlocks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		base := &memoryStore{rows: make(map[string]Record)}
		m, _ := fixture(t, base)
		defer m.Close(context.Background())
		block := &blockingStore{memoryStore: base, entered: make(chan struct{}, 1), continueRead: make(chan struct{})}
		m.store = block
		issued, err := m.Create(context.Background(), caller())
		if err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		life, release, err := m.Acquire(context.Background(), issued.Principal.Reference)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		block.mu.Lock()
		block.block = true
		block.mu.Unlock()
		time.Sleep(3 * time.Second)
		endedWith(t, life, lease.ErrExpired)
		if time.Since(start) != 3*time.Second {
			t.Fatal("lease outlived its admitting read")
		}
		close(block.continueRead)
		m.Nudge()
		synctest.Wait()
		if m.leases.Len() != 0 {
			t.Fatal("expired watch resurrected")
		}
	})
}

func TestWatchStoreFailureExpiryAndClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := &memoryStore{rows: make(map[string]Record)}
		m, clock := fixture(t, s)
		defer m.Close(context.Background())
		issued, err := m.Create(context.Background(), caller())
		if err != nil {
			t.Fatal(err)
		}
		life, release, err := m.Acquire(context.Background(), issued.Principal.Reference)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		// An unavailable store renews nothing; the stream ends when its lease runs out, not on the first failed read.
		s.mu.Lock()
		s.fail = errors.New("store failed")
		s.mu.Unlock()
		time.Sleep(3 * time.Second)
		endedWith(t, life, lease.ErrExpired)
		s.mu.Lock()
		s.fail = nil
		s.mu.Unlock()

		life, release, err = m.Acquire(context.Background(), issued.Principal.Reference)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		clock.Advance(time.Hour)
		m.Nudge()
		synctest.Wait()
		endedWith(t, life, lease.ErrRevoked)
		clock.Advance(-time.Hour)

		life, release, err = m.Acquire(context.Background(), issued.Principal.Reference)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		if err := m.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
		endedWith(t, life, lease.ErrClosed)
		if err := m.Close(context.Background()); err != nil {
			t.Fatal("non-idempotent close")
		}
	})
}

func TestAdmittedStreamEndsAtSessionExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := &memoryStore{rows: make(map[string]Record)}
		m, clock := fixture(t, s)
		defer m.Close(context.Background())
		issued, err := m.Create(context.Background(), caller())
		if err != nil {
			t.Fatal(err)
		}
		clock.Advance(Lifetime - 10*time.Second)
		life, release, err := m.Acquire(context.Background(), issued.Principal.Reference)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		// Freeze the domain clock: the monotonic lifetime alone ends the stream at the session's expiry.
		time.Sleep(10*time.Second - time.Nanosecond)
		synctest.Wait()
		if life.Err() != nil {
			t.Fatalf("ended early: %v", context.Cause(life))
		}
		time.Sleep(time.Nanosecond)
		endedWith(t, life, lease.ErrEnded)
	})
}

func TestCleanupBoundAndRetention(t *testing.T) {
	s := &memoryStore{rows: make(map[string]Record)}
	m, clock := fixture(t, s)
	for range 1024 {
		if _, err := m.Create(context.Background(), caller()); err != nil {
			t.Fatal(err)
		}
	}
	clock.Advance(time.Hour + Retention - time.Nanosecond)
	n, err := m.Cleanup(context.Background())
	if err != nil || n != 0 {
		t.Fatal("early deletion", n, err)
	}
	clock.Advance(time.Nanosecond)
	for range 4 {
		n, err = m.Cleanup(context.Background())
		if err != nil || n > 256 {
			t.Fatal("unbounded cleanup", n, err)
		}
	}
	s.mu.Lock()
	remaining := len(s.rows)
	s.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("cleanup remaining=%d", remaining)
	}
}
