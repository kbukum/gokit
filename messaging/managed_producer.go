package messaging

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kbukum/gokit/logging"
)

// ManagedProducer wraps a Producer with explicit lifecycle management, symmetric to
// ManagedConsumer. Publishing is only permitted while the producer is started, so a
// mis-sequenced or post-shutdown send fails fast rather than racing a closing
// transport. An optional MetricsCollector records each publish outcome.
type ManagedProducer struct {
	producer Producer
	name     string
	log      *logging.Logger
	metrics  MetricsCollector

	mu      sync.Mutex
	running bool
}

// ManagedProducerConfig holds configuration for creating a ManagedProducer.
type ManagedProducerConfig struct {
	// Producer is the underlying transport producer. Required.
	Producer Producer
	// Name identifies the producer in logs. Optional.
	Name string
	// Log is the logger. Optional; nil uses the process default logger.
	Log *logging.Logger
	// Metrics optionally records publish outcomes. Nil disables metrics.
	Metrics MetricsCollector
}

// NewManagedProducer creates a managed producer with lifecycle support. The producer
// must already be created and configured. A nil Log falls back to the process default
// logger so construction never panics.
func NewManagedProducer(cfg ManagedProducerConfig) *ManagedProducer {
	log := cfg.Log
	if log == nil {
		log = logging.Default()
	}
	return &ManagedProducer{
		producer: cfg.Producer,
		name:     cfg.Name,
		log:      log.WithComponent("managed_producer"),
		metrics:  cfg.Metrics,
	}
}

// Start marks the producer ready to publish. It is idempotent.
func (m *ManagedProducer) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return nil
	}
	m.running = true
	m.log.InfoCtx(ctx, "Starting managed producer", map[string]any{"name": m.name})
	return nil
}

// Stop closes the underlying producer and marks it stopped. It is idempotent; a
// producer that was never started is a no-op.
func (m *ManagedProducer) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return nil
	}
	m.running = false
	m.log.InfoCtx(ctx, "Stopping managed producer", map[string]any{"name": m.name})
	return m.producer.Close()
}

// IsRunning reports whether the producer is currently started.
func (m *ManagedProducer) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// Send publishes a single message, recording the outcome in metrics.
func (m *ManagedProducer) Send(ctx context.Context, msg Message) error {
	if err := m.ensureRunning(); err != nil {
		return err
	}
	start := time.Now()
	err := m.producer.Send(ctx, msg)
	m.record(msg.Topic, start, err)
	return err
}

// SendBatch publishes a batch of messages, recording one metric per message.
func (m *ManagedProducer) SendBatch(ctx context.Context, messages []Message) error {
	if err := m.ensureRunning(); err != nil {
		return err
	}
	start := time.Now()
	err := m.producer.SendBatch(ctx, messages)
	for _, msg := range messages {
		m.record(msg.Topic, start, err)
	}
	return err
}

// Flush flushes any buffered messages in the underlying producer.
func (m *ManagedProducer) Flush(ctx context.Context) error {
	if err := m.ensureRunning(); err != nil {
		return err
	}
	return m.producer.Flush(ctx)
}

// Close stops the producer, closing the underlying transport.
func (m *ManagedProducer) Close() error {
	return m.Stop(context.Background())
}

func (m *ManagedProducer) ensureRunning() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return fmt.Errorf("messaging: managed producer %q is not started", m.name)
	}
	return nil
}

func (m *ManagedProducer) record(topic string, start time.Time, err error) {
	if m.metrics != nil {
		m.metrics.RecordPublish(topic, time.Since(start), err)
	}
}
