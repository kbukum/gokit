package jetstream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"strings"
	"sync"
	"time"

	natsgo "github.com/nats-io/nats.go"
	natsjs "github.com/nats-io/nats.go/jetstream"

	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/resilience"
)

// Producer stores messages in streams. Every send returns once the server has acknowledged that a stream stores the message, bounded by nats.Config.PublishTimeout. A message whose [MsgIDHeader] the stream already holds within its duplicates window is acknowledged without being stored again, which is success.
//
// Only a message with a deduplication ID is retried, since a lost acknowledgement could otherwise store it twice; any other message is attempted once. Failures that cannot pass, such as an oversized message or a stream mismatch, are never retried.
type Producer struct {
	source source
	cfg    ProducerConfig
	owned  io.Closer
	mu     sync.Mutex
	closed bool
}

var _ messaging.Producer = (*Producer)(nil)

func newProducer(src source, cfg ProducerConfig, owned io.Closer) (*Producer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cfg.RetryAttempts = max(cfg.RetryAttempts, 1)
	return &Producer{source: src, cfg: cfg, owned: owned}, nil
}

// Send stores msg on the subject msg.Topic. A non-empty Key is carried in the message-key header.
func (p *Producer) Send(ctx context.Context, msg messaging.Message) error {
	headers := maps.Clone(msg.Headers)
	if msg.Key != "" {
		if headers == nil {
			headers = make(map[string]string, 1)
		}
		if _, ok := headers[keyHeader]; !ok {
			headers[keyHeader] = msg.Key
		}
	}
	return p.publish(ctx, msg.Topic, msg.Payload, headers)
}

// SendBatch stores messages in order, stopping at the first failure.
func (p *Producer) SendBatch(ctx context.Context, messages []messaging.Message) error {
	for _, msg := range messages {
		if err := p.Send(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}

// Publish stores event as JSON with the event ID as its deduplication ID.
func (p *Producer) Publish(ctx context.Context, topic string, event messaging.Event, key ...string) error {
	data, err := event.ToJSON()
	if err != nil {
		return fmt.Errorf("jetstream: marshal event: %w", err)
	}
	headers := map[string]string{
		MsgIDHeader: event.ID, "event-id": event.ID, "event-type": event.Type, "event-source": event.Source,
		"content-type": "application/json",
	}
	if k := event.Subject; k != "" {
		headers[keyHeader] = k
	} else if len(key) > 0 && key[0] != "" {
		headers[keyHeader] = key[0]
	}
	return p.publish(ctx, topic, data, headers)
}

// PublishJSON stores value as JSON.
func (p *Producer) PublishJSON(ctx context.Context, topic, key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("jetstream: marshal JSON: %w", err)
	}
	return p.publish(ctx, topic, data, keyed(key, "application/json"))
}

// PublishBinary stores raw bytes.
func (p *Producer) PublishBinary(ctx context.Context, topic, key string, data []byte) error {
	return p.publish(ctx, topic, data, keyed(key, "application/octet-stream"))
}

// Flush returns at once: every send already waits for the server's acknowledgement.
func (p *Producer) Flush(ctx context.Context) error {
	if _, err := p.client(); err != nil {
		return err
	}
	return ctx.Err()
}

// Close stops the producer, and closes the client only if the producer owns it. It is safe to call more than once.
func (p *Producer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	if p.owned != nil {
		return p.owned.Close()
	}
	return nil
}

func (p *Producer) client() (*Client, error) {
	p.mu.Lock()
	closed := p.closed
	p.mu.Unlock()
	if closed {
		return nil, messaging.ErrClosed
	}
	return p.source()
}

func (p *Producer) publish(ctx context.Context, subject string, data []byte, headers map[string]string) error {
	if err := validateSubject(subject, false); err != nil {
		return err
	}
	headers, id, err := canonicalID(headers)
	if err != nil {
		return err
	}
	c, err := p.client()
	if err != nil {
		return err
	}
	msg := &natsgo.Msg{Subject: subject, Data: data, Header: natsgo.Header{}}
	for k, v := range headers {
		msg.Header.Set(k, v)
	}
	var opts []natsjs.PublishOpt
	if p.cfg.Stream != "" {
		opts = append(opts, natsjs.WithExpectStream(p.cfg.Stream))
	}
	attempts := 1
	if id != "" {
		attempts = p.cfg.RetryAttempts
	}
	retry := resilience.RetryConfig{
		MaxAttempts: attempts, InitialBackoff: max(p.cfg.RetryBackoff, time.Millisecond),
		MaxBackoff: max(time.Duration(p.cfg.RetryAttempts)*p.cfg.RetryBackoff, time.Millisecond), Strategy: resilience.LinearBackoff,
		RetryIf: func(err error) bool { return ctx.Err() == nil && !permanent(err) },
	}
	err = resilience.RetryFunc(ctx, retry, func() error {
		js, apiErr := c.api()
		if apiErr != nil {
			return apiErr
		}
		attempt, cancel := context.WithTimeout(ctx, c.publishTimeout())
		defer cancel()
		_, pubErr := js.PublishMsg(attempt, msg, opts...)
		return pubErr
	})
	if err != nil {
		return fmt.Errorf("jetstream: publish %s: %w", subject, err)
	}
	return nil
}

// canonicalID finds the deduplication ID header in any letter case, checks it and stores it under [MsgIDHeader], which the server matches exactly. It returns the ID, or "" when there is none.
func canonicalID(headers map[string]string) (canonical map[string]string, id string, err error) {
	var key string
	for k := range headers {
		if strings.EqualFold(k, MsgIDHeader) {
			if key != "" {
				return nil, "", errors.New("jetstream: more than one message ID header")
			}
			key = k
		}
	}
	if key == "" {
		return headers, "", nil
	}
	id = headers[key]
	if invalid := validateMessageID(id); invalid != nil {
		return nil, "", invalid
	}
	if key != MsgIDHeader {
		headers = maps.Clone(headers)
		delete(headers, key)
		headers[MsgIDHeader] = id
	}
	return headers, id, nil
}

func keyed(key, contentType string) map[string]string {
	headers := map[string]string{"content-type": contentType}
	if key != "" {
		headers[keyHeader] = key
	}
	return headers
}
