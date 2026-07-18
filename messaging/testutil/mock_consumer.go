package testutil

import (
	"context"

	"github.com/kbukum/gokit/messaging"
)

// ChannelConsumer implements messaging.Consumer for testing. Pre-load messages with Feed(), then call Consume() to deliver them to the provided handler. Consume blocks until the context is canceled.
type ChannelConsumer struct {
	topic string
	ch    chan messaging.Message
}

var _ messaging.Consumer = (*ChannelConsumer)(nil)

// NewChannelConsumer creates a ChannelConsumer with an optional buffer size (default 100).
func NewChannelConsumer(topic string, bufSize ...int) *ChannelConsumer {
	size := 100
	if len(bufSize) > 0 && bufSize[0] > 0 {
		size = bufSize[0]
	}
	return &ChannelConsumer{
		topic: topic,
		ch:    make(chan messaging.Message, size),
	}
}

// Feed enqueues messages for the consumer's Consume loop.
func (c *ChannelConsumer) Feed(msgs ...messaging.Message) {
	for _, m := range msgs {
		c.ch <- m
	}
}

// Consume blocks, calling handler for each message fed to the consumer. It returns when ctx is canceled after draining any buffered messages.
func (c *ChannelConsumer) Consume(ctx context.Context, handler messaging.MessageHandler) error {
	for {
		select {
		case <-ctx.Done():
			for {
				select {
				case msg := <-c.ch:
					_ = handler(ctx, msg)
				default:
					return ctx.Err()
				}
			}
		case msg := <-c.ch:
			if err := handler(ctx, msg); err != nil {
				return err
			}
		}
	}
}

// Topic returns the consumer's topic.
func (c *ChannelConsumer) Topic() string { return c.topic }

// Close is a no-op for the test consumer.
func (c *ChannelConsumer) Close() error { return nil }
