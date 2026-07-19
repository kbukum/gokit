package consumer

import (
	"context"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/messaging/kafka"
)

// StartConsumerConfig describes a raw Kafka consumer convenience startup.
type StartConsumerConfig struct {
	Config  kafka.Config
	GroupID string
	Topic   string
	Handler func(ctx context.Context, data []byte) error
	Log     *logging.Logger
}

// StartConsumer creates and starts a ManagedConsumer that processes raw message values.
// This is the most common consumer pattern: read bytes from a topic and hand them to a handler.
//
// Example:
//
//	consumer, err := consumer.StartConsumer(ctx, consumer.StartConsumerConfig{
//		Config:  cfg,
//		GroupID: "my-group",
//		Topic:   "my-topic",
//		Handler: func(ctx context.Context, data []byte) error {
//			return processEvent(ctx, data)
//		},
//		Log: log,
//	})
func StartConsumer(ctx context.Context, cfg StartConsumerConfig) (*ManagedConsumer, error) {
	mc, err := NewManagedConsumer(ManagedConsumerConfig{ //nolint:contextcheck // kafka-go connection error logger callback fires without a request context
		Common: messaging.Config{Adapter: "kafka", ConsumerGroup: cfg.GroupID},
		Config: cfg.Config,
		Topic:  cfg.Topic,
		Handler: func(ctx context.Context, msg messaging.Message) error {
			return cfg.Handler(ctx, msg.Value)
		},
		Log: cfg.Log,
	})
	if err != nil {
		return nil, err
	}
	if err := mc.Start(ctx); err != nil {
		return nil, err
	}
	return mc, nil
}
