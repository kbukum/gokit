package producer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/messaging/kafka"
	"github.com/kbukum/gokit/resilience"
)

const defaultProviderName = "kafka.producer"

// Producer wraps a kafka-go Writer with TLS/SASL and gokit logging.
type Producer struct {
	writer         *kafkago.Writer
	cfg            kafka.Config
	name           string
	retryAttempts  int
	retryBackoff   time.Duration
	requestTimeout time.Duration
	log            *logging.Logger
	mu             sync.RWMutex
	closed         bool
}

// NewProducer creates a new Kafka producer with eager initialization.
func NewProducer(common messaging.Config, cfg kafka.Config, log *logging.Logger) (*Producer, error) {
	p, err := newProducer(common, cfg, log)
	if err != nil {
		return nil, err
	}
	if err := p.initWriter(); err != nil {
		return nil, err
	}
	return p, nil
}

// NewLazyProducer creates a Producer that initializes the underlying writer
// on first use (thread-safe). Useful when Kafka may not be available at startup.
func NewLazyProducer(common messaging.Config, cfg kafka.Config, log *logging.Logger) (*Producer, error) {
	return newProducer(common, cfg, log)
}

func newProducer(common messaging.Config, cfg kafka.Config, log *logging.Logger) (*Producer, error) {
	common.ApplyDefaults()
	if err := common.Validate(); err != nil {
		return nil, fmt.Errorf("kafka producer common config: %w", err)
	}
	if !common.IsEnabled() {
		return nil, fmt.Errorf("kafka producer: messaging is disabled")
	}
	if err := kafka.ValidateCommonProducer(common); err != nil {
		return nil, err
	}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("kafka producer config: %w", err)
	}

	requestTimeout, err := time.ParseDuration(common.RequestTimeout)
	if err != nil {
		return nil, fmt.Errorf("kafka producer request_timeout: %w", err)
	}
	retryBackoff, err := time.ParseDuration(common.RetryBackoff)
	if err != nil {
		return nil, fmt.Errorf("kafka producer retry_backoff: %w", err)
	}
	name := common.Name
	if name == "" {
		name = defaultProviderName
	}
	if log == nil {
		log = logging.NewDefault("messaging")
	}
	return &Producer{
		cfg:            cfg,
		name:           name,
		retryAttempts:  common.RetryAttempts,
		retryBackoff:   retryBackoff,
		requestTimeout: requestTimeout,
		log:            log.WithComponent("kafka.producer"),
	}, nil
}

// initWriter creates the underlying kafka.Writer (idempotent, thread-safe).
func (p *Producer) initWriter() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.writer != nil {
		return nil
	}
	if p.closed {
		return messaging.ErrClosed
	}

	transport, err := kafka.CreateTransport(&p.cfg)
	if err != nil {
		return fmt.Errorf("kafka producer transport: %w", err)
	}

	p.writer = &kafkago.Writer{
		Addr:         kafkago.TCP(p.cfg.Brokers...),
		Transport:    transport,
		Balancer:     &kafkago.LeastBytes{},
		BatchSize:    p.cfg.BatchSize,
		BatchTimeout: kafka.ParseDuration(p.cfg.BatchTimeout),
		RequiredAcks: kafkago.RequiredAcks(p.cfg.RequiredAcks),
		Compression:  kafka.ResolveCompression(p.cfg.Compression),
		WriteTimeout: p.requestTimeout,
		ErrorLogger: kafkago.LoggerFunc(func(msg string, args ...interface{}) {
			p.log.Error("writer: "+msg, map[string]interface{}{ //nolint:contextcheck // kafka-go callback fires from internal goroutines without a request context
				"args": fmt.Sprintf("%v", args),
			})
		}),
	}

	p.log.Debug("Kafka producer initialized", map[string]interface{}{
		"brokers":         p.cfg.Brokers,
		"compression":     p.cfg.Compression,
		"batch_size":      p.cfg.BatchSize,
		"request_timeout": p.requestTimeout.String(),
		"retry_attempts":  p.retryAttempts,
	})

	return nil
}

// ensureWriter guarantees the writer is initialized before use.
func (p *Producer) ensureWriter() error {
	p.mu.RLock()
	if p.writer != nil {
		p.mu.RUnlock()
		return nil
	}
	p.mu.RUnlock()
	return p.initWriter()
}

