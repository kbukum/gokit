package provider

import (
	"context"
	"fmt"
	"time"
)

// Meta holds open-ended metadata annotations for a provider.
// Keys are strings, values are any type. Common dimensions include
// "cost", "latency_ms", "reliability", "requires", but any project
// can define whatever dimensions matter to it.
type Meta map[string]any

// Float returns the value for key as a float64.
func (m Meta) Float(key string) (float64, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

// String returns the value for key as a string.
func (m Meta) String(key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// Duration returns the value for key as a time.Duration.
// Accepts time.Duration directly or numeric values interpreted as milliseconds.
func (m Meta) Duration(key string) (time.Duration, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch d := v.(type) {
	case time.Duration:
		return d, true
	case float64:
		return time.Duration(d * float64(time.Millisecond)), true
	case int:
		return time.Duration(d) * time.Millisecond, true
	case int64:
		return time.Duration(d) * time.Millisecond, true
	default:
		return 0, false
	}
}

// Bool returns the value for key as a bool.
func (m Meta) Bool(key string) (val bool, ok bool) {
	v, ok := m[key]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

// Has returns true if the key exists in the metadata.
func (m Meta) Has(key string) bool {
	_, ok := m[key]
	return ok
}

// Merge returns a new Meta combining m with other. Values in other take precedence.
func (m Meta) Merge(other Meta) Meta {
	result := make(Meta, len(m)+len(other))
	for k, v := range m {
		result[k] = v
	}
	for k, v := range other {
		result[k] = v
	}
	return result
}

// MetaProvider is the interface implemented by providers wrapped with WithMeta.
type MetaProvider interface {
	Meta() Meta
}

// WithMeta wraps a RequestResponse provider with metadata annotations.
// The metadata does not affect execution behavior — it is informational,
// consumed by DAG ordering strategies and observability.
func WithMeta[I, O any](p RequestResponse[I, O], m Meta) RequestResponse[I, O] {
	return &metaRR[I, O]{
		inner: p,
		meta:  m,
	}
}

// GetMeta retrieves metadata from a wrapped provider.
// Returns an empty Meta if the provider has no metadata.
func GetMeta[I, O any](p RequestResponse[I, O]) Meta {
	if mp, ok := p.(MetaProvider); ok {
		return mp.Meta()
	}
	return Meta{}
}

// GetMetaFromAny retrieves metadata from any value that might be a MetaProvider.
func GetMetaFromAny(p any) Meta {
	if mp, ok := p.(MetaProvider); ok {
		return mp.Meta()
	}
	return Meta{}
}

type metaRR[I, O any] struct {
	inner RequestResponse[I, O]
	meta  Meta
}

func (m *metaRR[I, O]) Name() string {
	return m.inner.Name()
}

func (m *metaRR[I, O]) IsAvailable(ctx context.Context) bool {
	return m.inner.IsAvailable(ctx)
}

func (m *metaRR[I, O]) Execute(ctx context.Context, input I) (O, error) {
	return m.inner.Execute(ctx, input)
}

func (m *metaRR[I, O]) Meta() Meta {
	return m.meta
}

func (m *metaRR[I, O]) String() string {
	return fmt.Sprintf("MetaProvider(%s)", m.inner.Name())
}

// WithSinkMeta wraps a Sink provider with metadata annotations.
func WithSinkMeta[I any](s Sink[I], m Meta) Sink[I] {
	return &metaSink[I]{
		inner: s,
		meta:  m,
	}
}

type metaSink[I any] struct {
	inner Sink[I]
	meta  Meta
}

func (m *metaSink[I]) Name() string                         { return m.inner.Name() }
func (m *metaSink[I]) IsAvailable(ctx context.Context) bool { return m.inner.IsAvailable(ctx) }
func (m *metaSink[I]) Send(ctx context.Context, input I) error {
	return m.inner.Send(ctx, input)
}
func (m *metaSink[I]) Meta() Meta { return m.meta }

// WithStreamMeta wraps a Stream provider with metadata annotations.
func WithStreamMeta[I, O any](s Stream[I, O], m Meta) Stream[I, O] {
	return &metaStream[I, O]{
		inner: s,
		meta:  m,
	}
}

type metaStream[I, O any] struct {
	inner Stream[I, O]
	meta  Meta
}

func (m *metaStream[I, O]) Name() string                         { return m.inner.Name() }
func (m *metaStream[I, O]) IsAvailable(ctx context.Context) bool { return m.inner.IsAvailable(ctx) }
func (m *metaStream[I, O]) Execute(ctx context.Context, input I) (Iterator[O], error) {
	return m.inner.Execute(ctx, input)
}
func (m *metaStream[I, O]) Meta() Meta { return m.meta }
