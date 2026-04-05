package messaging

import (
	"context"
	"sync"
	"time"

	"github.com/kbukum/gokit/logger"
)

// ManagedConsumer wraps a Consumer with background lifecycle management.
// It runs the consume loop in a goroutine and provides Start/Stop/IsRunning.
type ManagedConsumer struct {
	consumer  Consumer
	handler   MessageHandler
	topic     string
	log       *logger.Logger
	isRunning bool
	cancelFn  context.CancelFunc
	done      chan struct{}
	mu        sync.Mutex
}

// ManagedConsumerConfig holds configuration for creating a ManagedConsumer.
type ManagedConsumerConfig struct {
	Consumer Consumer
	Handler  MessageHandler
	Log      *logger.Logger
}

// NewManagedConsumer creates a managed consumer with lifecycle support.
// The consumer must already be created and configured.
func NewManagedConsumer(cfg ManagedConsumerConfig) *ManagedConsumer {
	return &ManagedConsumer{
		consumer: cfg.Consumer,
		handler:  cfg.Handler,
		topic:    cfg.Consumer.Topic(),
		log:      cfg.Log.WithComponent("managed_consumer"),
	}
}

// Start begins consuming messages in a background goroutine.
func (m *ManagedConsumer) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.isRunning {
		m.mu.Unlock()
		return nil
	}

	consumeCtx, cancel := context.WithCancel(ctx)
	m.cancelFn = cancel
	m.isRunning = true
	m.done = make(chan struct{})
	m.mu.Unlock()

	m.log.Info("Starting managed consumer", map[string]interface{}{
		"topic": m.topic,
	})

	go func() {
		defer close(m.done)

		if err := m.consumer.Consume(consumeCtx, m.handler); err != nil {
			if err != context.Canceled {
				m.log.Error("Managed consumer stopped with error", map[string]interface{}{
					"topic": m.topic,
					"error": err.Error(),
				})
			}
		}

		m.mu.Lock()
		m.isRunning = false
		m.mu.Unlock()

		m.log.Info("Managed consumer stopped", map[string]interface{}{
			"topic": m.topic,
		})
	}()

	return nil
}

// Stop gracefully stops the consumer and waits for the goroutine to finish.
func (m *ManagedConsumer) Stop() error {
	m.mu.Lock()
	if !m.isRunning {
		m.mu.Unlock()
		return nil
	}

	m.log.Info("Stopping managed consumer", map[string]interface{}{
		"topic": m.topic,
	})

	done := m.done
	if m.cancelFn != nil {
		m.cancelFn()
	}
	m.mu.Unlock()

	if done != nil {
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			m.log.Warn("Managed consumer stop timed out", map[string]interface{}{
				"topic": m.topic,
			})
		}
	}

	m.mu.Lock()
	m.isRunning = false
	m.mu.Unlock()

	return m.consumer.Close()
}

// IsRunning returns whether the consumer is currently running.
func (m *ManagedConsumer) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.isRunning
}

// Topic returns the topic this consumer is subscribed to.
func (m *ManagedConsumer) Topic() string { return m.topic }
