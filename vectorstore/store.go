package vectorstore

import (
	"context"
)

// PointPayload represents the metadata stored alongside each vector point.
type PointPayload struct {
	Fields map[string]interface{} `json:"fields"`
}

// NewPointPayload creates a new empty PointPayload.
func NewPointPayload() *PointPayload {
	return &PointPayload{
		Fields: make(map[string]interface{}),
	}
}

// WithField adds a field to the payload and returns the payload for chaining.
func (p *PointPayload) WithField(key string, value interface{}) *PointPayload {
	p.Fields[key] = value
	return p
}

// SearchResult represents a single result from a vector search.
type SearchResult struct {
	ID      string       `json:"id"`
	Score   float32      `json:"score"`
	Payload *PointPayload `json:"payload"`
}

// SearchFilter represents optional filters for search queries.
type SearchFilter struct {
	Must []struct {
		Field string
		Value interface{}
	}
}

// NewSearchFilter creates a new empty SearchFilter.
func NewSearchFilter() *SearchFilter {
	return &SearchFilter{
		Must: []struct {
			Field string
			Value interface{}
		}{},
	}
}

// MustMatch adds a must-match condition to the filter.
func (f *SearchFilter) MustMatch(field string, value interface{}) *SearchFilter {
	f.Must = append(f.Must, struct {
		Field string
		Value interface{}
	}{Field: field, Value: value})
	return f
}

// Store is the interface for vector similarity search stores.
type Store interface {
	// EnsureCollection ensures a collection exists, creating it if necessary.
	EnsureCollection(ctx context.Context, collection string, dimensions int) error

	// Upsert inserts or updates a vector point.
	Upsert(ctx context.Context, collection, id string, vector []float32, payload *PointPayload) error

	// Search searches for similar vectors.
	Search(ctx context.Context, collection string, vector []float32, limit int, filter *SearchFilter) ([]SearchResult, error)

	// Delete deletes a point by ID.
	Delete(ctx context.Context, collection, id string) error
}
