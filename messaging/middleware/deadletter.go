package middleware

import (
	"context"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kbukum/gokit/messaging"
)

const (
	redactedValue      = "<redacted>"
	maxDLQPayloadBytes = 4096
)

// DeadLetterEnvelope is the JSON payload written to the DLQ topic.
type DeadLetterEnvelope struct {
	OriginalTopic string            `json:"original_topic"`
	Error         string            `json:"error"`
	RetryCount    int               `json:"retry_count"`
	Timestamp     time.Time         `json:"timestamp"`
	Headers       map[string]string `json:"headers,omitempty"`
	Payload       string            `json:"payload"`
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
	publisher messaging.Producer
	suffix    string
}

// NewDeadLetterProducer creates a DeadLetterProducer that publishes to "{original_topic}{suffix}" (default suffix is ".dlq").
func NewDeadLetterProducer(publisher messaging.Producer, opts ...DLQOption) *DeadLetterProducer {
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
// The envelope includes a redacted payload summary, redacted headers, sanitized error summary,
// retry count (read from the "x-retry-count" header), and a UTC timestamp.
func (d *DeadLetterProducer) Send(ctx context.Context, msg messaging.Message, originalErr error) error {
	retryCount := 0
	if rc, ok := msg.Headers["x-retry-count"]; ok {
		retryCount, _ = strconv.Atoi(rc)
	}

	envelope := DeadLetterEnvelope{
		OriginalTopic: msg.Topic,
		Error:         sanitizeSummary(originalErr.Error()),
		RetryCount:    retryCount,
		Timestamp:     time.Now().UTC(),
		Headers:       redactHeaders(msg.Headers),
		Payload:       summarizePayloadBytes(msg.Value),
	}

	dlqTopic := msg.Topic + d.suffix
	key := msg.Key
	if key == "" {
		key = "dlq"
	}

	return d.publisher.PublishJSON(ctx, dlqTopic, key, envelope)
}

func redactHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		if isSensitiveKey(key) || containsSensitiveMarker(value) {
			out[key] = redactedValue
			continue
		}
		out[key] = value
	}
	return out
}

func sanitizeSummary(value string) string {
	if containsSensitiveMarker(value) {
		return redactedValue
	}
	return truncateStringBytes(value)
}

func summarizePayloadBytes(payload []byte) string {
	if containsSensitiveMarkerBytes(payload) {
		return redactedValue
	}
	if len(payload) <= maxDLQPayloadBytes {
		return string(payload)
	}
	return string(payload[:maxDLQPayloadBytes]) + "…"
}

func truncateStringBytes(value string) string {
	if len(value) <= maxDLQPayloadBytes {
		return value
	}
	limit := maxDLQPayloadBytes
	for limit > 0 && !utf8.RuneStart(value[limit]) {
		limit--
	}
	if limit == 0 {
		return "…"
	}
	return value[:limit] + "…"
}

func isSensitiveKey(key string) bool {
	return containsSensitiveMarker(key)
}

func containsSensitiveMarker(value string) bool {
	lower := strings.ToLower(value)
	for _, part := range []string{"authorization", "cookie", "token", "secret", "password", "credential", "api-key", "apikey"} {
		if strings.Contains(lower, part) {
			return true
		}
	}
	return false
}

func containsSensitiveMarkerBytes(value []byte) bool {
	for _, marker := range sensitiveMarkers() {
		if containsASCIIFold(value, marker) {
			return true
		}
	}
	return false
}

func containsASCIIFold(value []byte, marker string) bool {
	if marker == "" {
		return true
	}
	if len(value) < len(marker) {
		return false
	}
	first := marker[0]
	for offset := 0; offset <= len(value)-len(marker); offset++ {
		if lowerASCII(value[offset]) != first {
			continue
		}
		if hasASCIIFoldPrefix(value[offset:], marker) {
			return true
		}
	}
	return false
}

func hasASCIIFoldPrefix(value []byte, marker string) bool {
	for i := range marker {
		if lowerASCII(value[i]) != marker[i] {
			return false
		}
	}
	return true
}

func lowerASCII(value byte) byte {
	if value >= 'A' && value <= 'Z' {
		return value + ('a' - 'A')
	}
	return value
}

func sensitiveMarkers() []string {
	return []string{"authorization", "cookie", "token", "secret", "password", "credential", "api-key", "apikey"}
}
