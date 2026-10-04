package session

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
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

func awaitCanceled(t *testing.T, ctx context.Context) {
	t.Helper()
	deadline, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	select {
	case <-ctx.Done():
	case <-deadline.Done():
		t.Fatal("lifetime not canceled within bound")
	}
}

func TestLeaseIndependentFromBlockedRead(t *testing.T) {
	base := &memoryStore{rows: make(map[string]Record)}
	m, clock := fixture(t, base)
	block := &blockingStore{memoryStore: base, entered: make(chan struct{}, 1), continueRead: make(chan struct{})}
	m.store = block
	issued, err := m.Create(context.Background(), caller())
	if err != nil {
		t.Fatal(err)
	}
	life, release, err := m.Acquire(context.Background(), issued.Principal.Reference)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	block.mu.Lock()
	block.block = true
	block.mu.Unlock()
	pollDone := make(chan struct{})
	go func() { m.Poll(context.Background()); close(pollDone) }()
	select {
	case <-block.entered:
	case <-time.After(time.Second):
		t.Fatal("read did not start")
	}
	clock.Advance(3 * time.Second)
	awaitCanceled(t, life)
	close(block.continueRead)
	select {
	case <-pollDone:
	case <-time.After(time.Second):
		t.Fatal("poll leaked")
	}
	// A late successful read cannot resurrect a released lease.
	m.mu.Lock()
	n := len(m.watches)
	m.mu.Unlock()
	if n != 0 {
		t.Fatal("expired watch resurrected")
	}
}

func TestWatchStoreFailureExpiryAndClose(t *testing.T) {
	s := &memoryStore{rows: make(map[string]Record)}
	m, clock := fixture(t, s)
	issued, err := m.Create(context.Background(), caller())
	if err != nil {
		t.Fatal(err)
	}
	life, release, err := m.Acquire(context.Background(), issued.Principal.Reference)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	s.mu.Lock()
	s.fail = errors.New("store failed")
	s.mu.Unlock()
	m.Poll(context.Background())
	awaitCanceled(t, life)
	s.mu.Lock()
	s.fail = nil
	s.mu.Unlock()
	life, release, err = m.Acquire(context.Background(), issued.Principal.Reference)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	clock.Advance(time.Hour)
	m.Poll(context.Background())
	awaitCanceled(t, life)
	issued, err = m.Create(context.Background(), caller())
	if err != nil {
		t.Fatal(err)
	}
	life, release, err = m.Acquire(context.Background(), issued.Principal.Reference)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := m.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	awaitCanceled(t, life)
	if err := m.Close(context.Background()); err != nil {
		t.Fatal("non-idempotent close")
	}
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
