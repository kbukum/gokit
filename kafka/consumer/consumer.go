package consumer

import (
	"context"
	"fmt"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/skillsenselab/gokit/kafka"
	"github.com/skillsenselab/gokit/logger"
)

// MessageHandler processes a Kafka message. Return a non-nil error to log a
// processing failure (the consumer will continue to the next message).
type MessageHandler func(ctx context.Context, msg kafkago.Message) error

// Consumer wraps a kafka-go Reader with TLS/SASL, backoff, and gokit logging.
type Consumer struct {
	reader   *kafkago.Reader
	topic    string
	groupID  string
	log      *logger.Logger
	failures int
}

// NewConsumer creates a new Kafka consumer for a single topic.
func NewConsumer(cfg kafka.Config, topic string, log *logger.Logger) (*Consumer, error) {
	cfg.ApplyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("kafka consumer config: %w", err)
	}

	if !cfg.Enabled {
		return nil, fmt.Errorf("kafka is disabled")
	}

	dialer, err := kafka.CreateDialer(&cfg)
	if err != nil {
		return nil, fmt.Errorf("kafka consumer dialer: %w", err)
	}

	clog := log.WithComponent("kafka.consumer")

	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:           cfg.Brokers,
		Topic:             topic,
		GroupID:           cfg.GroupID,
		Dialer:            dialer,
		StartOffset:       kafkago.FirstOffset,
		MinBytes:          1,
		MaxBytes:          10e6,
		SessionTimeout:    kafka.ParseDuration(cfg.SessionTimeout),
		HeartbeatInterval: kafka.ParseDuration(cfg.HeartbeatInterval),
		RebalanceTimeout:  kafka.ParseDuration(cfg.RebalanceTimeout),
		ErrorLogger: kafkago.LoggerFunc(func(msg string, args ...interface{}) {
			clog.Error("reader: "+msg, map[string]interface{}{
				"args":    fmt.Sprintf("%v", args),
				"topic":   topic,
				"groupID": cfg.GroupID,
			})
		}),
	})

	clog.Info("Kafka consumer initialized", map[string]interface{}{
		"topic":   topic,
		"groupID": cfg.GroupID,
		"brokers": cfg.Brokers,
	})

	return &Consumer{
		reader:  reader,
		topic:   topic,
		groupID: cfg.GroupID,
		log:     clog,
	}, nil
}

// Consume reads messages in a loop, calling handler for each one.
// It blocks until ctx is cancelled or an unrecoverable error occurs.
func (c *Consumer) Consume(ctx context.Context, handler MessageHandler) error {
	c.log.Info("Starting consume loop", map[string]interface{}{
		"topic":   c.topic,
		"groupID": c.groupID,
	})

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			msg, err := c.reader.ReadMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if retryErr := c.handleFailure(ctx, err); retryErr != nil {
					return retryErr
				}
				continue
			}

			c.failures = 0

			if err := handler(ctx, msg); err != nil {
				c.log.Error("Message processing failed", map[string]interface{}{
					"error":  err.Error(),
					"topic":  msg.Topic,
					"offset": msg.Offset,
				})
			}
		}
	}
}

func (c *Consumer) handleFailure(ctx context.Context, err error) error {
	c.failures++
	if c.failures <= 3 {
		c.log.Error("Kafka read error", map[string]interface{}{
			"error":    err.Error(),
			"failures": c.failures,
			"topic":    c.topic,
			"groupID":  c.groupID,
		})
	}

	backoff := time.Duration(c.failures) * time.Second
	if backoff > 30*time.Second {
		backoff = 30 * time.Second
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(backoff):
		return nil
	}
}

// Topic returns the consumer's topic.
func (c *Consumer) Topic() string { return c.topic }

// GroupID returns the consumer's group ID.
func (c *Consumer) GroupID() string { return c.groupID }

// Stats returns reader statistics.
func (c *Consumer) Stats() kafkago.ReaderStats { return c.reader.Stats() }

// Close shuts down the consumer.
func (c *Consumer) Close() error {
	c.log.Info("Kafka consumer closing", map[string]interface{}{
		"topic":   c.topic,
		"groupID": c.groupID,
	})
	return c.reader.Close()
}
