package consumer

import (
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
}

// NewManagedConsumer creates a managed consumer with lifecycle support.
func NewManagedConsumer(cfg ManagedConsumerConfig) (*ManagedConsumer, error) {
	consumer, err := NewConsumer(cfg.Common, cfg.Config, cfg.Topic, cfg.Log)
	if err != nil {
		return nil, err
	}

	mc := messaging.NewManagedConsumer(messaging.ManagedConsumerConfig{
		Consumer: consumer,
		Handler:  cfg.Handler,
		Log:      cfg.Log,
	})

	return &ManagedConsumer{
		ManagedConsumer: mc,
		groupID:         cfg.Common.ConsumerGroup,
	}, nil
}

// GroupID returns the consumer group ID.
func (m *ManagedConsumer) GroupID() string { return m.groupID }
