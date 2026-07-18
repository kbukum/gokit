package vectorstore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"sync"
)

type storedPoint struct {
	ID      string
	Vector  []float32
	Payload *PointPayload
}

type collection struct {
	Dimensions int
	Metric     string
	Points     []*storedPoint
}

// InMemoryStore is an in-memory vector store implementation backed by a simple slice.
// It performs linear scan search using the configured similarity metric. Intended for unit tests
// and prototyping — not suitable for production workloads. Thread-safe via sync.RWMutex.
type InMemoryStore struct {
	mu          sync.RWMutex
	collections map[string]*collection
	metric      string
}

// NewInMemoryStore creates a new empty in-memory vector store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		collections: make(map[string]*collection),
		metric:      DefaultMetric,
	}
}

// NewInMemoryStoreWithConfig creates an in-memory vector store with config.
func NewInMemoryStoreWithConfig(cfg Config) (*InMemoryStore, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &InMemoryStore{
		collections: make(map[string]*collection),
		metric:      cfg.Metric,
	}, nil
}

// EnsureCollection ensures a collection exists, creating it if necessary.
func (s *InMemoryStore) EnsureCollection(ctx context.Context, collectionName string, dimensions int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.collections[collectionName]; !exists {
		s.collections[collectionName] = &collection{
			Dimensions: dimensions,
			Metric:     s.metric,
			Points:     make([]*storedPoint, 0),
		}
	}

	return nil
}

// Upsert inserts or updates a vector point.
func (s *InMemoryStore) Upsert(ctx context.Context, collectionName string, point Point) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	col, exists := s.collections[collectionName]
	if !exists {
		return fmt.Errorf("collection %q does not exist", collectionName)
	}

	if len(point.Vector) != col.Dimensions {
		return fmt.Errorf("vector dimensions mismatch: expected %d, got %d", col.Dimensions, len(point.Vector))
	}

	// Update existing or insert new
	for _, existing := range col.Points {
		if existing.ID == point.ID {
			existing.Vector = point.Vector
			existing.Payload = point.Payload
			return nil
		}
	}

	col.Points = append(col.Points, &storedPoint{
		ID:      point.ID,
		Vector:  point.Vector,
		Payload: point.Payload,
	})

	return nil
}

// Search searches for similar vectors using the collection's similarity metric.
func (s *InMemoryStore) Search(ctx context.Context, collectionName string, query SearchQuery) ([]SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	col, exists := s.collections[collectionName]
	if !exists {
		return nil, fmt.Errorf("collection %q does not exist", collectionName)
	}
	if len(query.Vector) != col.Dimensions {
		return nil, fmt.Errorf("query vector dimensions mismatch: expected %d, got %d", col.Dimensions, len(query.Vector))
	}

	var results []SearchResult

	for _, point := range col.Points {
		// Apply filter if provided
		if query.Filter != nil && !matchesFilter(point.Payload, query.Filter) {
			continue
		}

		score, err := similarity(col.Metric, query.Vector, point.Vector)
		if err != nil {
			return nil, err
		}

		results = append(results, SearchResult{
			ID:      point.ID,
			Score:   score,
			Payload: point.Payload,
		})
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Truncate to limit
	if query.Limit < len(results) {
		results = results[:query.Limit]
	}

	return results, nil
}

// Delete deletes a point by ID.
func (s *InMemoryStore) Delete(ctx context.Context, collectionName, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	col, exists := s.collections[collectionName]
	if !exists {
		return fmt.Errorf("collection %q does not exist", collectionName)
	}

	// Remove point with matching ID
	newPoints := make([]*storedPoint, 0, len(col.Points))
	for _, point := range col.Points {
		if point.ID != id {
			newPoints = append(newPoints, point)
		}
	}
	col.Points = newPoints

	return nil
}

func similarity(metric string, a, b []float32) (float32, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("vector length mismatch: %d != %d", len(a), len(b))
	}
	switch metric {
	case MetricCosine:
		return cosineSimilarity(a, b), nil
	case MetricDot:
		return dotProduct(a, b), nil
	case MetricL2:
		return -l2Distance(a, b), nil
	default:
		return 0, &MetricError{Metric: metric}
	}
}

func cosineSimilarity(a, b []float32) float32 {
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	normA = float32(math.Sqrt(float64(normA)))
	normB = float32(math.Sqrt(float64(normB)))

	if normA == 0.0 || normB == 0.0 {
		return 0.0
	}

	return dot / (normA * normB)
}

func dotProduct(a, b []float32) float32 {
	var dot float32
	for i := range a {
		dot += a[i] * b[i]
	}
	return dot
}

func l2Distance(a, b []float32) float32 {
	var sum float32
	for i := range a {
		d := a[i] - b[i]
		sum += d * d
	}
	return float32(math.Sqrt(float64(sum)))
}

// matchesFilter checks if a payload matches all must conditions.
func matchesFilter(payload *PointPayload, filter *SearchFilter) bool {
	if payload == nil {
		return false
	}

	for _, condition := range filter.Must {
		actual, exists := payload.Fields[condition.Field]
		if !exists || !valueEquals(actual, condition.Value) {
			return false
		}
	}

	return true
}

// valueEquals compares two payload values,
// falling back to JSON for values whose dynamic type is not directly comparable (slices, maps).
func valueEquals(a, b any) bool {
	if isComparable(a) && isComparable(b) && a == b {
		return true
	}

	aJSON, err1 := json.Marshal(a)
	bJSON, err2 := json.Marshal(b)
	if err1 == nil && err2 == nil {
		return bytes.Equal(aJSON, bJSON)
	}

	return false
}

func isComparable(v any) bool {
	if v == nil {
		return true
	}
	return reflect.TypeOf(v).Comparable()
}
