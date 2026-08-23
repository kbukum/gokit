package sse

import (
	"testing"
	"time"
)

func drain[T any](sub *Subscription[T]) []Event[T] {
	var out []Event[T]
	for {
		select {
		case e, ok := <-sub.Events():
			if !ok {
				return out
			}
			out = append(out, e)
		default:
			return out
		}
	}
}

func TestNewBus_CapacityValidation(t *testing.T) {
	t.Parallel()
	if _, err := NewBus[int](0); err == nil {
		t.Error("expected error for zero capacity")
	}
	if _, err := NewBus[int](-1); err == nil {
		t.Error("expected error for negative capacity")
	}
	if _, err := NewBus[int](MaxBusCapacity + 1); err == nil {
		t.Error("expected error for capacity above maximum")
	}
	if _, err := NewBus[int](4); err != nil {
		t.Errorf("valid capacity: %v", err)
	}
}

func TestBus_PublishAssignsMonotonicIDs(t *testing.T) {
	t.Parallel()
	bus, _ := NewBus[string](8)
	e1 := bus.Publish("a")
	e2 := bus.Publish("b")
	e3 := bus.PublishNamed("tick", "c")
	if e1.ID != "1" || e2.ID != "2" || e3.ID != "3" {
		t.Fatalf("ids = %s,%s,%s want 1,2,3", e1.ID, e2.ID, e3.ID)
	}
	if e3.Name != "tick" {
		t.Errorf("name = %q, want tick", e3.Name)
	}
}

func TestBus_WithRetryAttached(t *testing.T) {
	t.Parallel()
	bus, _ := NewBus[int](4, WithRetry(2*time.Second))
	e := bus.Publish(1)
	if e.Retry != 2*time.Second {
		t.Errorf("retry = %v, want 2s", e.Retry)
	}
}

func TestBus_Subscribe_LiveOnly(t *testing.T) {
	t.Parallel()
	bus, _ := NewBus[string](4)
	bus.Publish("before") // must NOT be replayed to a live subscriber
	sub := bus.Subscribe()
	defer sub.Close()
	bus.Publish("after")

	got := drain(sub)
	if len(got) != 1 || got[0].Data != "after" {
		t.Fatalf("live sub events = %v, want only 'after'", got)
	}
}

func TestBus_SubscribeAfter_FullReplay(t *testing.T) {
	t.Parallel()
	bus, _ := NewBus[string](4)
	bus.Publish("a")
	bus.Publish("b")

	sub := bus.SubscribeAfter("") // empty => full buffered replay
	defer sub.Close()
	bus.Publish("c")

	got := drain(sub)
	if len(got) != 3 {
		t.Fatalf("got %d events, want 3 (a,b replay + c live)", len(got))
	}
	if got[0].Data != "a" || got[1].Data != "b" || got[2].Data != "c" {
		t.Fatalf("order = %v, want a,b,c", got)
	}
}

func TestBus_SubscribeAfter_ResumesAfterID(t *testing.T) {
	t.Parallel()
	bus, _ := NewBus[string](8)
	bus.Publish("a") // id 1
	bus.Publish("b") // id 2
	bus.Publish("c") // id 3

	sub := bus.SubscribeAfter("2") // resume after id 2 => only c
	defer sub.Close()

	got := drain(sub)
	if len(got) != 1 || got[0].Data != "c" || got[0].ID != "3" {
		t.Fatalf("resume events = %v, want only c(id 3)", got)
	}
}

func TestBus_ReplayBufferBounded(t *testing.T) {
	t.Parallel()
	bus, _ := NewBus[int](3)
	for i := 1; i <= 5; i++ {
		bus.Publish(i) // ids 1..5, buffer holds last 3 (ids 3,4,5)
	}
	sub := bus.SubscribeAfter("")
	defer sub.Close()

	got := drain(sub)
	if len(got) != 3 {
		t.Fatalf("replayed %d events, want 3 (bounded)", len(got))
	}
	if got[0].Data != 3 || got[1].Data != 4 || got[2].Data != 5 {
		t.Fatalf("bounded replay = %v, want [3 4 5]", []int{got[0].Data, got[1].Data, got[2].Data})
	}
}

func TestBus_SubscriberCountAndClose(t *testing.T) {
	t.Parallel()
	bus, _ := NewBus[int](4)
	s1 := bus.Subscribe()
	s2 := bus.Subscribe()
	if n := bus.SubscriberCount(); n != 2 {
		t.Fatalf("subscriber count = %d, want 2", n)
	}
	s1.Close()
	if n := bus.SubscriberCount(); n != 1 {
		t.Fatalf("after close count = %d, want 1", n)
	}
	// Draining a closed subscription's channel yields closed.
	if _, ok := <-s1.Events(); ok {
		t.Error("closed subscription channel should be closed")
	}

	bus.Close()
	if n := bus.SubscriberCount(); n != 0 {
		t.Fatalf("after bus close count = %d, want 0", n)
	}
	if _, ok := <-s2.Events(); ok {
		t.Error("bus close should close subscriber channels")
	}
}

func TestBus_SlowSubscriberDropsWhenFull(t *testing.T) {
	t.Parallel()
	bus, _ := NewBus[int](2)
	sub := bus.Subscribe()
	defer sub.Close()

	// Publish more than the bounded queue can hold without draining.
	for i := 0; i < 10; i++ {
		bus.Publish(i)
	}
	got := drain(sub)
	if len(got) > 2 {
		t.Fatalf("received %d events, want <= capacity 2 (bounded drop)", len(got))
	}
}

func TestBus_CloseIsIdempotent(t *testing.T) {
	t.Parallel()
	bus, _ := NewBus[int](2)
	sub := bus.Subscribe()
	bus.Close()
	bus.Close() // must not panic
	sub.Close() // must not panic (channel already closed by bus)
}
