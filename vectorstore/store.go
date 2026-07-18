package vectorstore

import (
	"context"
	"fmt"
)

const (
	// ProviderMemory is the lean in-process vectorstore backend.
	ProviderMemory = "memory"

	// MetricCosine ranks by cosine similarity.
	MetricCosine = "cosine"
	// MetricDot ranks by dot product.
	MetricDot = "dot"
	// MetricL2 ranks by negative Euclidean distance, so higher scores are better.
	MetricL2 = "l2"

	DefaultProvider = ProviderMemory
	DefaultMetric   = MetricCosine
)

// Config holds provider-agnostic vectorstore configuration.
type Config struct {
	Name     string `mapstructure:"name" json:"name" yaml:"name"`
	Provider string `mapstructure:"provider" json:"provider" yaml:"provider"`
	Metric   string `mapstructure:"metric" json:"metric" yaml:"metric"`
}

// ApplyDefaults fills zero-valued fields.
func (c *Config) ApplyDefaults() {
	if c.Provider == "" {
		c.Provider = DefaultProvider
	}
	if c.Metric == "" {
		c.Metric = DefaultMetric
	}
}

// Validate checks provider-agnostic settings.
func (c *Config) Validate() error {
	switch c.Metric {
	case MetricCosine, MetricDot, MetricL2:
		return nil
	default:
		return &MetricError{Metric: c.Metric}
	}
}

// MetricError reports an unsupported similarity metric.
type MetricError struct {
	Metric string
}

func (e *MetricError) Error() string {
	return fmt.Sprintf("vectorstore: unsupported metric %q (supported: %s, %s, %s)", e.Metric, MetricCosine, MetricDot, MetricL2)
}

// PointPayload represents the metadata stored alongside each vector point.
type PointPayload struct {
	Fields map[string]any `json:"fields"`
}

// NewPointPayload creates a new empty PointPayload.
func NewPointPayload() *PointPayload {
	return &PointPayload{
		Fields: make(map[string]any),
	}
}

// WithField adds a field to the payload and returns the payload for chaining.
func (p *PointPayload) WithField(key string, value any) *PointPayload {
	p.Fields[key] = value
	return p
}

// Point is a vector point to insert or update in a collection.
type Point struct {
	ID      string
	Vector  []float32
	Payload *PointPayload
}

// SearchQuery describes a vector similarity search.
type SearchQuery struct {
	Vector []float32
	Limit  int
	Filter *SearchFilter
}

// SearchResult represents a single result from a vector search.
type SearchResult struct {
	ID      string        `json:"id"`
	Score   float32       `json:"score"`
	Payload *PointPayload `json:"payload"`
}

// SearchFilter represents optional filters for search queries.
type SearchFilter struct {
	Must []struct {
		Field string
		Value any
	}
}

// NewSearchFilter creates a new empty SearchFilter.
func NewSearchFilter() *SearchFilter {
	return &SearchFilter{
		Must: []struct {
			Field string
			Value any
		}{},
	}
}

// MustMatch adds a must-match condition to the filter.
func (f *SearchFilter) MustMatch(field string, value any) *SearchFilter {
	f.Must = append(f.Must, struct {
		Field string
		Value any
	}{Field: field, Value: value})
	return f
}

// Store is the interface for vector similarity search stores.
type Store interface {
	// EnsureCollection ensures a collection exists, creating it if necessary.
	EnsureCollection(ctx context.Context, collection string, dimensions int) error

	// Upsert inserts or updates a vector point.
	Upsert(ctx context.Context, collection string, point Point) error

	// Search searches for similar vectors.
	Search(ctx context.Context, collection string, query SearchQuery) ([]SearchResult, error)

	// Delete deletes a point by ID.
	Delete(ctx context.Context, collection, id string) error
}
