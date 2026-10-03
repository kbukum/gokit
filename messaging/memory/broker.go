package memory

import (
	"fmt"
	"sort"
	"sync"

	"github.com/kbukum/gokit/messaging"
)

const defaultBufferSize = 256

// InMemoryBroker routes live messages through bounded subscriber queues without retaining publication history. Full recording is available through messaging/testutil.MockProducer.
type InMemoryBroker struct {
	mu      sync.RWMutex
	topics  map[string][]*InMemoryConsumer
	bufSize int
	closed  bool

	closeCh chan struct{}
}

// NewBroker creates a new in-memory broker with the default buffer size.
func NewBroker() *InMemoryBroker {
	return &InMemoryBroker{
		topics:  make(map[string][]*InMemoryConsumer),
		bufSize: defaultBufferSize,
		closeCh: make(chan struct{}),
	}
}

// NewBrokerWithBuffer creates a new in-memory broker with a custom buffer size.
func NewBrokerWithBuffer(size int) *InMemoryBroker {
	return &InMemoryBroker{
		topics:  make(map[string][]*InMemoryConsumer),
		bufSize: size,
		closeCh: make(chan struct{}),
	}
}

// CreateTopic pre-creates a topic so that it appears in [Topics] even before any subscriber
// or publisher uses it.
func (b *InMemoryBroker) CreateTopic(topic string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.topics[topic]; !ok {
		b.topics[topic] = nil
	}
}

// Topics returns explicitly created topics and topics with active subscribers. Publication alone never retains a topic.
func (b *InMemoryBroker) Topics() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	seen := make(map[string]struct{})
	for t := range b.topics {
		seen[t] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

func (b *InMemoryBroker) publish(topic string, msg messaging.Message) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return fmt.Errorf("broker: %w", messaging.ErrClosed)
	}
	msg.Topic = topic

	for _, subscriber := range b.topics[topic] {
		select {
		case subscriber.ch <- msg:
		default:
			return fmt.Errorf("topic %q buffer full", topic)
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
	b.mu.Lock()
	defer b.mu.Unlock()
	consumer := &InMemoryConsumer{broker: b, topic: topic, ch: make(chan messaging.Message, b.bufSize), commitStrategy: commit, done: make(chan struct{})}
	if b.closed {
		close(consumer.done)
		consumer.closed = true
		return consumer
	}
	b.topics[topic] = append(b.topics[topic], consumer)
	return consumer
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
	for _, subscribers := range b.topics {
		for _, subscriber := range subscribers {
			subscriber.pending = nil
		}
	}
	// Don't close individual subscriber channels; closeCh signals no more messages will come.
	// Subscribers detect closure via closeCh select case.
	// This avoids a race where requeue might be sending on a channel as Close closes it.
	b.topics = make(map[string][]*InMemoryConsumer)
}
