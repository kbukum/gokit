package bench

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// --- Timing Middleware ---

// TimingMiddleware wraps an evaluator and records per-sample execution times.
type TimingMiddleware[L comparable] struct {
	inner   Evaluator[L]
	mu      sync.Mutex
	timings map[string]time.Duration // sampleID → duration
}

// WithTiming wraps an evaluator with timing instrumentation.
func WithTiming[L comparable](eval Evaluator[L]) *TimingMiddleware[L] {
	return &TimingMiddleware[L]{
		inner:   eval,
		timings: make(map[string]time.Duration),
	}
}

func (m *TimingMiddleware[L]) Name() string                         { return m.inner.Name() }
func (m *TimingMiddleware[L]) IsAvailable(ctx context.Context) bool { return m.inner.IsAvailable(ctx) }

func (m *TimingMiddleware[L]) Execute(ctx context.Context, input []byte) (Prediction[L], error) {
	start := time.Now()
	pred, err := m.inner.Execute(ctx, input)
	elapsed := time.Since(start)

	// Use the prediction's SampleID as key; fall back to input hash.
	key := pred.SampleID
	if key == "" {
		key = hashBytes(input)
	}

	m.mu.Lock()
	m.timings[key] = elapsed
	m.mu.Unlock()

	return pred, err
}

// Timings returns a copy of the recorded per-sample durations.
func (m *TimingMiddleware[L]) Timings() map[string]time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]time.Duration, len(m.timings))
	for k, v := range m.timings {
		out[k] = v
	}
	return out
}

// --- Caching Middleware ---

// CachingMiddleware wraps an evaluator and caches results by input hash.
type CachingMiddleware[L comparable] struct {
	inner  Evaluator[L]
	mu     sync.RWMutex
	cache  map[string]Prediction[L]
	hits   int
	misses int
}

// WithCaching wraps an evaluator with SHA-256 input-based caching.
func WithCaching[L comparable](eval Evaluator[L]) *CachingMiddleware[L] {
	return &CachingMiddleware[L]{
		inner: eval,
		cache: make(map[string]Prediction[L]),
	}
}

func (m *CachingMiddleware[L]) Name() string                         { return m.inner.Name() }
func (m *CachingMiddleware[L]) IsAvailable(ctx context.Context) bool { return m.inner.IsAvailable(ctx) }

func (m *CachingMiddleware[L]) Execute(ctx context.Context, input []byte) (Prediction[L], error) {
	key := hashBytes(input)

	// Fast path: check cache with read lock.
	m.mu.RLock()
	if pred, ok := m.cache[key]; ok {
		m.mu.RUnlock()
		m.mu.Lock()
		m.hits++
		m.mu.Unlock()
		return pred, nil
	}
	m.mu.RUnlock()

	// Cache miss: execute and store.
	pred, err := m.inner.Execute(ctx, input)
	if err != nil {
		m.mu.Lock()
		m.misses++
		m.mu.Unlock()
		return pred, err
	}

	m.mu.Lock()
	m.misses++
	m.cache[key] = pred
	m.mu.Unlock()

	return pred, nil
}

// Stats returns the number of cache hits and misses.
func (m *CachingMiddleware[L]) Stats() (hits, misses int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.hits, m.misses
}

// hashBytes returns the hex-encoded SHA-256 digest of data.
func hashBytes(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
