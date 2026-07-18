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

// PausableConsumer is an optional capability for transports that can pause and resume delivery. Core consumers are not required to implement it; callers should check support explicitly.
type PausableConsumer interface {
	Pause(ctx context.Context) error
	Resume(ctx context.Context) error
}

// RebalanceAwareConsumer is an optional capability for transports that expose group rebalance hooks. Adapters that do not implement it reject rebalance semantics at configuration time or omit this interface.
type RebalanceAwareConsumer interface {
	OnRebalanceStart(func(context.Context) error)
	OnRebalanceComplete(func(context.Context) error)
}
