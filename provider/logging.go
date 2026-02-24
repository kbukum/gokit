package provider

import (
	"context"
	"time"

	"github.com/kbukum/gokit/logger"
)

// WithLogging returns a Middleware that logs each Execute call.
// Logs: provider name, duration, and success/error status.
func WithLogging[I, O any](log *logger.Logger) Middleware[I, O] {
	return func(inner RequestResponse[I, O]) RequestResponse[I, O] {
		return &loggingRR[I, O]{inner: inner, log: log}
	}
}

type loggingRR[I, O any] struct {
	inner RequestResponse[I, O]
	log   *logger.Logger
}

func (l *loggingRR[I, O]) Name() string                         { return l.inner.Name() }
func (l *loggingRR[I, O]) IsAvailable(ctx context.Context) bool { return l.inner.IsAvailable(ctx) }

func (l *loggingRR[I, O]) Execute(ctx context.Context, input I) (O, error) {
	start := time.Now()
	output, err := l.inner.Execute(ctx, input)
	duration := time.Since(start)

	fields := map[string]interface{}{
		"provider": l.inner.Name(),
		"duration": duration.String(),
	}

	if err != nil {
		fields["error"] = err.Error()
		l.log.Error("provider execute failed", fields)
	} else {
		l.log.Debug("provider execute ok", fields)
	}

	return output, err
}
