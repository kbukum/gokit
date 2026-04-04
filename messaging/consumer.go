package messaging

import "context"

// Consumer runs a blocking consume loop.
type Consumer interface {
	Consume(ctx context.Context, handler MessageHandler) error
	Topic() string
	Close() error
}

// ConsumerRunner is used by Component to manage consumer lifecycle.
type ConsumerRunner interface {
	Consume(ctx context.Context) error
	Close() error
	Topic() string
}
