package consumer

import (
	"context"

	"github.com/kbukum/gokit/kafka"
	"github.com/kbukum/gokit/logger"
)

// StartConsumer creates and starts a ManagedConsumer that processes raw message
// values. This is the most common consumer pattern: read bytes from a topic and
// hand them to a handler.
//
// Example:
//
//	consumer, err := consumer.StartConsumer(ctx, cfg, "my-group", "my-topic",
//	    func(ctx context.Context, data []byte) error {
//	        return processEvent(ctx, data)
//	    }, log)
func StartConsumer(
	ctx context.Context,
	cfg kafka.Config,
	groupID string,
	topic string,
	handler func(ctx context.Context, data []byte) error,
	log *logger.Logger,
) (*ManagedConsumer, error) {
	cfg.GroupID = groupID
	mc, err := NewManagedConsumer(ManagedConsumerConfig{
		Config: cfg,
		Topic:  topic,
		Handler: func(ctx context.Context, msg kafka.Message) error {
			return handler(ctx, msg.Value)
		},
		Log: log,
	})
	if err != nil {
		return nil, err
	}
	if err := mc.Start(ctx); err != nil {
		return nil, err
	}
	return mc, nil
}
