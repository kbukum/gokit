package middleware

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/observability"
)

// kafkaHeaderCarrier adapts a map[string]string to propagation.TextMapCarrier
// so OpenTelemetry propagators can inject/extract trace context via message headers.
type kafkaHeaderCarrier map[string]string

func (c kafkaHeaderCarrier) Get(key string) string { return c[key] }
func (c kafkaHeaderCarrier) Set(key, value string) { c[key] = value }
func (c kafkaHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// InjectTraceContext writes the current span's trace context into the provided message headers using the globally registered propagator.
func InjectTraceContext(ctx context.Context, headers map[string]string) {
	observability.InjectTraceContext(ctx, kafkaHeaderCarrier(headers))
}

// ExtractTraceContext reads trace context from message headers
// and returns a new context carrying the extracted span context.
func ExtractTraceContext(ctx context.Context, headers map[string]string) context.Context {
	return observability.ExtractTraceContext(ctx, kafkaHeaderCarrier(headers))
}

// TracingOption configures TracingHandler behavior.
type TracingOption func(*tracingConfig)

type tracingConfig struct {
	spanNameFunc func(messaging.Message) string
	tracerName   string
	system       string
}

func defaultTracingConfig() tracingConfig {
	return tracingConfig{
		spanNameFunc: func(msg messaging.Message) string {
			return fmt.Sprintf("%s consume", msg.Topic)
		},
		tracerName: "messaging.consumer",
		system:     "messaging",
	}
}

// WithSpanNameFunc overrides the default span naming strategy.
func WithSpanNameFunc(fn func(messaging.Message) string) TracingOption {
	return func(c *tracingConfig) {
		c.spanNameFunc = fn
	}
}

// WithTracerName overrides the tracer name used for consumer spans.
func WithTracerName(name string) TracingOption {
	return func(c *tracingConfig) {
		if name != "" {
			c.tracerName = name
		}
	}
}

// WithMessagingSystem sets the messaging.system span attribute.
func WithMessagingSystem(system string) TracingOption {
	return func(c *tracingConfig) {
		if system != "" {
			c.system = system
		}
	}
}

// TracingHandler wraps a MessageHandler with OpenTelemetry distributed tracing.
// It extracts trace context from incoming message headers, creates a consumer span,
// and annotates it with messaging-specific attributes.
func TracingHandler(handler messaging.MessageHandler, opts ...TracingOption) messaging.MessageHandler {
	cfg := defaultTracingConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(ctx context.Context, msg messaging.Message) error {
		ctx = ExtractTraceContext(ctx, msg.Headers)

		spanName := cfg.spanNameFunc(msg)
		attrs := []observability.SpanAttribute{
			observability.StringAttribute("messaging.system", cfg.system),
			observability.StringAttribute("messaging.destination", msg.Topic),
		}
		if cfg.system == "kafka" {
			attrs = append(attrs,
				observability.IntAttribute("messaging.kafka.partition", msg.Partition),
				observability.StringAttribute("messaging.kafka.message.key", msg.Key),
			)
		}

		ctx, span := observability.StartNamedSpan(ctx, cfg.tracerName, spanName,
			observability.WithSpanKind(observability.SpanKindConsumer),
			observability.WithSpanAttributes(attrs...),
		)
		defer span.End()

		err := handler(ctx, msg)
		if err != nil {
			span.RecordError(err)
			span.SetError(err.Error())
		}

		return err
	}
}
