package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/kbukum/gokit/messaging"
)

const defaultBufferSize = 256

// InMemoryBroker is a testing broker that routes messages through channels.
type InMemoryBroker struct {
	mu      sync.RWMutex
	topics  map[string][]chan messaging.Message
	history map[string][]messaging.Message
	bufSize int
	closed  bool

	// msgCh is signaled (non-blocking) after every publish
	// so that WaitForMessage can wake up without polling.
	msgCh   chan struct{}
	closeCh chan struct{}
}

// NewBroker creates a new in-memory broker with the default buffer size.
func NewBroker() *InMemoryBroker {
	return &InMemoryBroker{
		topics:  make(map[string][]chan messaging.Message),
		history: make(map[string][]messaging.Message),
		bufSize: defaultBufferSize,
		msgCh:   make(chan struct{}, 1),
		closeCh: make(chan struct{}),
	}
}

// NewBrokerWithBuffer creates a new in-memory broker with a custom buffer size.
func NewBrokerWithBuffer(size int) *InMemoryBroker {
	return &InMemoryBroker{
		topics:  make(map[string][]chan messaging.Message),
		history: make(map[string][]messaging.Message),
		bufSize: size,
		msgCh:   make(chan struct{}, 1),
		closeCh: make(chan struct{}),
	}
}

// Messages returns all messages published to the given topic.
func (b *InMemoryBroker) Messages(topic string) []messaging.Message {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]messaging.Message, len(b.history[topic]))
	copy(out, b.history[topic])
	return out
}

// AllMessages returns all published messages across every topic,
// ordered by topic name then by publish order.
func (b *InMemoryBroker) AllMessages() []messaging.Message {
	b.mu.RLock()
	defer b.mu.RUnlock()

	topics := make([]string, 0, len(b.history))
	for t := range b.history {
		topics = append(topics, t)
	}
	sort.Strings(topics)

	var total int
	for _, t := range topics {
		total += len(b.history[t])
	}
	out := make([]messaging.Message, 0, total)
	for _, t := range topics {
		out = append(out, b.history[t]...)
	}
	return out
}

// MessageCount returns the number of messages published to the given topic.
func (b *InMemoryBroker) MessageCount(topic string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.history[topic])
}

// Reset clears all recorded message history (channels and subscriptions are not affected).
func (b *InMemoryBroker) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.history = make(map[string][]messaging.Message)
}

// CreateTopic pre-creates a topic so that it appears in [Topics] even before any subscriber
// or publisher uses it.
func (b *InMemoryBroker) CreateTopic(topic string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.topics[topic]; !ok {
		b.topics[topic] = nil
	}
	if _, ok := b.history[topic]; !ok {
		b.history[topic] = nil
	}
}

// Topics returns the sorted list of topics that have been created, subscribed to, or published to.
func (b *InMemoryBroker) Topics() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	seen := make(map[string]struct{})
	for t := range b.topics {
		seen[t] = struct{}{}
	}
	for t := range b.history {
		seen[t] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

func (b *InMemoryBroker) subscribe(topic string) chan messaging.Message {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan messaging.Message, b.bufSize)
	b.topics[topic] = append(b.topics[topic], ch)
	return ch
}

func (b *InMemoryBroker) publish(topic string, msg messaging.Message) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return fmt.Errorf("broker: %w", messaging.ErrClosed)
	}
	msg.Topic = topic

	// Record into history before fan-out.
	b.history[topic] = append(b.history[topic], msg)

	for _, ch := range b.topics[topic] {
		select {
		case ch <- msg:
		default:
			return fmt.Errorf("topic %q buffer full", topic)
		}
	}

	// Signal any WaitForMessage callers.
	select {
	case b.msgCh <- struct{}{}:
	default:
	}

	return nil
}

func (b *InMemoryBroker) requeue(ctx context.Context, topic string, msg messaging.Message) error {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return fmt.Errorf("broker: %w", messaging.ErrClosed)
	}
	subs := append([]chan messaging.Message(nil), b.topics[topic]...)
	closeCh := b.closeCh
	b.mu.RUnlock()

	for _, ch := range subs {
		select {
		case ch <- msg:
		case <-ctx.Done():
			return ctx.Err()
		case <-closeCh:
			return fmt.Errorf("broker: %w", messaging.ErrClosed)
		}
	}
	return nil
}

// Producer creates a new in-memory producer backed by this broker.
func (b *InMemoryBroker) Producer() *InMemoryProducer {
	return &InMemoryProducer{broker: b}
}

// Consumer creates a new in-memory consumer for the given topic.
func (b *InMemoryBroker) Consumer(topic string) *InMemoryConsumer {
	return b.consumer(topic, messaging.CommitAuto)
}

func (b *InMemoryBroker) consumer(topic string, commit messaging.CommitStrategy) *InMemoryConsumer {
	ch := b.subscribe(topic)
	return &InMemoryConsumer{broker: b, topic: topic, ch: ch, commitStrategy: commit}
}

// Close marks the broker as closed.
func (b *InMemoryBroker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	close(b.closeCh)
	// Don't close individual subscriber channels; closeCh signals no more messages will come.
	// Subscribers detect closure via closeCh select case.
	// This avoids a race where requeue might be sending on a channel as Close closes it.
	b.topics = make(map[string][]chan messaging.Message)
}
