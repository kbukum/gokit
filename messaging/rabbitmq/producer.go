package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/resilience"
)

// Producer publishes messages through RabbitMQ.
type Producer struct {
	conn          rabbitConn
	ch            rabbitChannel
	cfg           Config
	retryAttempts int
	retryBackoff  time.Duration
	mu            sync.Mutex
	closed        bool
	declared      map[string]struct{}
}

var _ messaging.Producer = (*Producer)(nil)

// NewProducer creates a lazy RabbitMQ producer. It connects on first publish.
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
	return &Producer{cfg: cfg, retryAttempts: retryAttempts, retryBackoff: retryBackoff, declared: make(map[string]struct{})}, nil
}

func (p *Producer) ensureChannelLocked() (rabbitChannel, error) {
	if p.closed {
		return nil, messaging.ErrClosed
	}
	if p.ch != nil {
		return p.ch, nil
	}
	conn, err := dialRabbit(p.cfg)
	if err != nil {
		return nil, redactError("rabbitmq producer connect", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, redactError("rabbitmq producer channel", err)
	}
	if err := declareExchange(ch, p.cfg); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, err
	}
	p.conn = conn
	p.ch = ch
	return ch, nil
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
		return fmt.Errorf("rabbitmq marshal event: %w", err)
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
		return fmt.Errorf("rabbitmq marshal JSON: %w", err)
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
	p.mu.Lock()
	defer p.mu.Unlock()

	ch, err := p.ensureChannelLocked()
	if err != nil {
		return err
	}
	table := amqp.Table{}
	contentType := headers["content-type"]
	for key, value := range headers {
		table[key] = value
	}
	queue := queueName(p.cfg, topic)
	if _, ok := p.declared[queue]; !ok {
		if _, err := ch.QueueDeclare(queue, p.cfg.QueueDurable, false, false, false, nil); err != nil {
			return fmt.Errorf("rabbitmq declare queue: %w", err)
		}
		if err := bindQueue(ch, p.cfg, queue, routingKey(p.cfg, topic)); err != nil {
			return err
		}
		p.declared[queue] = struct{}{}
	}
	publishing := amqp.Publishing{
		ContentType: contentType,
		Body:        data,
		Timestamp:   time.Now().UTC(),
		Headers:     table,
	}
	return p.publishWithRetry(ctx, ch, topic, publishing)
}

func (p *Producer) publishWithRetry(ctx context.Context, ch rabbitChannel, topic string, publishing amqp.Publishing) error {
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
		publishCtx := ctx
		cancel := func() {}
		if _, ok := ctx.Deadline(); !ok {
			publishCtx, cancel = context.WithTimeout(ctx, mustDuration(p.cfg.PublishTimeout))
		}
		err := ch.PublishWithContext(publishCtx, p.cfg.Exchange, routingKey(p.cfg, topic), false, false, publishing)
		cancel()
		if err == nil {
			return nil
		}
		return fmt.Errorf("rabbitmq publish: %w", err)
	})
	if err != nil {
		return fmt.Errorf("rabbitmq publish after %d attempts: %w", p.retryAttempts, err)
	}
	return nil
}

// Flush is a no-op for RabbitMQ because PublishWithContext confirms handoff to the client.
func (p *Producer) Flush(ctx context.Context) error { return ctx.Err() }

// Close closes the channel and connection.
func (p *Producer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	var chErr error
	if p.ch != nil {
		chErr = p.ch.Close()
		p.ch = nil
	}
	var connErr error
	if p.conn != nil {
		connErr = p.conn.Close()
		p.conn = nil
	}
	if chErr != nil {
		return chErr
	}
	return connErr
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
