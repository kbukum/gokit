package testutil

import (
	"context"
	"fmt"
	"sync"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/kafka"
	"github.com/kbukum/gokit/testutil"
)

// Message represents a produced or consumed message.
type Message struct {
	Topic string
	Key   []byte
	Value []byte
}

// MockProducer is an in-memory producer that records all written messages.
type MockProducer struct {
	messages []Message
	mu       sync.Mutex
}

var _ kafka.ProducerCloser = (*MockProducer)(nil)

// WriteMessage records a message.
func (p *MockProducer) WriteMessage(topic string, key, value []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.messages = append(p.messages, Message{Topic: topic, Key: key, Value: value})
}

// Messages returns all recorded messages.
func (p *MockProducer) Messages() []Message {
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := make([]Message, len(p.messages))
	copy(cp, p.messages)
	return cp
}

// Reset clears all recorded messages.
func (p *MockProducer) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.messages = nil
}

// Close is a no-op for the mock.
func (p *MockProducer) Close() error { return nil }

// MockConsumer is an in-memory consumer that blocks until fed messages or canceled.
type MockConsumer struct {
	topic    string
	messages chan Message
	handler  func(Message)
}

var _ kafka.ConsumerRunner = (*MockConsumer)(nil)

// NewMockConsumer creates a mock consumer for the given topic.
func NewMockConsumer(topic string) *MockConsumer {
	return &MockConsumer{
		topic:    topic,
		messages: make(chan Message, 100),
	}
}

// OnMessage sets a handler called for each consumed message.
func (c *MockConsumer) OnMessage(fn func(Message)) {
	c.handler = fn
}

// Feed sends a message to the consumer's input channel.
func (c *MockConsumer) Feed(msg Message) {
	c.messages <- msg
}

// Consume blocks until context is canceled, processing fed messages.
func (c *MockConsumer) Consume(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg := <-c.messages:
			if c.handler != nil {
				c.handler(msg)
			}
		}
	}
}

// Close is a no-op for the mock.
func (c *MockConsumer) Close() error { return nil }

// Topic returns the consumer's topic.
func (c *MockConsumer) Topic() string { return c.topic }

// Component is a test Kafka component with mock producer and consumers.
type Component struct {
	producer  *MockProducer
	consumers map[string]*MockConsumer
	started   bool
	mu        sync.RWMutex
}

var _ component.Component = (*Component)(nil)
var _ testutil.TestComponent = (*Component)(nil)

// NewComponent creates a new mock Kafka test component.
func NewComponent() *Component {
	return &Component{
		producer:  &MockProducer{},
		consumers: make(map[string]*MockConsumer),
	}
}

// AddConsumer adds a mock consumer for the given topic.
func (c *Component) AddConsumer(topic string) *MockConsumer {
	c.mu.Lock()
	defer c.mu.Unlock()
	mc := NewMockConsumer(topic)
	c.consumers[topic] = mc
	return mc
}

// MockProducerClient returns the mock producer for assertions.
func (c *Component) MockProducerClient() *MockProducer {
	return c.producer
}

// MockConsumerClient returns the mock consumer for a specific topic.
func (c *Component) MockConsumerClient(topic string) *MockConsumer {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.consumers[topic]
}

// --- component.Component ---

func (c *Component) Name() string { return "kafka-test" }

func (c *Component) Start(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started {
		return fmt.Errorf("component already started")
	}
	c.started = true
	return nil
}

func (c *Component) Stop(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.started = false
	return nil
}

func (c *Component) Health(_ context.Context) component.Health {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.started {
		return component.Health{Name: c.Name(), Status: component.StatusUnhealthy, Message: "not started"}
	}
	return component.Health{Name: c.Name(), Status: component.StatusHealthy}
}

// --- testutil.TestComponent ---

func (c *Component) Reset(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.started {
		return fmt.Errorf("component not started")
	}
	c.producer.Reset()
	return nil
}

func (c *Component) Snapshot(_ context.Context) (interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.producer.Messages(), nil
}

func (c *Component) Restore(_ context.Context, _ interface{}) error {
	// Kafka is a log — restoring state doesn't make semantic sense.
	// Reset is the appropriate operation for test isolation.
	return nil
}
