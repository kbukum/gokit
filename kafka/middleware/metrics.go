package middleware

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/kbukum/gokit/kafka"
	"github.com/kbukum/gokit/observability"
)

// InstrumentHandler wraps a MessageHandler with OpenTelemetry metrics:
//
//   - kafka_consumer_messages_total      (counter)   — total messages processed
//   - kafka_consumer_errors_total        (counter)   — total processing errors
//   - kafka_consumer_processing_duration_seconds (histogram) — processing latency
//
// All instruments are labeled with "topic" and "group".
func InstrumentHandler(topic, group string, handler kafka.MessageHandler) kafka.MessageHandler {
	meter := observability.Meter("kafka.consumer")

	messagesTotal, _ := meter.Int64Counter("kafka_consumer_messages_total",
		metric.WithDescription("Total number of consumed Kafka messages"),
	)
	errorsTotal, _ := meter.Int64Counter("kafka_consumer_errors_total",
		metric.WithDescription("Total number of Kafka consumer errors"),
	)
	processingDuration, _ := meter.Float64Histogram(
		"kafka_consumer_processing_duration_seconds",
		metric.WithDescription("Duration of Kafka message processing in seconds"),
		metric.WithUnit("s"),
	)

	attrs := metric.WithAttributes(
		attribute.String("topic", topic),
		attribute.String("group", group),
	)

	return func(ctx context.Context, msg kafka.Message) error {
		start := time.Now()
		err := handler(ctx, msg)
		duration := time.Since(start)

		messagesTotal.Add(ctx, 1, attrs)
		processingDuration.Record(ctx, duration.Seconds(), attrs)

		if err != nil {
			errorsTotal.Add(ctx, 1, attrs)
		}

		return err
	}
}
