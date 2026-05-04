package messaging

import "context"

// Producer is a transport-agnostic message producer.
//
// Three methods for three use cases:
//   - Publish:       structured domain events (gokit Event with headers/metadata)
//   - PublishJSON:   arbitrary data as JSON (direct marshal, no envelope)
//   - PublishBinary: raw bytes (protobuf, avro, etc. — zero encoding overhead)
type Producer interface {
	Send(ctx context.Context, msg Message) error
	SendBatch(ctx context.Context, messages []Message) error
	Publish(ctx context.Context, topic string, event Event, key ...string) error
	PublishJSON(ctx context.Context, topic, key string, value any) error
	PublishBinary(ctx context.Context, topic, key string, data []byte) error
	Flush(ctx context.Context) error
	Close() error
}

// ProducerCloser is satisfied by any producer that can be closed.
type ProducerCloser interface {
	Close() error
}
