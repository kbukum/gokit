package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	natsgo "github.com/nats-io/nats.go"

	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/resilience"
)

// Producer publishes messages to NATS subjects.
type Producer struct {
	conn          natsConn
	connect       func(string, ...natsgo.Option) (natsConn, error)
	cfg           Config
	retryAttempts int
	retryBackoff  time.Duration
	mu            sync.Mutex
	closed        bool
}

var _ messaging.Producer = (*Producer)(nil)

// NewProducer creates a lazy NATS producer. It connects on first publish.
func NewProducer(cfg Config) (*Producer, error) {
	return newProducer(cfg, 1, 0)
}

func newProducer(cfg Config, retryAttempts int, retryBackoff time.Duration) (*Producer, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if retryAttempts <= 0 {
		retryAttempts = 1
	}
	return &Producer{cfg: cfg, retryAttempts: retryAttempts, retryBackoff: retryBackoff, connect: defaultConnectNATS}, nil
}

func (p *Producer) ensureConnLocked() (natsConn, error) {
	if p.closed {
		return nil, messaging.ErrClosed
	}
	if p.conn != nil && !p.conn.IsClosed() {
		return p.conn, nil
	}
	opts, err := p.cfg.connectOptions()
	if err != nil {
		return nil, err
	}
	conn, err := p.connect(p.cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("nats producer connect %s: %w", p.cfg.RedactedURL(), err)
	}
	p.conn = conn
	return conn, nil
}

// Send writes a pre-built transport-agnostic message.
func (p *Producer) Send(ctx context.Context, msg messaging.Message) error {
	return p.publish(ctx, msg.Topic, msg.Value, headersWithMessageKey(msg.Headers, msg.Key))
}

// SendBatch writes pre-built messages in order.
func (p *Producer) SendBatch(ctx context.Context, messages []messaging.Message) error {
	for _, msg := range messages {
		if err := p.Send(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}

// Publish sends a structured event.
func (p *Producer) Publish(ctx context.Context, topic string, event messaging.Event, key ...string) error {
	data, err := event.ToJSON()
	if err != nil {
		return fmt.Errorf("nats marshal event: %w", err)
	}
	headers := map[string]string{
		"event-id":     event.ID,
		"event-type":   event.Type,
		"event-source": event.Source,
		"content-type": "application/json",
	}
	partitionKey := event.Subject
	if partitionKey == "" && len(key) > 0 {
		partitionKey = key[0]
	}
	if partitionKey != "" {
		headers["message-key"] = partitionKey
	}
	return p.publish(ctx, topic, data, headers)
}

// PublishJSON marshals value as JSON and publishes it.
func (p *Producer) PublishJSON(ctx context.Context, topic, key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("nats marshal JSON: %w", err)
	}
	headers := map[string]string{"content-type": "application/json"}
	if key != "" {
		headers["message-key"] = key
	}
	return p.publish(ctx, topic, data, headers)
}

// PublishBinary publishes raw bytes.
func (p *Producer) PublishBinary(ctx context.Context, topic, key string, data []byte) error {
	headers := map[string]string{"content-type": "application/octet-stream"}
	if key != "" {
		headers["message-key"] = key
	}
	return p.publish(ctx, topic, data, headers)
}

func (p *Producer) publish(ctx context.Context, topic string, data []byte, headers map[string]string) error {
	if err := messaging.ValidateTopic(topic); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	conn, err := p.ensureConnLocked()
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	msg := &natsgo.Msg{Subject: subject(p.cfg, topic), Data: data, Header: natsgo.Header{}}
	for key, value := range headers {
		msg.Header.Set(key, value)
	}
	return p.publishWithRetry(ctx, conn, msg)
}

func (p *Producer) publishWithRetry(ctx context.Context, conn natsConn, msg *natsgo.Msg) error {
	retryCfg := resilience.RetryConfig{
		MaxAttempts:    p.retryAttempts,
		InitialBackoff: p.retryBackoff,
		MaxBackoff:     time.Duration(p.retryAttempts) * p.retryBackoff,
		Strategy:       resilience.LinearBackoff,
		Jitter:         0,
		RetryIf: func(error) bool {
			return ctx.Err() == nil
		},
	}
	if retryCfg.InitialBackoff <= 0 {
		retryCfg.InitialBackoff = time.Second
		retryCfg.MaxBackoff = time.Duration(p.retryAttempts) * time.Second
	}

	err := resilience.RetryFunc(ctx, retryCfg, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := conn.PublishMsg(msg); err != nil {
			return fmt.Errorf("nats publish: %w", err)
		}
		if err := conn.FlushTimeout(mustDuration(p.cfg.PublishTimeout)); err != nil {
			return fmt.Errorf("nats publish flush: %w", err)
		}
		return ctx.Err()
	})
	if err != nil {
		return fmt.Errorf("nats publish after %d attempts: %w", p.retryAttempts, err)
	}
	return nil
}

// Flush waits for pending NATS publishes.
func (p *Producer) Flush(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	conn, err := p.ensureConnLocked()
	if err != nil {
		return err
	}
	if err := conn.FlushTimeout(mustDuration(p.cfg.PublishTimeout)); err != nil {
		return fmt.Errorf("nats flush: %w", err)
	}
	return ctx.Err()
}

// Close drains the NATS connection.
func (p *Producer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	if p.conn == nil {
		return nil
	}
	if err := p.conn.Drain(); err != nil {
		return fmt.Errorf("nats drain: %w", err)
	}
	p.conn.Close()
	p.conn = nil
	return nil
}

func headersWithMessageKey(headers map[string]string, key string) map[string]string {
	if key == "" {
		return headers
	}
	out := make(map[string]string, len(headers)+1)
	for header, value := range headers {
		out[header] = value
	}
	if _, ok := out["message-key"]; !ok {
		out["message-key"] = key
	}
	return out
}
