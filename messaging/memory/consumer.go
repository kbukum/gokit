package memory

import (
	"context"
	"errors"
	"fmt"

	"github.com/kbukum/gokit/messaging"
)

// InMemoryConsumer implements messaging.Consumer using an InMemoryBroker.
type InMemoryConsumer struct {
	broker         *InMemoryBroker
	topic          string
	ch             chan messaging.Message
	commitStrategy messaging.CommitStrategy
}

var _ messaging.Consumer = (*InMemoryConsumer)(nil)

// Consume blocks reading from the broker channel, calling handler for each message.
func (c *InMemoryConsumer) Consume(ctx context.Context, handler messaging.MessageHandler) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-c.ch:
			if !ok {
				return nil
			}
			if err := handler(ctx, msg); err != nil {
				if c.commitStrategy == messaging.CommitAfterHandlerSuccess {
					if requeueErr := c.broker.requeue(ctx, c.topic, msg); requeueErr != nil {
						return errors.Join(err, fmt.Errorf("requeue message: %w", requeueErr))
					}
				}
				return err
			}
		}
	}
}

// Topic returns the consumer's topic.
func (c *InMemoryConsumer) Topic() string { return c.topic }

// Close is a no-op — the broker manages the channel lifecycle.
func (c *InMemoryConsumer) Close() error { return nil }
