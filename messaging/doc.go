// Package messaging provides transport-agnostic message producer and consumer
// abstractions for event-driven architectures.
//
// It defines shared types (Message, Event, handler functions), interfaces
// (Producer, Consumer, ErrorClassifier, MetricsCollector), and generic patterns
// (AsRunner, ManagedConsumer, provider adapters) that are independent of any
// specific message broker.
//
// # Router
//
// Route incoming messages to different handlers based on topic, event type,
// or custom rules using [Router]. Supports exact match, wildcard patterns
// (e.g. "content.*"), and a default fallback handler.
//
// # BatchProducer
//
// Collect messages and flush in batches via [BatchProducer]. Supports
// size-triggered, time-triggered, and byte-triggered flushing with
// graceful shutdown.
//
// # Sub-packages
//
//   - messaging/kafka:      Kafka implementation using segmentio/kafka-go
//   - messaging/memory:     In-memory broker for testing
//   - messaging/middleware:  Transport-agnostic middleware (retry, DLQ, tracing, metrics, dedup, circuit breaker)
//   - messaging/testutil:   Broker-agnostic mock producer/consumer for testing
//
// # Configuration
//
// Kafka-specific settings are provided via kafka.Config with ApplyDefaults()/Validate().
package messaging
