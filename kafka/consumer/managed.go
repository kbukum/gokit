package consumer

import (
	"context"
	"sync"
	"time"

	"github.com/kbukum/gokit/kafka"
	"github.com/kbukum/gokit/logger"
)

// ManagedConsumer wraps a Consumer with background lifecycle management.
type ManagedConsumer struct {
	consumer  *Consumer
	handler   kafka.MessageHandler
	topic     string
	groupID   string
	log       *logger.Logger
	isRunning bool
	cancelFn  context.CancelFunc
	done      chan struct{}
	mu        sync.Mutex
}

// ManagedConsumerConfig holds configuration for a ManagedConsumer.
type ManagedConsumerConfig struct {
	Config  kafka.Config
	Topic   string
	Handler kafka.MessageHandler
	Log     *logger.Logger
}

// NewManagedConsumer creates a managed consumer with lifecycle support.
func NewManagedConsumer(cfg ManagedConsumerConfig) (*ManagedConsumer, error) {
	consumer, err := NewConsumer(cfg.Config, cfg.Topic, cfg.Log)
	if err != nil {
		return nil, err
	}

	return &ManagedConsumer{
		consumer: consumer,
		handler:  cfg.Handler,
		topic:    cfg.Topic,
		groupID:  cfg.Config.GroupID,
		log:      cfg.Log.WithComponent("kafka.managed_consumer"),
	}, nil
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
		"topic":    m.topic,
		"group_id": m.groupID,
	})

	go func() {
		defer close(m.done)

		if err := m.consumer.Consume(consumeCtx, m.handler); err != nil {
			if err != context.Canceled {
				m.log.Error("Managed consumer stopped with error", map[string]interface{}{
					"topic":    m.topic,
					"group_id": m.groupID,
					"error":    err.Error(),
				})
			}
		}

		m.mu.Lock()
		m.isRunning = false
		m.mu.Unlock()

		m.log.Info("Managed consumer stopped", map[string]interface{}{
			"topic":    m.topic,
			"group_id": m.groupID,
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
		"topic":    m.topic,
		"group_id": m.groupID,
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
				"topic":    m.topic,
				"group_id": m.groupID,
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

// GroupID returns the consumer group ID.
func (m *ManagedConsumer) GroupID() string { return m.groupID }
