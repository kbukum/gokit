package vectorstore

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
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
	Points     []*storedPoint
}

// InMemoryStore is an in-memory vector store implementation backed by a simple slice.
// It performs linear scan search using cosine similarity.
// Intended for unit tests and prototyping — not suitable for production workloads.
// Thread-safe via sync.RWMutex.
type InMemoryStore struct {
	mu          sync.RWMutex
	collections map[string]*collection
}

// NewInMemoryStore creates a new empty in-memory vector store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		collections: make(map[string]*collection),
	}
}

// EnsureCollection ensures a collection exists, creating it if necessary.
func (s *InMemoryStore) EnsureCollection(ctx context.Context, collectionName string, dimensions int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.collections[collectionName]; !exists {
		s.collections[collectionName] = &collection{
			Dimensions: dimensions,
			Points:     make([]*storedPoint, 0),
		}
	}

	return nil
}

// Upsert inserts or updates a vector point.
func (s *InMemoryStore) Upsert(ctx context.Context, collectionName, id string, vector []float32, payload *PointPayload) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	col, exists := s.collections[collectionName]
	if !exists {
		return fmt.Errorf("collection %q does not exist", collectionName)
	}

	if len(vector) != col.Dimensions {
		return fmt.Errorf("vector dimensions mismatch: expected %d, got %d", col.Dimensions, len(vector))
	}

	// Update existing or insert new
	for _, point := range col.Points {
		if point.ID == id {
			point.Vector = vector
			point.Payload = payload
			return nil
		}
	}

	col.Points = append(col.Points, &storedPoint{
		ID:      id,
		Vector:  vector,
		Payload: payload,
	})

	return nil
}

// Search searches for similar vectors using cosine similarity.
func (s *InMemoryStore) Search(ctx context.Context, collectionName string, vector []float32, limit int, filter *SearchFilter) ([]SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	col, exists := s.collections[collectionName]
	if !exists {
		return nil, fmt.Errorf("collection %q does not exist", collectionName)
	}

	var results []SearchResult

	for _, point := range col.Points {
		// Apply filter if provided
		if filter != nil && !matchesFilter(point.Payload, filter) {
			continue
		}

		// Compute cosine similarity
		score, err := cosineSimilarity(vector, point.Vector)
		if err != nil {
			continue
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
	if limit < len(results) {
		results = results[:limit]
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

// cosineSimilarity computes cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) (float32, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("vector length mismatch: %d != %d", len(a), len(b))
	}

	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	normA = float32(math.Sqrt(float64(normA)))
	normB = float32(math.Sqrt(float64(normB)))

	if normA == 0.0 || normB == 0.0 {
		return 0.0, nil
	}

	return dot / (normA * normB), nil
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

// valueEquals compares two values for equality.
func valueEquals(a, b interface{}) bool {
	if a == b {
		return true
	}

	// Try JSON marshaling for deep comparison
	aJSON, err1 := json.Marshal(a)
	bJSON, err2 := json.Marshal(b)

	if err1 == nil && err2 == nil {
		return string(aJSON) == string(bJSON)
	}

	return false
}
