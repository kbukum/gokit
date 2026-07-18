// Package middleware provides composable middleware for message handlers.
//
// Middleware wraps [messaging.MessageHandler] functions to add cross-cutting concerns such as retry logic,
// dead-letter routing, distributed tracing, metrics collection, deduplication, and circuit breaking
// — all built on top of existing gokit modules.
//
// # Retry
//
// Wrap a handler with exponential-backoff retry using [resilience.RetryFunc]:
//
//	wrapped := middleware.RetryHandler(handler, middleware.RetryMiddlewareConfig{
//	    RetryConfig: resilience.DefaultRetryConfig(),
//	    OnExhausted: func(ctx context.Context, msg messaging.Message, err error) error {
//	        return dlq.Send(ctx, msg, err)
//	    },
//	})
//
// # Dead-Letter Queue
//
// Route failed messages to a DLQ topic:
//
//	dlq := middleware.NewDeadLetterProducer(publisher)
//	dlq.Send(ctx, msg, err)
//
// # Tracing
//
// Add OpenTelemetry distributed tracing to message processing:
//
//	traced := middleware.TracingHandler(handler)
//
// # Metrics
//
// Instrument a handler with OTel metrics (counters + histogram):
//
//	instrumented := middleware.InstrumentHandler("my-topic", "my-group", handler)
//
// # Deduplication
//
// Skip duplicate messages by message ID or custom key:
//
//	deduped := middleware.DedupHandler(handler, middleware.DedupConfig{TTL: 5 * time.Minute})
//
// # Circuit Breaker
//
// Fail-fast when downstream is unhealthy (wraps resilience.CircuitBreaker):
//
//	protected := middleware.CircuitBreakerHandler(handler, middleware.CircuitBreakerConfig{
//	    Threshold: 5, Timeout: 30 * time.Second,
//	})
package middleware
