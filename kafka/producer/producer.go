package producer

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/kbukum/gokit/kafka"
	"github.com/kbukum/gokit/logger"
)

// Producer wraps a kafka-go Writer with TLS/SASL, retries, and gokit logging.
type Producer struct {
	writer *kafkago.Writer
	cfg    kafka.Config
	log    *logger.Logger
	mu     sync.RWMutex
	closed bool
}

// NewProducer creates a new Kafka producer with eager initialization.
func NewProducer(cfg kafka.Config, log *logger.Logger) (*Producer, error) {
	cfg.ApplyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("kafka producer config: %w", err)
	}

	if !cfg.Enabled {
		return nil, fmt.Errorf("kafka is disabled")
	}

	p := &Producer{cfg: cfg, log: log.WithComponent("kafka.producer")}
	if err := p.initWriter(); err != nil {
		return nil, err
	}
	return p, nil
}

// NewLazyProducer creates a Producer that initializes the underlying writer
// on first use (thread-safe). Useful when Kafka may not be available at startup.
func NewLazyProducer(cfg kafka.Config, log *logger.Logger) (*Producer, error) {
	cfg.ApplyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("kafka producer config: %w", err)
	}

	if !cfg.Enabled {
		return nil, fmt.Errorf("kafka is disabled")
	}

	return &Producer{cfg: cfg, log: log.WithComponent("kafka.producer")}, nil
}

// initWriter creates the underlying kafka.Writer (idempotent, thread-safe).
func (p *Producer) initWriter() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.writer != nil {
		return nil
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
		WriteTimeout: kafka.ParseDuration(p.cfg.WriteTimeout),
		ErrorLogger: kafkago.LoggerFunc(func(msg string, args ...interface{}) {
			p.log.Error("writer: "+msg, map[string]interface{}{
				"args": fmt.Sprintf("%v", args),
			})
		}),
	}

	p.log.Info("Kafka producer initialized", map[string]interface{}{
		"brokers":     p.cfg.Brokers,
		"compression": p.cfg.Compression,
		"batch_size":  p.cfg.BatchSize,
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
	if err := p.ensureWriter(); err != nil {
		return err
	}

	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return fmt.Errorf("producer is closed")
	}
	p.mu.RUnlock()

	var lastErr error
	for attempt := 1; attempt <= p.cfg.Retries; attempt++ {
		if err := p.writer.WriteMessages(ctx, msgs...); err == nil {
			return nil
		} else {
			lastErr = err
			if attempt < p.cfg.Retries {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Duration(attempt) * 100 * time.Millisecond):
				}
			}
		}
	}
	return fmt.Errorf("write after %d retries: %w", p.cfg.Retries, lastErr)
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

// Close shuts down the producer.
func (p *Producer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}
	p.closed = true
	p.log.Info("Kafka producer closing")
	if p.writer != nil {
		return p.writer.Close()
	}
	return nil
}

// SendJSON marshals value as JSON and sends it to the given topic with the given key.
func (p *Producer) SendJSON(ctx context.Context, topic, key string, value interface{}) error {
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

// SendBinary sends raw bytes to the given topic with the given key.
func (p *Producer) SendBinary(ctx context.Context, topic, key string, data []byte) error {
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
