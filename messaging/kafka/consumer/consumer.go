package consumer

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/kbukum/gokit/logger"
	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/messaging/kafka"
)

// Consumer wraps a kafka-go Reader with TLS/SASL, backoff, and gokit logging.
type Consumer struct {
	reader   *kafkago.Reader
	topic    string
	groupID  string
	log      *logger.Logger
	failures int
	errCount *atomic.Int64 // tracks kafka-go internal error count for rate-limiting
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

	// Rate-limited error logger: log first error, suppress subsequent until recovery.
	var errCount atomic.Int64
	rateLimitedErrLogger := kafkago.LoggerFunc(func(msg string, args ...interface{}) {
		n := errCount.Add(1)
		if n == 1 {
			clog.Warn("Kafka connection issue (retrying automatically)", map[string]interface{}{ //nolint:contextcheck // kafka-go callback fires from internal goroutines without a request context
				"topic":   topic,
				"groupID": cfg.GroupID,
			})
		}
	})

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
		ErrorLogger:       rateLimitedErrLogger,
	})

	clog.Debug("Kafka consumer initialized", map[string]interface{}{
		"topic":   topic,
		"groupID": cfg.GroupID,
		"brokers": cfg.Brokers,
	})

	return &Consumer{
		reader:   reader,
		topic:    topic,
		groupID:  cfg.GroupID,
		log:      clog,
		errCount: &errCount,
	}, nil
}

// Consume reads messages in a loop, calling handler for each one.
// It blocks until ctx is canceled or an unrecoverable error occurs.
func (c *Consumer) Consume(ctx context.Context, handler messaging.MessageHandler) error {
	c.log.DebugCtx(ctx, "Starting consume loop", map[string]interface{}{
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
			if c.errCount.Load() > 0 {
				c.log.InfoCtx(ctx, "Kafka connection recovered", map[string]interface{}{
					"topic":   c.topic,
					"groupID": c.groupID,
				})
				c.errCount.Store(0)
			}

			domainMsg := kafka.FromKafkaMessage(msg)

			if err := handler(ctx, domainMsg); err != nil {
				c.log.ErrorCtx(ctx, "Message processing failed", map[string]interface{}{
					"error":  err.Error(),
					"topic":  domainMsg.Topic,
					"offset": domainMsg.Offset,
				})
			}
		}
	}
}

func (c *Consumer) handleFailure(ctx context.Context, err error) error {
	c.failures++
	if c.failures <= 3 {
		c.log.ErrorCtx(ctx, "Kafka read error", map[string]interface{}{
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
	c.log.Debug("Kafka consumer closing", map[string]interface{}{
		"topic":   c.topic,
		"groupID": c.groupID,
	})
	return c.reader.Close()
}