// WriteMessages sends one or more messages to Kafka with retry logic.
func (p *Producer) WriteMessages(ctx context.Context, msgs ...kafkago.Message) error {
	for i := range msgs {
		if err := messaging.ValidateTopic(msgs[i].Topic); err != nil {
			return err
		}
	}
	if err := p.ensureWriter(); err != nil { //nolint:contextcheck // lazy writer init has no request context; underlying error logger annotated separately
		return err
	}

	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return messaging.ErrClosed
	}
	writer := p.writer
	p.mu.RUnlock()

	attempts := p.retryAttempts
	if attempts <= 0 {
		attempts = 1
	}
	retryCfg := resilience.RetryConfig{
		MaxAttempts:    attempts,
		InitialBackoff: p.retryBackoff,
		MaxBackoff:     time.Duration(attempts) * p.retryBackoff,
		Strategy:       resilience.LinearBackoff,
		Jitter:         0,
		RetryIf: func(err error) bool {
			return !errors.Is(err, context.Canceled) &&
				!errors.Is(err, context.DeadlineExceeded) &&
				ctx.Err() == nil
		},
	}
	if retryCfg.InitialBackoff <= 0 {
		retryCfg.InitialBackoff = time.Second
		retryCfg.MaxBackoff = time.Duration(attempts) * time.Second
	}

	err := resilience.RetryFunc(ctx, retryCfg, func() error {
		writeCtx := ctx
		cancel := func() {}
		if _, ok := ctx.Deadline(); !ok && p.requestTimeout > 0 {
			writeCtx, cancel = context.WithTimeout(ctx, p.requestTimeout)
		}
		err := writer.WriteMessages(writeCtx, msgs...)
		cancel()
		return err
	})
	if err != nil {
		return fmt.Errorf("write after %d attempts: %w", attempts, err)
	}
	return nil
}

// Stats returns writer statistics.
func (p *Producer) Stats() kafkago.WriterStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.writer != nil {
		return p.writer.Stats()
	}
	return kafkago.WriterStats{}
}

// Flush is a no-op because kafka-go Writer writes synchronously when BatchSize
// and BatchTimeout are used without Async. It still reports cancellation and
// closed-producer state so callers can rely on the Producer contract.
func (p *Producer) Flush(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return messaging.ErrClosed
	}
	return nil
}

// Close shuts down the producer.
func (p *Producer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}
	p.closed = true
	p.log.Debug("Kafka producer closing")
	if p.writer != nil {
		return p.writer.Close()
	}
	return nil
}

// SendBatch writes pre-built transport-agnostic messages to Kafka in order.
func (p *Producer) SendBatch(ctx context.Context, messages []messaging.Message) error {
	msgs := make([]kafkago.Message, 0, len(messages))
	for _, msg := range messages {
		msgs = append(msgs, kafka.ToKafkaMessage(msg))
	}
	return p.WriteMessages(ctx, msgs...)
}

// PublishJSON marshals value as JSON and publishes it to the given topic.
func (p *Producer) PublishJSON(ctx context.Context, topic, key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	msg := kafkago.Message{
		Topic: topic,
		Key:   []byte(key),
		Value: data,
		Headers: []kafkago.Header{
			{Key: "content-type", Value: []byte("application/json")},
		},
	}
	return p.WriteMessages(ctx, msg)
}

// PublishBinary publishes raw bytes to the given topic (e.g. protobuf, avro).
func (p *Producer) PublishBinary(ctx context.Context, topic, key string, data []byte) error {
	msg := kafkago.Message{
		Topic: topic,
		Key:   []byte(key),
		Value: data,
		Headers: []kafkago.Header{
			{Key: "content-type", Value: []byte("application/octet-stream")},
		},
	}
	return p.WriteMessages(ctx, msg)
}

// Publish sends a structured gokit Event to Kafka with event metadata headers.
func (p *Producer) Publish(ctx context.Context, topic string, event messaging.Event, key ...string) error {
	data, err := event.ToJSON()
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	partitionKey := event.Subject
	if partitionKey == "" && len(key) > 0 {
		partitionKey = key[0]
	}
	if partitionKey == "" {
		partitionKey = event.ID
	}

	msg := kafkago.Message{
		Topic: topic,
		Key:   []byte(partitionKey),
		Value: data,
		Headers: []kafkago.Header{
			{Key: "event-id", Value: []byte(event.ID)},
			{Key: "event-type", Value: []byte(event.Type)},
			{Key: "event-source", Value: []byte(event.Source)},
			{Key: "content-type", Value: []byte("application/json")},
		},
		Time: event.Timestamp,
	}

	return p.WriteMessages(ctx, msg)
}

// Verify Producer implements messaging.Producer at compile time.
var _ messaging.Producer = (*Producer)(nil)
