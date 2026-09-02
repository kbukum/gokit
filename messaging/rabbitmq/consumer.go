package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/kbukum/gokit/messaging"
)

// errDeliveriesClosed reports that the broker closed the delivery stream while
// the consumer was neither canceled nor closed — an abnormal channel/connection
// drop that must not be reported as a clean, success-shaped termination. Its
// message includes "connection closed" so messaging.IsConnectionError classifies
// it as a retryable connection failure.
var errDeliveriesClosed = errors.New("rabbitmq: consume connection closed")

// Consumer consumes messages from RabbitMQ.
type Consumer struct {
	conn   rabbitConn
	ch     rabbitChannel
	dial   func(Config) (rabbitConn, error)
	cfg    Config
	topic  string
	queue  string
	mu     sync.Mutex
	closed bool
}

var _ messaging.Consumer = (*Consumer)(nil)

// NewConsumer creates a lazy RabbitMQ consumer for topic. It connects on Consume.
func NewConsumer(cfg Config, topic string) (*Consumer, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := messaging.ValidateTopic(topic); err != nil {
		return nil, err
	}
	return &Consumer{cfg: cfg, topic: topic, queue: queueName(cfg, topic), dial: defaultDialRabbit}, nil
}

func (c *Consumer) ensureChannel() (rabbitChannel, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, messaging.ErrClosed
	}
	if c.ch != nil {
		return c.ch, nil
	}
	if c.dial == nil {
		c.dial = defaultDialRabbit
	}
	conn, err := c.dial(c.cfg)
	if err != nil {
		return nil, redactError("rabbitmq consumer connect", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		return nil, errors.Join(redactError("rabbitmq consumer channel", err), conn.Close())
	}
	if err := declareExchange(ch, c.cfg); err != nil {
		return nil, errors.Join(err, ch.Close(), conn.Close())
	}
	if _, err := ch.QueueDeclare(c.queue, c.cfg.QueueDurable, false, false, false, nil); err != nil {
		return nil, errors.Join(fmt.Errorf("rabbitmq declare queue: %w", err), ch.Close(), conn.Close())
	}
	if err := bindQueue(ch, c.cfg, c.queue, routingKey(c.cfg, c.topic)); err != nil {
		return nil, errors.Join(err, ch.Close(), conn.Close())
	}
	c.conn = conn
	c.ch = ch
	return ch, nil
}

// Consume reads messages until ctx is canceled.
func (c *Consumer) Consume(ctx context.Context, handler messaging.MessageHandler) error {
	ch, err := c.ensureChannel()
	if err != nil {
		return err
	}
	if c.cfg.PrefetchCount > 0 {
		if qosErr := ch.Qos(c.cfg.PrefetchCount, 0, false); qosErr != nil {
			return fmt.Errorf("rabbitmq qos: %w", qosErr)
		}
	}
	deliveries, err := ch.ConsumeWithContext(ctx, c.queue, "", c.cfg.AutoAck, false, false, false, nil)
	if err != nil {
		if c.isClosed() {
			return messaging.ErrClosed
		}
		return fmt.Errorf("rabbitmq consume: %w", err)
	}
	for delivery := range deliveries {
		headers := make(map[string]string, len(delivery.Headers))
		for key, value := range delivery.Headers {
			headers[key] = fmt.Sprint(value)
		}
		domain := messaging.Message{
			Key:       headers["message-key"],
			Payload:   delivery.Body,
			Topic:     c.topic,
			Timestamp: delivery.Timestamp,
			Headers:   headers,
		}
		if err := handler(ctx, domain); err != nil {
			if !c.cfg.AutoAck {
				if nackErr := delivery.Nack(false, false); nackErr != nil {
					return errors.Join(err, fmt.Errorf("rabbitmq nack: %w", nackErr))
				}
			}
			return err
		}
		if !c.cfg.AutoAck {
			if err := delivery.Ack(false); err != nil {
				return fmt.Errorf("rabbitmq ack: %w", err)
			}
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if c.isClosed() {
		return messaging.ErrClosed
	}
	return fmt.Errorf("rabbitmq consume: %w", errDeliveriesClosed)
}

// Topic returns the subscribed topic.
func (c *Consumer) Topic() string { return c.topic }

// Close closes the channel and connection.
func (c *Consumer) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	var chErr error
	if c.ch != nil {
		chErr = c.ch.Close()
		c.ch = nil
	}
	var connErr error
	if c.conn != nil {
		connErr = c.conn.Close()
		c.conn = nil
	}
	if chErr != nil {
		return chErr
	}
	return connErr
}

func (c *Consumer) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}
