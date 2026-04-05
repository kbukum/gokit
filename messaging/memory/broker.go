// Package memory provides an in-memory messaging broker for testing.
//
// The InMemoryBroker creates producers and consumers that communicate via
// buffered channels, enabling fast and deterministic integration tests
// without a running message broker.
//
// Message history is always tracked so that test assertion helpers
// ([AssertPublished], [AssertPublishedN], [WaitForMessage], [AssertNoMessages])
// can inspect what was published without consuming from channels.
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

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

	// msgCh is signaled (non-blocking) after every publish so that
	// WaitForMessage can wake up without polling.
	msgCh chan struct{}
}

// NewBroker creates a new in-memory broker with the default buffer size.
func NewBroker() *InMemoryBroker {
	return &InMemoryBroker{
		topics:  make(map[string][]chan messaging.Message),
		history: make(map[string][]messaging.Message),
		bufSize: defaultBufferSize,
		msgCh:   make(chan struct{}, 1),
	}
}

// NewBrokerWithBuffer creates a new in-memory broker with a custom buffer size.
func NewBrokerWithBuffer(size int) *InMemoryBroker {
	return &InMemoryBroker{
		topics:  make(map[string][]chan messaging.Message),
		history: make(map[string][]messaging.Message),
		bufSize: size,
		msgCh:   make(chan struct{}, 1),
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

// AllMessages returns all published messages across every topic, ordered by
// topic name then by publish order.
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

// Reset clears all recorded message history (channels and subscriptions are
// not affected).
func (b *InMemoryBroker) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.history = make(map[string][]messaging.Message)
}

// CreateTopic pre-creates a topic so that it appears in [Topics] even before
// any subscriber or publisher uses it.
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

// Topics returns the sorted list of topics that have been created,
// subscribed to, or published to.
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
		return fmt.Errorf("broker is closed")
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

// Producer creates a new in-memory producer backed by this broker.
func (b *InMemoryBroker) Producer() *InMemoryProducer {
	return &InMemoryProducer{broker: b}
}

// Consumer creates a new in-memory consumer for the given topic.
func (b *InMemoryBroker) Consumer(topic string) *InMemoryConsumer {
	ch := b.subscribe(topic)
	return &InMemoryConsumer{topic: topic, ch: ch}
}

// Close marks the broker as closed.
func (b *InMemoryBroker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for _, subs := range b.topics {
		for _, ch := range subs {
			close(ch)
		}
	}
	b.topics = make(map[string][]chan messaging.Message)
}

// InMemoryProducer implements messaging.Producer using an InMemoryBroker.
type InMemoryProducer struct {
	broker *InMemoryBroker
	closed bool
	mu     sync.Mutex
}

var _ messaging.Producer = (*InMemoryProducer)(nil)

// Publish sends a structured event to the broker.
func (p *InMemoryProducer) Publish(_ context.Context, topic string, event messaging.Event, key ...string) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return fmt.Errorf("producer is closed")
	}
	p.mu.Unlock()

	data, err := event.ToJSON()
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	partitionKey := event.Subject
	if partitionKey == "" && len(key) > 0 {
		partitionKey = key[0]
	}
	if partitionKey == "" {
		partitionKey = event.ID
	}
	msg := messaging.Message{
		Key:       partitionKey,
		Value:     data,
		Topic:     topic,
		Timestamp: event.Timestamp,
		Headers: map[string]string{
			"event-id":     event.ID,
			"event-type":   event.Type,
			"event-source": event.Source,
			"content-type": "application/json",
		},
	}
	return p.broker.publish(topic, msg)
}

// PublishJSON marshals value as JSON and sends it to the broker.
func (p *InMemoryProducer) PublishJSON(_ context.Context, topic string, key string, value interface{}) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return fmt.Errorf("producer is closed")
	}
	p.mu.Unlock()

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	msg := messaging.Message{
		Key:       key,
		Value:     data,
		Topic:     topic,
		Timestamp: time.Now().UTC(),
		Headers:   map[string]string{"content-type": "application/json"},
	}
	return p.broker.publish(topic, msg)
}

// PublishBinary sends raw bytes to the broker.
func (p *InMemoryProducer) PublishBinary(_ context.Context, topic string, key string, data []byte) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return fmt.Errorf("producer is closed")
	}
	p.mu.Unlock()

	msg := messaging.Message{
		Key:       key,
		Value:     data,
		Topic:     topic,
		Timestamp: time.Now().UTC(),
		Headers:   map[string]string{"content-type": "application/octet-stream"},
	}
	return p.broker.publish(topic, msg)
}

// Close marks the producer as closed.
func (p *InMemoryProducer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	return nil
}

// InMemoryConsumer implements messaging.Consumer using an InMemoryBroker.
type InMemoryConsumer struct {
	topic string
	ch    chan messaging.Message
}

var _ messaging.Consumer = (*InMemoryConsumer)(nil)

// Consume blocks reading from the broker channel, calling handler for each message.
func (c *InMemoryConsumer) Consume(ctx context.Context, handler messaging.MessageHandler) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-c.ch:
			if !ok {
				return nil
			}
			if err := handler(ctx, msg); err != nil {
				return err
			}
		}
	}
}

// Topic returns the consumer's topic.
func (c *InMemoryConsumer) Topic() string { return c.topic }

// Close is a no-op — the broker manages the channel lifecycle.
func (c *InMemoryConsumer) Close() error { return nil }

// NewEvent is a convenience helper that creates an Event with auto-generated ID.
func NewEvent[D any](eventType, source string, data D, subject ...string) (messaging.Event, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return messaging.Event{}, fmt.Errorf("memory: marshal event data: %w", err)
	}
	e := messaging.Event{
		ID:        uuid.New().String(),
		Type:      eventType,
		Source:    source,
		Timestamp: time.Now().UTC(),
		Data:      raw,
	}
	if len(subject) > 0 {
		e.Subject = subject[0]
	}
	return e, nil
}
