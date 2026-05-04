package nats

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kbukum/gokit/messaging"
	natsgo "github.com/nats-io/nats.go"
)

// Consumer consumes messages from a NATS subject.
type Consumer struct {
	conn   *natsgo.Conn
	sub    *natsgo.Subscription
	cfg    Config
	topic  string
	mu     sync.Mutex
	closed bool
}

var _ messaging.Consumer = (*Consumer)(nil)

// NewConsumer creates a lazy NATS consumer for topic. It connects on Consume.
func NewConsumer(cfg Config, topic string) (*Consumer, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := messaging.ValidateTopic(topic); err != nil {
		return nil, err
	}
	return &Consumer{cfg: cfg, topic: topic}, nil
}

func (c *Consumer) ensureSubscription() (*natsgo.Subscription, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, messaging.ErrClosed
	}
	if c.sub != nil {
		return c.sub, nil
	}
	opts, err := c.cfg.connectOptions()
	if err != nil {
		return nil, err
	}
	conn, err := natsgo.Connect(c.cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("nats consumer connect %s: %w", c.cfg.RedactedURL(), err)
	}
	var sub *natsgo.Subscription
	if c.cfg.QueueGroup != "" {
		sub, err = conn.QueueSubscribeSync(subject(c.cfg, c.topic), c.cfg.QueueGroup)
	} else {
		sub, err = conn.SubscribeSync(subject(c.cfg, c.topic))
	}
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("nats subscribe: %w", err)
	}
	c.conn = conn
	c.sub = sub
	return sub, nil
}

// Consume reads messages until ctx is canceled.
func (c *Consumer) Consume(ctx context.Context, handler messaging.MessageHandler) error {
	sub, err := c.ensureSubscription()
	if err != nil {
		return err
	}
	for {
		msg, err := sub.NextMsgWithContext(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if c.isClosed() {
				return messaging.ErrClosed
			}
			return fmt.Errorf("nats receive: %w", err)
		}
		headers := make(map[string]string, len(msg.Header))
		for key, values := range msg.Header {
			if len(values) > 0 {
				headers[key] = values[0]
			}
		}
		domain := messaging.Message{
			Key:       headers["message-key"],
			Value:     msg.Data,
			Topic:     c.topic,
			Timestamp: time.Now().UTC(),
			Headers:   headers,
		}
		if err := handler(ctx, domain); err != nil {
			return err
		}
	}
}

// Topic returns the subscribed topic.
func (c *Consumer) Topic() string { return c.topic }

// Close unsubscribes and closes the connection.
func (c *Consumer) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	var err error
	if c.sub != nil {
		err = c.sub.Unsubscribe()
		c.sub = nil
	}
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	return err
}

func (c *Consumer) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}
