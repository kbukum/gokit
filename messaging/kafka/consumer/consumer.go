package consumer

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/messaging/kafka"
	"github.com/kbukum/gokit/resilience"
)

// Consumer wraps a kafka-go Reader with TLS/SASL, backoff, and gokit logging.
type Consumer struct {
	reader         *kafkago.Reader
	topic          string
	groupID        string
	log            *logging.Logger
	failures       int
	errCount       *atomic.Int64 // tracks kafka-go internal error count for rate-limiting
	commitStrategy messaging.CommitStrategy
}

// NewConsumer creates a new Kafka consumer for a single topic.
func NewConsumer(common messaging.Config, cfg kafka.Config, topic string, log *logging.Logger) (*Consumer, error) {
	common.ApplyDefaults()
	if err := common.Validate(); err != nil {
		return nil, fmt.Errorf("kafka consumer common config: %w", err)
	}
	if !common.IsEnabled() {
		return nil, fmt.Errorf("kafka consumer: messaging is disabled")
	}
	if err := kafka.ValidateCommonConsumer(common); err != nil {
		return nil, err
	}
	cfg.ApplyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("kafka consumer config: %w", err)
	}
	if err := messaging.ValidateTopic(topic); err != nil {
		return nil, err
	}

	dialer, err := kafka.CreateDialer(&cfg)
	if err != nil {
		return nil, fmt.Errorf("kafka consumer dialer: %w", err)
	}

	if log == nil {
		log = logging.NewDefault("messaging")
	}
	clog := log.WithComponent("kafka.consumer")

	// Rate-limited error logger: log first error, suppress subsequent until recovery.
	var errCount atomic.Int64
	rateLimitedErrLogger := kafkago.LoggerFunc(func(msg string, args ...any) {
		n := errCount.Add(1)
		if n == 1 {
			clog.Warn("Kafka connection issue (retrying automatically)", map[string]any{
				"topic":   topic,
				"groupID": common.ConsumerGroup,
			})
		}
	})

	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:           cfg.Brokers,
		Topic:             topic,
		GroupID:           common.ConsumerGroup,
		Dialer:            dialer,
		StartOffset:       kafkago.FirstOffset,
		MinBytes:          1,
		MaxBytes:          10e6,
		SessionTimeout:    kafka.ParseDuration(cfg.SessionTimeout),
		HeartbeatInterval: kafka.ParseDuration(cfg.HeartbeatInterval),
		RebalanceTimeout:  kafka.ParseDuration(cfg.RebalanceTimeout),
		ErrorLogger:       rateLimitedErrLogger,
	})

	clog.Debug("Kafka consumer initialized", map[string]any{
		"topic":   topic,
		"groupID": common.ConsumerGroup,
		"brokers": cfg.Brokers,
	})

	return &Consumer{
		reader:         reader,
		topic:          topic,
		groupID:        common.ConsumerGroup,
		log:            clog,
		errCount:       &errCount,
		commitStrategy: common.CommitStrategy,
	}, nil
}

// Consume reads messages in a loop, calling handler for each one. It blocks until ctx is canceled
// or an unrecoverable error occurs.
func (c *Consumer) Consume(ctx context.Context, handler messaging.MessageHandler) error {
	c.log.DebugCtx(ctx, "Starting consume loop", map[string]any{
		"topic":   c.topic,
		"groupID": c.groupID,
	})

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		msg, err := c.read(ctx)
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
			c.log.InfoCtx(ctx, "Kafka connection recovered", map[string]any{
				"topic":   c.topic,
				"groupID": c.groupID,
			})
			c.errCount.Store(0)
		}

		domainMsg := kafka.FromKafkaMessage(msg)
		if err := handler(ctx, domainMsg); err != nil {
			c.log.ErrorCtx(ctx, "Message processing failed", map[string]any{
				"error":  err.Error(),
				"topic":  domainMsg.Topic,
				"offset": domainMsg.Offset,
			})
			return err
		}
		if c.commitStrategy == messaging.CommitAfterHandlerSuccess {
			if err := c.reader.CommitMessages(ctx, msg); err != nil {
				return fmt.Errorf("kafka commit message: %w", err)
			}
		}
	}
}

func (c *Consumer) read(ctx context.Context) (kafkago.Message, error) {
	if c.commitStrategy == messaging.CommitAfterHandlerSuccess {
		return c.reader.FetchMessage(ctx)
	}
	return c.reader.ReadMessage(ctx)
}

func (c *Consumer) handleFailure(ctx context.Context, err error) error {
	c.failures++
	if c.failures <= 3 {
		c.log.ErrorCtx(ctx, "Kafka read error", map[string]any{
			"error":    err.Error(),
			"failures": c.failures,
			"topic":    c.topic,
			"groupID":  c.groupID,
		})
	}

	backoff := resilience.BackoffDelay(c.failures, resilience.RetryConfig{
		InitialBackoff: time.Second,
		MaxBackoff:     30 * time.Second,
		Strategy:       resilience.LinearBackoff,
		Jitter:         0,
	})

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
	c.log.Debug("Kafka consumer closing", map[string]any{
		"topic":   c.topic,
		"groupID": c.groupID,
	})
	return c.reader.Close()
}
