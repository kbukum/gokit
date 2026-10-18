package consumer

import (
	"context"
	"errors"
	"fmt"

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
// Consumption outlives ctx; the caller must Stop the returned consumer.
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
	if cfg.Handler == nil {
		return nil, fmt.Errorf("kafka consumer: StartConsumerConfig.Handler is required")
	}
	mc, err := NewManagedConsumer(ManagedConsumerConfig{ //nolint:contextcheck // kafka-go connection error logger callback fires without a request context
		Common: messaging.Config{Adapter: "kafka", ConsumerGroup: cfg.GroupID},
		Config: cfg.Config,
		Topic:  cfg.Topic,
		Handler: func(ctx context.Context, msg messaging.Message) error {
			return cfg.Handler(ctx, msg.Payload)
		},
		Log: cfg.Log,
	})
	if err != nil {
		return nil, err
	}
	if err := mc.Start(ctx); err != nil {
		return nil, errors.Join(err, mc.Stop(context.WithoutCancel(ctx)))
	}
	return mc, nil
}
