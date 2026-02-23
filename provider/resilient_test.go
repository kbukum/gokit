package provider_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	goerrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/provider"
	"github.com/kbukum/gokit/resilience"
)

// --- Test helpers ---

var errTransient = errors.New("transient failure")

type failingProvider struct {
	name      string
	callCount atomic.Int32
	failUntil int32 // fail the first N calls
}

func (p *failingProvider) Name() string                       { return p.name }
func (p *failingProvider) IsAvailable(_ context.Context) bool { return true }
func (p *failingProvider) Execute(_ context.Context, in string) (string, error) {
	n := p.callCount.Add(1)
	if n <= p.failUntil {
		return "", errTransient
	}
	return "ok:" + in, nil
}

type alwaysFailProvider struct{ name string }

func (p *alwaysFailProvider) Name() string                       { return p.name }
func (p *alwaysFailProvider) IsAvailable(_ context.Context) bool { return true }
func (p *alwaysFailProvider) Execute(_ context.Context, _ string) (string, error) {
	return "", errTransient
}

// --- Sink test helper ---

type failingSink struct {
	callCount atomic.Int32
	failUntil int32
	sent      []string
}

func (s *failingSink) Name() string                       { return "fail-sink" }
func (s *failingSink) IsAvailable(_ context.Context) bool { return true }
func (s *failingSink) Send(_ context.Context, in string) error {
	n := s.callCount.Add(1)
	if n <= s.failUntil {
		return errTransient
	}
	s.sent = append(s.sent, in)
	return nil
}

// --- Stream test helper ---

type failingStream struct {
	callCount atomic.Int32
	failUntil int32
}

func (s *failingStream) Name() string                       { return "fail-stream" }
func (s *failingStream) IsAvailable(_ context.Context) bool { return true }
func (s *failingStream) Execute(_ context.Context, in string) (provider.Iterator[byte], error) {
	n := s.callCount.Add(1)
	if n <= s.failUntil {
		return nil, errTransient
	}
	items := make([]byte, len(in))
	for i := range in {
		items[i] = in[i]
	}
	return &sliceIterator[byte]{items: items}, nil
}

// --- Tests: Empty config passthrough ---

