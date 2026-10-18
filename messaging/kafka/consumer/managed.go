package consumer

import (
	"errors"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/messaging/kafka"
)

// ManagedConsumer wraps a Consumer with background lifecycle management.
// It embeds the generic messaging.ManagedConsumer and adds Kafka-specific metadata (GroupID).
type ManagedConsumer struct {
	*messaging.ManagedConsumer
	groupID string
}

// ManagedConsumerConfig holds configuration for a ManagedConsumer.
type ManagedConsumerConfig struct {
	Common  messaging.Config
	Config  kafka.Config
	Topic   string
	Handler messaging.MessageHandler
	Log     *logging.Logger
	// Observer is told about handled messages and failures; nil logs failures to Log.
	Observer messaging.ConsumerObserver
}

// NewManagedConsumer creates a managed consumer with lifecycle support.
func NewManagedConsumer(cfg ManagedConsumerConfig) (*ManagedConsumer, error) {
	consumer, err := NewConsumer(cfg.Common, cfg.Config, cfg.Topic, cfg.Log)
	if err != nil {
		return nil, err
	}

	observer := cfg.Observer
	if observer == nil {
		observer = messaging.LoggingObserver(cfg.Log)
	}
	mc, err := messaging.NewManagedConsumer(messaging.ManagedConsumerConfig{
		Consumer: consumer,
		Handler:  cfg.Handler,
		Observer: observer,
	})
	if err != nil {
		return nil, errors.Join(err, consumer.Close())
	}

	return &ManagedConsumer{
		ManagedConsumer: mc,
		groupID:         cfg.Common.ConsumerGroup,
	}, nil
}

// GroupID returns the consumer group ID.
func (m *ManagedConsumer) GroupID() string { return m.groupID }
