package middleware

import (
	"context"
	"strconv"
	"time"

	"github.com/kbukum/gokit/kafka"
)

// DeadLetterEnvelope is the JSON payload written to the DLQ topic.
type DeadLetterEnvelope struct {
	OriginalTopic string            `json:"original_topic"`
	Error         string            `json:"error"`
	RetryCount    int               `json:"retry_count"`
	Timestamp     time.Time         `json:"timestamp"`
	Headers       map[string]string `json:"headers,omitempty"`
	Payload       []byte            `json:"payload"`
}

// DLQOption configures a DeadLetterProducer.
type DLQOption func(*DeadLetterProducer)

// WithSuffix overrides the default DLQ topic suffix (".dlq").
func WithSuffix(suffix string) DLQOption {
	return func(d *DeadLetterProducer) {
		d.suffix = suffix
	}
}

// DeadLetterProducer sends failed messages to a dead-letter topic.
type DeadLetterProducer struct {
	publisher kafka.Publisher
	suffix    string
}

// NewDeadLetterProducer creates a DeadLetterProducer that publishes to
// "{original_topic}{suffix}" (default suffix is ".dlq").
func NewDeadLetterProducer(publisher kafka.Publisher, opts ...DLQOption) *DeadLetterProducer {
	d := &DeadLetterProducer{
		publisher: publisher,
		suffix:    ".dlq",
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// Send publishes a DeadLetterEnvelope to the DLQ topic for the given message.
// The envelope includes the original payload, headers, error description,
// retry count (read from the "x-retry-count" header), and a UTC timestamp.
func (d *DeadLetterProducer) Send(ctx context.Context, msg kafka.Message, originalErr error) error {
	retryCount := 0
	if rc, ok := msg.Headers["x-retry-count"]; ok {
		retryCount, _ = strconv.Atoi(rc)
	}

	envelope := DeadLetterEnvelope{
		OriginalTopic: msg.Topic,
		Error:         originalErr.Error(),
		RetryCount:    retryCount,
		Timestamp:     time.Now().UTC(),
		Headers:       msg.Headers,
		Payload:       msg.Value,
	}

	dlqTopic := msg.Topic + d.suffix
	key := msg.Key
	if key == "" {
		key = "dlq"
	}

	return d.publisher.PublishJSON(ctx, dlqTopic, key, envelope)
}
