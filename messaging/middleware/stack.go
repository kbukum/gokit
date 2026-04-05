package middleware

import (
	"github.com/kbukum/gokit/messaging"
)

// StackBuilder composes messaging middleware into a handler pipeline.
//
// Middleware is applied in a fixed order regardless of builder call order:
//
//	Tracing → Metrics → Dedup → CircuitBreaker → Retry(+DLQ) → Handler
//
// The outermost middleware (Tracing) runs first on each message.
//
// Example:
//
//	handler := NewStack(baseHandler).
//	    WithRetry(retryCfg).
//	    WithMetrics("my-topic", "my-group").
//	    WithTracing().
//	    Build()
type StackBuilder struct {
	base messaging.MessageHandler

	retryCfg   *RetryMiddlewareConfig
	metrics    *metricsOpts
	tracing    bool
	tracingOpt []TracingOption
	dedupCfg   *DedupConfig
	cbCfg      *CircuitBreakerConfig
}

type metricsOpts struct {
	topic string
	group string
}

// NewStack creates a builder that wraps the given base handler.
func NewStack(base messaging.MessageHandler) *StackBuilder {
	return &StackBuilder{base: base}
}

// WithRetry adds retry middleware with the given configuration.
func (b *StackBuilder) WithRetry(cfg RetryMiddlewareConfig) *StackBuilder {
	b.retryCfg = &cfg
	return b
}

// WithMetrics adds metrics/instrumentation middleware.
func (b *StackBuilder) WithMetrics(topic, group string) *StackBuilder {
	b.metrics = &metricsOpts{topic: topic, group: group}
	return b
}

// WithTracing adds distributed tracing middleware.
func (b *StackBuilder) WithTracing(opts ...TracingOption) *StackBuilder {
	b.tracing = true
	b.tracingOpt = opts
	return b
}

// WithDedup adds deduplication middleware.
func (b *StackBuilder) WithDedup(cfg DedupConfig) *StackBuilder {
	b.dedupCfg = &cfg
	return b
}

// WithCircuitBreaker adds circuit breaker middleware.
func (b *StackBuilder) WithCircuitBreaker(cfg CircuitBreakerConfig) *StackBuilder {
	b.cbCfg = &cfg
	return b
}

// Build composes all configured middleware and returns the wrapped handler.
//
// Application order (inner → outer):
//  1. Retry (+DLQ on exhaustion) — innermost, closest to handler
//  2. CircuitBreaker — fail-fast before retry
//  3. Dedup — skip duplicates before processing
//  4. Metrics — record per-message metrics
//  5. Tracing — outermost, creates a span for the full pipeline
func (b *StackBuilder) Build() messaging.MessageHandler {
	h := b.base

	// 1. Retry (innermost)
	if b.retryCfg != nil {
		h = RetryHandler(h, *b.retryCfg)
	}

	// 2. Circuit breaker
	if b.cbCfg != nil {
		h = CircuitBreakerHandler(h, *b.cbCfg)
	}

	// 3. Dedup
	if b.dedupCfg != nil {
		h = DedupHandler(h, *b.dedupCfg)
	}

	// 4. Metrics
	if b.metrics != nil {
		h = InstrumentHandler(b.metrics.topic, b.metrics.group, h)
	}

	// 5. Tracing (outermost)
	if b.tracing {
		h = TracingHandler(h, b.tracingOpt...)
	}

	return h
}
