package sse

import (
	"strconv"
	"sync"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
)

// MaxBusCapacity bounds the replay buffer and per-subscriber queue of a Bus.
const MaxBusCapacity = 1 << 24

// Event is a toolkit-native SSE event carrying replay metadata. The bus assigns
// each event a monotonic ID used for Last-Event-ID resume.
type Event[T any] struct {
	// ID is the monotonic event identifier assigned by the bus.
	ID string
	// Name is the optional SSE event type.
	Name string
	// Retry is the optional client reconnection interval.
	Retry time.Duration
	// Data is the event payload.
	Data T
}

// Bus is a bounded, typed, multi-subscriber Server-Sent Events bus.
//
// Live fan-out and the replay buffer are both bounded by the configured
// capacity. A slow subscriber never blocks the bus: once its queue is full,
// newer live events are dropped for that subscriber (best-effort delivery). The
// bounded replay buffer stores the most recent events so a reconnecting client
// can resume after its Last-Event-ID.
type Bus[T any] struct {
	mu        sync.Mutex
	capacity  int
	retry     time.Duration
	nextID    uint64
	replay    []Event[T]
	subs      map[int]*Subscription[T]
	nextSubID int
	closed    bool
}

// BusOption configures a Bus.
type BusOption func(*busOptions)

type busOptions struct {
	retry time.Duration
}

// WithRetry sets the reconnection interval attached to published events.
func WithRetry(retry time.Duration) BusOption {
	return func(o *busOptions) { o.retry = retry }
}

// NewBus creates a Bus with the given bounded capacity. Capacity must be greater
// than zero and at most MaxBusCapacity.
func NewBus[T any](capacity int, opts ...BusOption) (*Bus[T], error) {
	if capacity <= 0 {
		return nil, apperrors.InvalidInput("capacity", "SSE bus capacity must be greater than zero")
	}
	if capacity > MaxBusCapacity {
		return nil, apperrors.InvalidInput("capacity", "SSE bus capacity must be at most "+strconv.Itoa(MaxBusCapacity))
	}
	var o busOptions
	for _, opt := range opts {
		opt(&o)
	}
	return &Bus[T]{
		capacity: capacity,
		retry:    o.retry,
		nextID:   1,
		replay:   make([]Event[T], 0, capacity),
		subs:     make(map[int]*Subscription[T]),
	}, nil
}

// Publish appends an event to the replay buffer and fans it out to all live
// subscribers, returning the published event. Publishing without subscribers is
// successful; the event remains available for bounded replay until evicted.
func (b *Bus[T]) Publish(data T) Event[T] {
	return b.publish("", data)
}

// PublishNamed publishes an event with an explicit SSE event type.
func (b *Bus[T]) PublishNamed(name string, data T) Event[T] {
	return b.publish(name, data)
}

func (b *Bus[T]) publish(name string, data T) Event[T] {
	b.mu.Lock()
	defer b.mu.Unlock()

	event := Event[T]{
		ID:    strconv.FormatUint(b.nextID, 10),
		Name:  name,
		Retry: b.retry,
		Data:  data,
	}
	b.nextID++

	if b.closed {
		return event
	}

	b.pushReplay(event)
	for _, sub := range b.subs {
		sub.deliver(event)
	}
	return event
}

// pushReplay appends to the bounded replay buffer, evicting the oldest event
// when the buffer is full. Callers must hold b.mu.
func (b *Bus[T]) pushReplay(event Event[T]) {
	if len(b.replay) == b.capacity {
		copy(b.replay, b.replay[1:])
		b.replay[len(b.replay)-1] = event
		return
	}
	b.replay = append(b.replay, event)
}

// Subscribe returns a live subscription that receives events published after
// the call. It does not replay buffered events.
func (b *Bus[T]) Subscribe() *Subscription[T] {
	return b.subscribe(nil, false)
}

// SubscribeAfter returns a subscription that first replays buffered events with
// an ID greater than lastEventID, then delivers live events. An empty or
// unparseable lastEventID replays the entire buffer before going live.
func (b *Bus[T]) SubscribeAfter(lastEventID string) *Subscription[T] {
	last, ok := parseEventID(lastEventID)
	return b.subscribe(&last, ok)
}

func (b *Bus[T]) subscribe(after *uint64, hasBound bool) *Subscription[T] {
	b.mu.Lock()
	defer b.mu.Unlock()

	sub := &Subscription[T]{
		bus: b,
		ch:  make(chan Event[T], b.capacity),
	}

	if b.closed {
		close(sub.ch)
		return sub
	}

	if after != nil {
		for _, event := range b.replay {
			if hasBound {
				if id, err := strconv.ParseUint(event.ID, 10, 64); err != nil || id <= *after {
					continue
				}
			}
			sub.deliver(event)
		}
	}

	sub.id = b.nextSubID
	b.nextSubID++
	b.subs[sub.id] = sub
	return sub
}

// SubscriberCount returns the number of active subscribers.
func (b *Bus[T]) SubscriberCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}

// Close removes all subscribers and closes their channels. Subsequent Subscribe
// calls return an already-closed subscription and Publish becomes a no-op fan-out.
func (b *Bus[T]) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for id, sub := range b.subs {
		close(sub.ch)
		delete(b.subs, id)
	}
}

func (b *Bus[T]) removeSub(id int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if sub, ok := b.subs[id]; ok {
		delete(b.subs, id)
		close(sub.ch)
	}
}

func parseEventID(lastEventID string) (uint64, bool) {
	if lastEventID == "" {
		return 0, false
	}
	id, err := strconv.ParseUint(lastEventID, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

// Subscription is a single subscriber's stream of events from a Bus.
type Subscription[T any] struct {
	bus       *Bus[T]
	id        int
	ch        chan Event[T]
	closeOnce sync.Once
}

// Events returns the receive-only channel of events. The channel is closed when
// the subscription is closed or the bus is closed.
func (s *Subscription[T]) Events() <-chan Event[T] { return s.ch }

// Close unsubscribes from the bus and closes the event channel. It is safe to
// call multiple times.
func (s *Subscription[T]) Close() {
	s.closeOnce.Do(func() {
		s.bus.removeSub(s.id)
	})
}

// deliver performs a non-blocking send; a full queue drops the event to keep the
// bus bounded and non-blocking. Callers must hold the bus lock.
func (s *Subscription[T]) deliver(event Event[T]) {
	select {
	case s.ch <- event:
	default:
	}
}
