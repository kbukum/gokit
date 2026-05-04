package middleware

import (
	"context"
	"time"

	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/observability"
)

// InstrumentHandler wraps a MessageHandler with OpenTelemetry metrics:
//
//   - messaging_consumer_messages_total      (counter)   — total messages processed
//   - messaging_consumer_errors_total        (counter)   — total processing errors
//   - messaging_consumer_processing_duration_seconds (histogram) — processing latency
//
// All instruments are labeled with "topic" and "group".
func InstrumentHandler(topic, group string, handler messaging.MessageHandler) messaging.MessageHandler {
	messagesTotal, _ := observability.NewInt64Counter("messaging.consumer", "messaging_consumer_messages_total",
		observability.WithInstrumentDescription("Total number of consumed messages"),
	)
	errorsTotal, _ := observability.NewInt64Counter("messaging.consumer", "messaging_consumer_errors_total",
		observability.WithInstrumentDescription("Total number of messaging consumer errors"),
	)
	processingDuration, _ := observability.NewFloat64Histogram(
		"messaging.consumer",
		"messaging_consumer_processing_duration_seconds",
		observability.WithInstrumentDescription("Duration of message processing in seconds"),
		observability.WithInstrumentUnit("s"),
	)

	attrs := []observability.MetricAttribute{
		observability.MetricStringAttribute("topic", topic),
		observability.MetricStringAttribute("group", group),
	}

	return func(ctx context.Context, msg messaging.Message) error {
		start := time.Now()
		err := handler(ctx, msg)
		duration := time.Since(start)

		messagesTotal.Add(ctx, 1, attrs...)
		processingDuration.Record(ctx, duration.Seconds(), attrs...)

		if err != nil {
			errorsTotal.Add(ctx, 1, attrs...)
		}

		return err
	}
}
