package tool

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/kbukum/gokit/schema"
)

// --- WithRetry middleware ---

// RetryConfig controls retry behavior.
type RetryConfig struct {
	// MaxAttempts is the total number of attempts (including the initial call).
	// Defaults to 3 if zero.
	MaxAttempts int
	// BaseDelay is the initial delay before the first retry.
	// Defaults to 100ms if zero.
	BaseDelay time.Duration
	// MaxDelay caps the backoff delay.
	// Defaults to 5s if zero.
	MaxDelay time.Duration
	// ShouldRetry determines if an error is retryable.
	// If nil, all errors are retried.
	ShouldRetry func(err error) bool
}

func (c *RetryConfig) applyDefaults() {
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 3
	}
	if c.BaseDelay == 0 {
		c.BaseDelay = 100 * time.Millisecond
	}
	if c.MaxDelay == 0 {
		c.MaxDelay = 5 * time.Second
	}
}

// WithRetry returns middleware that retries failed tool calls with
// exponential backoff and jitter.
func WithRetry(cfg RetryConfig) Middleware {
	cfg.applyDefaults()
	return func(next Callable) Callable {
		return &retryCallable{next: next, cfg: cfg}
	}
}

type retryCallable struct {
	next Callable
	cfg  RetryConfig
}

func (r *retryCallable) Definition() Definition { return r.next.Definition() }

func (r *retryCallable) Validate(input json.RawMessage) schema.ValidationResult {
	return r.next.Validate(input)
}

func (r *retryCallable) Call(ctx *Context, input json.RawMessage) (*Result, error) {
	var lastErr error

	for attempt := 1; attempt <= r.cfg.MaxAttempts; attempt++ {
		result, err := r.next.Call(ctx, input)
		if err == nil {
			if attempt > 1 && result != nil {
				result.SetMeta("retry_attempts", attempt)
			}
			return result, nil
		}

		lastErr = err

		if r.cfg.ShouldRetry != nil && !r.cfg.ShouldRetry(err) {
			return nil, err
		}

		if attempt < r.cfg.MaxAttempts {
			if err := ctx.Err(); err != nil {
				return nil, fmt.Errorf("tool %q: context cancelled during retry: %w",
					r.next.Definition().Name, err)
			}
			delay := r.backoffDelay(attempt)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}

	return nil, fmt.Errorf("tool %q: all %d attempts failed: %w",
		r.next.Definition().Name, r.cfg.MaxAttempts, lastErr)
}

// backoffDelay computes exponential backoff with full jitter.
func (r *retryCallable) backoffDelay(attempt int) time.Duration {
	base := float64(r.cfg.BaseDelay) * math.Pow(2, float64(attempt-1))
	if base > float64(r.cfg.MaxDelay) {
		base = float64(r.cfg.MaxDelay)
	}
	// Full jitter: uniform random in [0, base)
	jittered := time.Duration(rand.Float64() * base)
	return jittered
}

// --- WithMetrics middleware ---

// MetricsCollector gathers tool execution metrics.
// Implementations can export to Prometheus, OpenTelemetry, etc.
type MetricsCollector interface {
	// RecordCall records a completed tool call.
	RecordCall(toolName string, duration time.Duration, err error)
}

// WithMetrics returns middleware that records call count, latency, and
// error count per tool name.
func WithMetrics(collector MetricsCollector) Middleware {
	return func(next Callable) Callable {
		return &metricsCallable{next: next, collector: collector}
	}
}

type metricsCallable struct {
	next      Callable
	collector MetricsCollector
}

func (m *metricsCallable) Definition() Definition { return m.next.Definition() }

func (m *metricsCallable) Validate(input json.RawMessage) schema.ValidationResult {
	return m.next.Validate(input)
}

func (m *metricsCallable) Call(ctx *Context, input json.RawMessage) (*Result, error) {
	start := time.Now()
	result, err := m.next.Call(ctx, input)
	duration := time.Since(start)
	m.collector.RecordCall(m.next.Definition().Name, duration, err)
	return result, err
}

// InMemoryMetrics is a simple in-memory metrics collector for testing.
type InMemoryMetrics struct {
	mu      sync.Mutex
	entries []MetricEntry
}

// MetricEntry records a single tool call metric.
type MetricEntry struct {
	ToolName string
	Duration time.Duration
	Err      error
}

// RecordCall implements MetricsCollector.
func (m *InMemoryMetrics) RecordCall(toolName string, duration time.Duration, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, MetricEntry{
		ToolName: toolName,
		Duration: duration,
		Err:      err,
	})
}

// Entries returns a copy of all recorded metrics.
func (m *InMemoryMetrics) Entries() []MetricEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]MetricEntry, len(m.entries))
	copy(cp, m.entries)
	return cp
}

// CallCount returns total calls for the given tool name.
func (m *InMemoryMetrics) CallCount(toolName string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, e := range m.entries {
		if e.ToolName == toolName {
			count++
		}
	}
	return count
}

// ErrorCount returns total error calls for the given tool name.
func (m *InMemoryMetrics) ErrorCount(toolName string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, e := range m.entries {
		if e.ToolName == toolName && e.Err != nil {
			count++
		}
	}
	return count
}
