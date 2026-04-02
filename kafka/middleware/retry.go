package middleware

import (
	"context"
	"strconv"

	"github.com/kbukum/gokit/kafka"
	"github.com/kbukum/gokit/resilience"
)

// RetryMiddlewareConfig extends resilience.RetryConfig with a callback
// invoked when all retry attempts are exhausted (e.g. for DLQ routing).
type RetryMiddlewareConfig struct {
	resilience.RetryConfig

	// OnExhausted is called after all retries fail. Use this to route
	// the message to a dead-letter queue. May be nil.
	OnExhausted func(ctx context.Context, msg kafka.Message, err error)
}

// RetryHandler wraps a MessageHandler with retry logic powered by
// resilience.RetryFunc. Each retry attempt updates the "x-retry-count"
// header on the message so downstream consumers (and DLQ producers)
// can observe how many times processing was attempted.
func RetryHandler(handler kafka.MessageHandler, cfg RetryMiddlewareConfig) kafka.MessageHandler {
	return func(ctx context.Context, msg kafka.Message) error {
		// Clone headers so retries don't mutate the caller's map.
		headers := make(map[string]string, len(msg.Headers)+1)
		for k, v := range msg.Headers {
			headers[k] = v
		}
		msg.Headers = headers

		var attempt int
		err := resilience.RetryFunc(ctx, cfg.RetryConfig, func() error {
			attempt++
			if attempt > 1 {
				msg.Headers["x-retry-count"] = strconv.Itoa(attempt - 1)
			}
			return handler(ctx, msg)
		})

		if err != nil && cfg.OnExhausted != nil {
			cfg.OnExhausted(ctx, msg, err)
		}
		return err
	}
}
