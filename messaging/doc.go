// Package messaging provides transport-agnostic message producer and consumer
// abstractions for event-driven architectures.
//
// It defines shared types (Message, Event, handler functions) and interfaces
// (Producer, Consumer) that are independent of any specific message broker.
//
// # Sub-packages
//
//   - messaging/kafka:      Kafka implementation using segmentio/kafka-go
//   - messaging/memory:     In-memory broker for testing
//   - messaging/middleware:  Transport-agnostic middleware (retry, DLQ, tracing, metrics)
//
// # Configuration
//
// Kafka-specific settings are provided via kafka.Config with ApplyDefaults()/Validate().
package messaging
