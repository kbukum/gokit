package consumer

import "context"

// runner wraps a Consumer + MessageHandler to satisfy kafka.ConsumerRunner.
type runner struct {
	consumer *Consumer
	handler  MessageHandler
}

// AsRunner wraps a Consumer with a MessageHandler to create a kafka.ConsumerRunner
// suitable for use with kafka.Component.AddConsumer().
func AsRunner(c *Consumer, h MessageHandler) *runner {
	return &runner{consumer: c, handler: h}
}

func (r *runner) Consume(ctx context.Context) error {
	return r.consumer.Consume(ctx, r.handler)
}

func (r *runner) Close() error {
	return r.consumer.Close()
}

func (r *runner) Topic() string {
	return r.consumer.Topic()
}
