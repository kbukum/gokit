package observability

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel/propagation"
)

// TextMapCarrier is the transport-neutral carrier used for trace propagation.
type TextMapCarrier interface {
	Get(key string) string
	Set(key, value string)
	Keys() []string
}

// MapCarrier adapts map-backed message headers for trace propagation.
type MapCarrier map[string]string

// Get returns the carrier value for key.
func (c MapCarrier) Get(key string) string { return c[key] }

// Set stores value under key.
func (c MapCarrier) Set(key, value string) { c[key] = value }

// Keys returns all carrier keys.
func (c MapCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// HeaderCarrier adapts HTTP headers for trace propagation.
type HeaderCarrier http.Header

// Get returns the carrier value for key.
func (c HeaderCarrier) Get(key string) string { return http.Header(c).Get(key) }

// Set stores value under key.
func (c HeaderCarrier) Set(key, value string) { http.Header(c).Set(key, value) }

// Keys returns all carrier keys.
func (c HeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// InjectTraceContext writes W3C trace context into a transport carrier without global state or baggage.
func InjectTraceContext(ctx context.Context, carrier TextMapCarrier) {
	propagation.TraceContext{}.Inject(ctx, carrier)
}

// ExtractTraceContext reads W3C trace context from a trusted transport carrier without global state or baggage. The caller must authenticate the peer before accepting its parent.
func ExtractTraceContext(ctx context.Context, carrier TextMapCarrier) context.Context {
	return propagation.TraceContext{}.Extract(ctx, carrier)
}