func TestWithResilience_EmptyConfig(t *testing.T) {
	p := &echoProvider{name: "passthrough"}
	wrapped := provider.WithResilience[string, string](p, provider.ResilienceConfig{})
	// Should return the same instance
	if wrapped.Name() != "passthrough" {
		t.Fatalf("expected same provider, got %s", wrapped.Name())
	}
	result, err := wrapped.Execute(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "echo:test" {
		t.Fatalf("expected echo:test, got %s", result)
	}
}

// --- Tests: Retry ---

func TestWithResilience_RetryRecoversTransient(t *testing.T) {
	p := &failingProvider{name: "retry-test", failUntil: 2}
	wrapped := provider.WithResilience[string, string](p, provider.ResilienceConfig{
		Retry: &resilience.RetryConfig{
			MaxAttempts:    3,
			InitialBackoff: time.Millisecond,
			MaxBackoff:     10 * time.Millisecond,
			BackoffFactor:  1.0,
		},
	})

	result, err := wrapped.Execute(context.Background(), "hello")
	if err != nil {
		t.Fatalf("expected retry to recover, got error: %v", err)
	}
	if result != "ok:hello" {
		t.Fatalf("expected ok:hello, got %s", result)
	}
	if p.callCount.Load() != 3 {
		t.Fatalf("expected 3 calls (2 fail + 1 success), got %d", p.callCount.Load())
	}
}

func TestWithResilience_RetryExhausted(t *testing.T) {
	p := &failingProvider{name: "exhaust-test", failUntil: 10}
	wrapped := provider.WithResilience[string, string](p, provider.ResilienceConfig{
		Retry: &resilience.RetryConfig{
			MaxAttempts:    3,
			InitialBackoff: time.Millisecond,
			MaxBackoff:     10 * time.Millisecond,
			BackoffFactor:  1.0,
		},
	})

	_, err := wrapped.Execute(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error after retry exhaustion")
	}
	if p.callCount.Load() != 3 {
		t.Fatalf("expected 3 attempts, got %d", p.callCount.Load())
	}
}

// --- Tests: Circuit Breaker ---

func TestWithResilience_CircuitBreakerTrips(t *testing.T) {
	p := &alwaysFailProvider{name: "cb-test"}
	wrapped := provider.WithResilience[string, string](p, provider.ResilienceConfig{
		CircuitBreaker: &resilience.CircuitBreakerConfig{
			Name:             "test-cb",
			MaxFailures:      3,
			Timeout:          time.Second,
			HalfOpenMaxCalls: 1,
		},
	})

	// Fail 3 times to trip CB
	for i := 0; i < 3; i++ {
		_, err := wrapped.Execute(context.Background(), "x")
		if !errors.Is(err, errTransient) {
			t.Fatalf("call %d: expected transient error, got %v", i, err)
		}
	}

	// Next call should be rejected by CB (circuit open) — wrapped as AppError
	_, err := wrapped.Execute(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error when circuit is open")
	}
	// Should be wrapped as AppError with SERVICE_UNAVAILABLE code
	appErr, ok := goerrors.AsAppError(err)
	if !ok {
		t.Fatalf("expected AppError, got %T: %v", err, err)
	}
	if appErr.Code != goerrors.ErrCodeServiceUnavailable {
		t.Fatalf("expected SERVICE_UNAVAILABLE code, got %s", appErr.Code)
	}
	// Original sentinel should be preserved via Cause chain
	if !errors.Is(err, resilience.ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen in cause chain, got %v", err)
	}
}

// --- Tests: CB + Retry combined ---

func TestWithResilience_CBAndRetry(t *testing.T) {
	p := &failingProvider{name: "cb-retry", failUntil: 1}
	wrapped := provider.WithResilience[string, string](p, provider.ResilienceConfig{
		CircuitBreaker: &resilience.CircuitBreakerConfig{
			Name:        "test-cb-retry",
			MaxFailures: 5,
			Timeout:     time.Second,
		},
		Retry: &resilience.RetryConfig{
			MaxAttempts:    3,
			InitialBackoff: time.Millisecond,
			BackoffFactor:  1.0,
		},
	})

	// Should succeed: first call fails, retry succeeds
	result, err := wrapped.Execute(context.Background(), "hi")
	if err != nil {
		t.Fatalf("expected success with retry, got: %v", err)
	}
	if result != "ok:hi" {
		t.Fatalf("expected ok:hi, got %s", result)
	}
}

// --- Tests: RateLimiter ---

func TestWithResilience_RateLimiter(t *testing.T) {
	p := &echoProvider{name: "rl-test"}
	wrapped := provider.WithResilience[string, string](p, provider.ResilienceConfig{
		RateLimiter: &resilience.RateLimiterConfig{
			Name:  "test-rl",
			Rate:  1000, // high rate so test doesn't block
			Burst: 10,
		},
	})

	result, err := wrapped.Execute(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "echo:test" {
		t.Fatalf("expected echo:test, got %s", result)
	}
}

// --- Tests: Bulkhead ---

func TestWithResilience_Bulkhead(t *testing.T) {
	p := &echoProvider{name: "bh-test"}
	wrapped := provider.WithResilience[string, string](p, provider.ResilienceConfig{
		Bulkhead: &resilience.BulkheadConfig{
			Name:          "test-bh",
			MaxConcurrent: 2,
			MaxWait:       0, // fail immediately if full
		},
	})

	result, err := wrapped.Execute(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "echo:test" {
		t.Fatalf("expected echo:test, got %s", result)
	}
}

// --- Tests: Name and IsAvailable delegation ---

func TestWithResilience_DelegatesNameAndAvailability(t *testing.T) {
	p := &echoProvider{name: "delegated"}
	wrapped := provider.WithResilience[string, string](p, provider.ResilienceConfig{
		CircuitBreaker: &resilience.CircuitBreakerConfig{
			Name:        "test",
			MaxFailures: 5,
			Timeout:     time.Second,
		},
	})

	if wrapped.Name() != "delegated" {
		t.Fatalf("expected name delegated, got %s", wrapped.Name())
	}
	if !wrapped.IsAvailable(context.Background()) {
		t.Fatal("expected provider to be available")
	}
}

// --- Tests: Stream resilience ---

func TestWithStreamResilience_RetryRecovers(t *testing.T) {
	p := &failingStream{failUntil: 1}
	wrapped := provider.WithStreamResilience[string, byte](p, provider.ResilienceConfig{
		Retry: &resilience.RetryConfig{
			MaxAttempts:    3,
			InitialBackoff: time.Millisecond,
			BackoffFactor:  1.0,
		},
	})

	iter, err := wrapped.Execute(context.Background(), "ab")
	if err != nil {
		t.Fatalf("expected retry to recover, got: %v", err)
	}
	defer iter.Close()

	var result []byte
	for {
		v, more, err := iter.Next(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !more {
			break
		}
		result = append(result, v)
	}
	if string(result) != "ab" {
		t.Fatalf("expected ab, got %s", string(result))
	}
}

// --- Tests: Sink resilience ---

func TestWithSinkResilience_RetryRecovers(t *testing.T) {
	s := &failingSink{failUntil: 1}
	wrapped := provider.WithSinkResilience[string](s, provider.ResilienceConfig{
		Retry: &resilience.RetryConfig{
			MaxAttempts:    3,
			InitialBackoff: time.Millisecond,
			BackoffFactor:  1.0,
		},
	})

	err := wrapped.Send(context.Background(), "msg")
	if err != nil {
		t.Fatalf("expected retry to recover, got: %v", err)
	}
	if len(s.sent) != 1 || s.sent[0] != "msg" {
		t.Fatalf("expected [msg], got %v", s.sent)
	}
}

// --- Tests: Manager with resilience ---

func TestManagerInitializeWithResilience(t *testing.T) {
	registry := provider.NewRegistry[provider.RequestResponse[string, string]]()
	selector := &provider.HealthCheckSelector[provider.RequestResponse[string, string]]{}
	mgr := provider.NewManager(registry, selector)

	callCount := 0
	registry.RegisterFactory("test", func(_ map[string]any) (provider.RequestResponse[string, string], error) {
		return &echoProvider{name: "test"}, nil
	})

	err := mgr.InitializeWithResilience(context.Background(), "test", nil, func(p provider.RequestResponse[string, string]) provider.RequestResponse[string, string] {
		callCount++
		return provider.WithResilience(p, provider.ResilienceConfig{
			CircuitBreaker: &resilience.CircuitBreakerConfig{
				Name:        "mgr-test",
				MaxFailures: 5,
				Timeout:     time.Second,
			},
		})
	})
	if err != nil {
		t.Fatalf("initialize error: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("expected wrap to be called once, got %d", callCount)
	}

	p, err := mgr.Get(context.Background())
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	result, err := p.Execute(context.Background(), "hello")
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if result != "echo:hello" {
		t.Fatalf("expected echo:hello, got %s", result)
	}
}
