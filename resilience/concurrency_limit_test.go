package resilience

import (
	"sync"
	"testing"
)

func TestConcurrencyLimitBoundsTotalAndPerKey(t *testing.T) {
	t.Parallel()
	l, err := NewConcurrencyLimit(3, 2)
	if err != nil {
		t.Fatal(err)
	}
	a, ok := l.TryAcquire()
	if !ok || !a.Bind("alice") {
		t.Fatal("first alice slot refused")
	}
	b, _ := l.TryAcquire()
	if !b.Bind("alice") {
		t.Fatal("second alice slot refused")
	}
	c, ok := l.TryAcquire()
	if !ok {
		t.Fatal("third global slot refused")
	}
	if c.Bind("alice") {
		t.Fatal("third alice slot admitted")
	}
	if !c.Bind("bob") {
		t.Fatal("bob refused after a refused bind")
	}
	if _, ok := l.TryAcquire(); ok {
		t.Fatal("fourth global slot admitted")
	}
	if l.InUse() != 3 || l.Held("alice") != 2 || l.Held("bob") != 1 {
		t.Fatalf("in use %d, alice %d, bob %d", l.InUse(), l.Held("alice"), l.Held("bob"))
	}
	a.Release()
	a.Release()
	if l.InUse() != 2 || l.Held("alice") != 1 {
		t.Fatalf("after release: in use %d, alice %d", l.InUse(), l.Held("alice"))
	}
	b.Release()
	c.Release()
	if l.InUse() != 0 || len(l.held) != 0 {
		t.Fatalf("after all releases: in use %d, keys %v", l.InUse(), l.held)
	}
}

func TestConcurrencySlotBindsOnce(t *testing.T) {
	t.Parallel()
	l, _ := NewConcurrencyLimit(2, 2)
	s, _ := l.TryAcquire()
	if !s.Bind("a") || s.Bind("b") || s.Bind("a") {
		t.Fatal("a slot binds only once")
	}
	s.Release()
	if s.Bind("a") {
		t.Fatal("a released slot binds")
	}
	if l.Held("a") != 0 || l.InUse() != 0 {
		t.Fatalf("held %d, in use %d", l.Held("a"), l.InUse())
	}
}

func TestNewConcurrencyLimitRejectsInvalidBounds(t *testing.T) {
	t.Parallel()
	for _, b := range [][2]int{{0, 1}, {1, 0}, {1, 2}, {-1, -1}} {
		if _, err := NewConcurrencyLimit(b[0], b[1]); err == nil {
			t.Fatalf("%v: want error", b)
		}
	}
}

func TestConcurrencyLimitConcurrentUse(t *testing.T) {
	t.Parallel()
	l, _ := NewConcurrencyLimit(8, 3)
	var wg sync.WaitGroup
	for i := range 200 {
		wg.Go(func() {
			if s, ok := l.TryAcquire(); ok {
				s.Bind(string(rune('a' + i%4)))
				s.Release()
			}
		})
	}
	wg.Wait()
	if l.InUse() != 0 || len(l.held) != 0 {
		t.Fatalf("leaked: in use %d, keys %v", l.InUse(), l.held)
	}
}
