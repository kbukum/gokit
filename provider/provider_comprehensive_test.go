package provider_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	goerrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logger"
	"github.com/kbukum/gokit/observability"
	"github.com/kbukum/gokit/provider"
	"github.com/kbukum/gokit/resilience"
)

// ============================================================================
// Shared test helpers (unique to this file)
// ============================================================================

// countingIterator counts items and can return an error on Close.
type countingIterator[T any] struct {
	items    []T
	pos      int
	closeErr error
}

func (it *countingIterator[T]) Next(_ context.Context) (val T, ok bool, err error) {
	if it.pos >= len(it.items) {
		var zero T
		return zero, false, nil
	}
	v := it.items[it.pos]
	it.pos++
	return v, true, nil
}

func (it *countingIterator[T]) Close() error { return it.closeErr }

// errorAtNIterator returns an error after N successful items.
type errorAtNIterator struct {
	items []int
	pos   int
	errAt int
}

func (it *errorAtNIterator) Next(_ context.Context) (val int, ok bool, err error) {
	if it.pos >= len(it.items) {
		return 0, false, nil
	}
	if it.pos == it.errAt {
		return 0, false, errors.New("iterator error")
	}
	v := it.items[it.pos]
	it.pos++
	return v, true, nil
}

func (it *errorAtNIterator) Close() error { return nil }

// blockingIterator blocks until context is canceled, useful for concurrency tests.
type blockingIterator struct {
	items    []int
	pos      int
	blockAt  int
	unblock  chan struct{}
	closeMu  sync.Mutex
	closed   bool
	closeErr error
}

func (it *blockingIterator) Next(ctx context.Context) (val int, ok bool, err error) {
	if it.pos >= len(it.items) {
		return 0, false, nil
	}
	if it.pos == it.blockAt {
		select {
		case <-it.unblock:
		case <-ctx.Done():
			return 0, false, ctx.Err()
		}
	}
	v := it.items[it.pos]
	it.pos++
	return v, true, nil
}

func (it *blockingIterator) Close() error {
	it.closeMu.Lock()
	defer it.closeMu.Unlock()
	it.closed = true
	return it.closeErr
}

// controlledDuplex is a Duplex provider that can simulate errors.
type controlledDuplex struct {
	name      string
	available bool
	openErr   error
	stream    *controlledDuplexStream
}

func (d *controlledDuplex) Name() string                       { return d.name }
func (d *controlledDuplex) IsAvailable(_ context.Context) bool { return d.available }
func (d *controlledDuplex) Open(_ context.Context) (provider.DuplexStream[string, string], error) {
	if d.openErr != nil {
		return nil, d.openErr
	}
	return d.stream, nil
}

type controlledDuplexStream struct {
	mu       sync.Mutex
	sendErr  error
	recvCh   chan string
	closed   bool
	closeErr error
}

func newControlledDuplexStream() *controlledDuplexStream {
	return &controlledDuplexStream{
		recvCh: make(chan string, 10),
	}
}

func (s *controlledDuplexStream) Send(in string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("stream closed")
	}
	if s.sendErr != nil {
		return s.sendErr
	}
	s.recvCh <- "echo:" + in
	return nil
}

func (s *controlledDuplexStream) Recv() (string, error) {
	v, ok := <-s.recvCh
	if !ok {
		return "", io.EOF
	}
	return v, nil
}

func (s *controlledDuplexStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		close(s.recvCh)
	}
	return s.closeErr
}

// initErrProvider is Initializable but returns an error from Init.
type initErrProvider struct {
	name string
}

func (p *initErrProvider) Name() string                                         { return p.name }
func (p *initErrProvider) IsAvailable(_ context.Context) bool                   { return true }
func (p *initErrProvider) Execute(_ context.Context, in string) (string, error) { return in, nil }
func (p *initErrProvider) Init(_ context.Context) error                         { return errors.New("init failed") }

// nonCloseableProvider does NOT implement Closeable.
type nonCloseableProvider struct {
	name string
}

func (p *nonCloseableProvider) Name() string                                         { return p.name }
func (p *nonCloseableProvider) IsAvailable(_ context.Context) bool                   { return true }
func (p *nonCloseableProvider) Execute(_ context.Context, in string) (string, error) { return in, nil }

// closeErrProvider implements Closeable but returns an error from Close.
type closeErrProvider struct {
	name string
}

func (p *closeErrProvider) Name() string                                         { return p.name }
func (p *closeErrProvider) IsAvailable(_ context.Context) bool                   { return true }
func (p *closeErrProvider) Execute(_ context.Context, in string) (string, error) { return in, nil }
func (p *closeErrProvider) Close(_ context.Context) error                        { return errors.New("close error") }

// healthCheckProvider implements HealthChecker.
type healthCheckProvider struct {
	name   string
	status provider.HealthStatus
}

func (p *healthCheckProvider) Name() string { return p.name }
func (p *healthCheckProvider) IsAvailable(_ context.Context) bool {
	return p.status.Status == provider.StatusHealthy
}

func (p *healthCheckProvider) Execute(_ context.Context, in string) (string, error) {
	return in, nil
}

func (p *healthCheckProvider) Health(_ context.Context) provider.HealthStatus {
	return p.status
}

// ============================================================================
// GAP 1: Stream + Resilience
// ============================================================================

func TestWithStreamResilience_CircuitBreakerTrips(t *testing.T) {
	t.Parallel()
	callCount := atomic.Int32{}
	stream := &streamTestHelper[string, int]{
		name: "cb-stream",
		fn: func(_ context.Context, _ string) (provider.Iterator[int], error) {
			callCount.Add(1)
			return nil, errors.New("stream open failed")
		},
	}

	wrapped := provider.WithStreamResilience[string, int](stream, provider.ResilienceConfig{
		CircuitBreaker: &resilience.CircuitBreakerConfig{
			Name:             "stream-cb",
			MaxFailures:      2,
			Timeout:          time.Second,
			HalfOpenMaxCalls: 1,
		},
	})

	// Trip the circuit breaker
	for i := 0; i < 2; i++ {
		_, err := wrapped.Execute(context.Background(), "input")
		if err == nil {
			t.Fatal("expected error")
		}
	}

	// Next call should be rejected by CB
	_, err := wrapped.Execute(context.Background(), "input")
	if err == nil {
		t.Fatal("expected circuit breaker error")
	}
	appErr, ok := goerrors.AsAppError(err)
	if !ok {
		t.Fatalf("expected AppError, got %T: %v", err, err)
	}
	if appErr.Code != goerrors.ErrCodeServiceUnavailable {
		t.Fatalf("expected SERVICE_UNAVAILABLE, got %s", appErr.Code)
	}
}

func TestWithStreamResilience_RateLimiterDuringIteration(t *testing.T) {
	t.Parallel()
	stream := &streamTestHelper[string, int]{
		name: "rl-stream",
		fn: func(_ context.Context, _ string) (provider.Iterator[int], error) {
			return newSliceIter(1, 2, 3), nil
		},
	}

	wrapped := provider.WithStreamResilience[string, int](stream, provider.ResilienceConfig{
		RateLimiter: &resilience.RateLimiterConfig{
			Name:  "stream-rl",
			Rate:  1000,
			Burst: 10,
		},
	})

	iter, err := wrapped.Execute(context.Background(), "input")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	defer iter.Close()

	var results []int
	for {
		v, ok, err := iter.Next(context.Background())
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		if !ok {
			break
		}
		results = append(results, v)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 items, got %d", len(results))
	}
}

func TestWithStreamResilience_ErrorPropagation(t *testing.T) {
	t.Parallel()
	stream := &streamTestHelper[string, int]{
		name: "err-stream",
		fn: func(_ context.Context, _ string) (provider.Iterator[int], error) {
			return &errorAtNIterator{items: []int{1, 2, 3}, errAt: 1}, nil
		},
	}

	wrapped := provider.WithStreamResilience[string, int](stream, provider.ResilienceConfig{
		RateLimiter: &resilience.RateLimiterConfig{
			Name:  "err-rl",
			Rate:  1000,
			Burst: 10,
		},
	})

	iter, err := wrapped.Execute(context.Background(), "input")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	defer iter.Close()

	// First item should succeed
	v, ok, err := iter.Next(context.Background())
	if err != nil || !ok || v != 1 {
		t.Fatalf("first Next: v=%d, ok=%v, err=%v", v, ok, err)
	}

	// Second should error
	_, _, err = iter.Next(context.Background())
	if err == nil {
		t.Fatal("expected error from iterator")
	}
}

func TestWithStreamResilience_EmptyConfig(t *testing.T) {
	t.Parallel()
	stream := &streamTestHelper[string, int]{
		name: "empty-cfg",
		fn: func(_ context.Context, _ string) (provider.Iterator[int], error) {
			return newSliceIter(1), nil
		},
	}

	wrapped := provider.WithStreamResilience[string, int](stream, provider.ResilienceConfig{})
	if wrapped.Name() != "empty-cfg" {
		t.Fatalf("expected passthrough, got %s", wrapped.Name())
	}
}

// ============================================================================
// GAP 2: Duplex Resilience
// ============================================================================

func TestWithDuplexResilience_CircuitBreakerOnOpen(t *testing.T) {
	t.Parallel()
	openErr := errors.New("connection failed")
	duplex := &controlledDuplex{
		name:      "cb-duplex",
		available: true,
		openErr:   openErr,
	}

	wrapped := provider.WithDuplexResilience[string, string](duplex, provider.ResilienceConfig{
		CircuitBreaker: &resilience.CircuitBreakerConfig{
			Name:             "duplex-cb",
			MaxFailures:      2,
			Timeout:          time.Second,
			HalfOpenMaxCalls: 1,
		},
	})

	// Trip the circuit breaker
	for i := 0; i < 2; i++ {
		_, err := wrapped.Open(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
	}

	// Next call should be rejected by CB
	_, err := wrapped.Open(context.Background())
	if err == nil {
		t.Fatal("expected circuit breaker error")
	}
	if !errors.Is(err, resilience.ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen in cause chain, got %v", err)
	}
}

func TestWithDuplexResilience_ConcurrentSendRecv(t *testing.T) {
	t.Parallel()
	stream := newControlledDuplexStream()
	duplex := &controlledDuplex{
		name:      "concurrent-duplex",
		available: true,
		stream:    stream,
	}

	wrapped := provider.WithDuplexResilience[string, string](duplex, provider.ResilienceConfig{
		RateLimiter: &resilience.RateLimiterConfig{
			Name:  "duplex-rl",
			Rate:  10000,
			Burst: 100,
		},
	})

	ds, err := wrapped.Open(context.Background())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	const n = 5
	var wg sync.WaitGroup

	// Send messages concurrently
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := ds.Send(fmt.Sprintf("msg%d", i)); err != nil {
				t.Errorf("Send %d: %v", i, err)
			}
		}(i)
	}

	// Receive messages concurrently
	received := make([]string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v, err := ds.Recv()
			if err != nil {
				t.Errorf("Recv %d: %v", i, err)
				return
			}
			received[i] = v
		}(i)
	}

	wg.Wait()

	// Verify all received
	for i, v := range received {
		if !strings.HasPrefix(v, "echo:msg") {
			t.Errorf("received[%d] = %q, expected echo:msg*", i, v)
		}
	}

	if err := ds.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestWithDuplexResilience_ClosePropagation(t *testing.T) {
	t.Parallel()
	stream := newControlledDuplexStream()
	stream.closeErr = errors.New("close error")
	duplex := &controlledDuplex{
		name:      "close-duplex",
		available: true,
		stream:    stream,
	}

	wrapped := provider.WithDuplexResilience[string, string](duplex, provider.ResilienceConfig{
		RateLimiter: &resilience.RateLimiterConfig{
			Name:  "duplex-close-rl",
			Rate:  10000,
			Burst: 100,
		},
	})

	ds, err := wrapped.Open(context.Background())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Close should propagate through the resilience wrapper
	err = ds.Close()
	if err == nil {
		t.Fatal("expected close error to propagate")
	}
	if err.Error() != "close error" {
		t.Fatalf("expected 'close error', got %q", err.Error())
	}
}

func TestWithDuplexResilience_EmptyConfig(t *testing.T) {
	t.Parallel()
	stream := newControlledDuplexStream()
	duplex := &controlledDuplex{
		name:      "passthrough-duplex",
		available: true,
		stream:    stream,
	}

	wrapped := provider.WithDuplexResilience[string, string](duplex, provider.ResilienceConfig{})
	if wrapped.Name() != "passthrough-duplex" {
		t.Fatalf("expected passthrough, got %s", wrapped.Name())
	}
}

func TestWithDuplexResilience_NameAndIsAvailable(t *testing.T) {
	t.Parallel()
	duplex := &controlledDuplex{
		name:      "delegate-duplex",
		available: true,
	}

	wrapped := provider.WithDuplexResilience[string, string](duplex, provider.ResilienceConfig{
		CircuitBreaker: &resilience.CircuitBreakerConfig{
			Name:        "delegate-cb",
			MaxFailures: 5,
			Timeout:     time.Second,
		},
	})

	if wrapped.Name() != "delegate-duplex" {
		t.Fatalf("expected delegate-duplex, got %s", wrapped.Name())
	}
	if !wrapped.IsAvailable(context.Background()) {
		t.Fatal("expected available")
	}
}

// ============================================================================
// GAP 3: Complex Composition
// ============================================================================

func TestChain_ThreePlusMiddlewares_WithCustom(t *testing.T) {
	t.Parallel()
	var order []string

	customMW := func(inner provider.RequestResponse[string, string]) provider.RequestResponse[string, string] {
		return &orderTracker[string, string]{inner: inner, tag: "custom", order: &order}
	}

	log := logger.NewDefault("test")
	meter := observability.Meter("test")
	metrics, err := observability.NewMetrics(meter)
	if err != nil {
		t.Fatalf("failed to create metrics: %v", err)
	}

	p := &echoProvider{name: "composed"}
	wrapped := provider.Chain(
		provider.WithLogging[string, string](log),
		provider.WithMetrics[string, string](metrics),
		provider.WithTracing[string, string]("test-svc"),
		provider.Middleware[string, string](customMW),
	)(p)

	result, err := wrapped.Execute(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "echo:hello" {
		t.Fatalf("expected echo:hello, got %q", result)
	}

	// Verify custom middleware was executed (among others)
	hasCustomBefore, hasCustomAfter := false, false
	for _, entry := range order {
		if entry == "custom:before" {
			hasCustomBefore = true
		}
		if entry == "custom:after" {
			hasCustomAfter = true
		}
	}
	if !hasCustomBefore || !hasCustomAfter {
		t.Fatalf("custom middleware not executed, order=%v", order)
	}
}

func TestChain_WithResilienceWrapper(t *testing.T) {
	t.Parallel()
	p := &echoProvider{name: "mw-res"}
	log := logger.NewDefault("test")

	chained := provider.Chain(
		provider.WithLogging[string, string](log),
	)(p)

	resilient := provider.WithResilience(chained, provider.ResilienceConfig{
		CircuitBreaker: &resilience.CircuitBreakerConfig{
			Name:        "mw-res-cb",
			MaxFailures: 5,
			Timeout:     time.Second,
		},
	})

	result, err := resilient.Execute(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "echo:test" {
		t.Fatalf("expected echo:test, got %q", result)
	}
}

func TestStateful_WithResilience_Combination(t *testing.T) {
	t.Parallel()
	store := provider.NewMemoryStore[chatState]()
	inner := &chatProvider{}

	stateful := provider.NewStateful(provider.StatefulConfig[chatRequest, chatResponse, chatState]{
		Inner:   inner,
		Store:   store,
		KeyFunc: func(req chatRequest) string { return req.SessionID },
		Inject:  func(req chatRequest, _ *chatState) chatRequest { return req },
		Extract: func(_ chatRequest, resp chatResponse) *chatState {
			return &chatState{Messages: resp.History}
		},
		TTL: time.Minute,
	})

	resilient := provider.WithResilience[chatRequest, chatResponse](stateful, provider.ResilienceConfig{
		CircuitBreaker: &resilience.CircuitBreakerConfig{
			Name:        "stateful-cb",
			MaxFailures: 5,
			Timeout:     time.Second,
		},
		RateLimiter: &resilience.RateLimiterConfig{
			Name:  "stateful-rl",
			Rate:  10000,
			Burst: 100,
		},
	})

	resp, err := resilient.Execute(context.Background(), chatRequest{SessionID: "s1", Message: "hi"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if resp.Reply != "echo:hi" {
		t.Fatalf("expected echo:hi, got %q", resp.Reply)
	}

	// Verify state was saved through the combined pipeline
	state, _ := store.Load(context.Background(), "s1")
	if state == nil || len(state.Messages) != 1 {
		t.Fatalf("expected state to be saved, got %v", state)
	}
}

func TestAdapt_Middleware_Resilience_Pipeline(t *testing.T) {
	t.Parallel()
	backend := &echoProvider{name: "backend"}

	// Adapt string→string to string→string with transformation
	adapted := provider.Adapt[string, string, string, string](
		backend,
		"adapted",
		func(_ context.Context, in string) (string, error) {
			return "transformed:" + in, nil
		},
		func(out string) (string, error) {
			return "mapped:" + out, nil
		},
	)

	// Add middleware
	log := logger.NewDefault("test")
	chained := provider.Chain(
		provider.WithLogging[string, string](log),
	)(adapted)

	// Add resilience
	resilient := provider.WithResilience(chained, provider.ResilienceConfig{
		CircuitBreaker: &resilience.CircuitBreakerConfig{
			Name:        "pipeline-cb",
			MaxFailures: 5,
			Timeout:     time.Second,
		},
	})

	result, err := resilient.Execute(context.Background(), "input")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "mapped:echo:transformed:input" {
		t.Fatalf("expected 'mapped:echo:transformed:input', got %q", result)
	}
}

// ============================================================================
// GAP 4: Manager Lifecycle Edge Cases
// ============================================================================

func TestManager_InitializeWithResilience_FactoryError(t *testing.T) {
	t.Parallel()
	registry := provider.NewRegistry[provider.RequestResponse[string, string]]()
	selector := &provider.HealthCheckSelector[provider.RequestResponse[string, string]]{}
	mgr := provider.NewManager(registry, selector)

	registry.RegisterFactory("fail-factory", func(_ map[string]any) (provider.RequestResponse[string, string], error) {
		return nil, errors.New("factory error")
	})

	err := mgr.InitializeWithResilience(context.Background(), "fail-factory", nil, func(p provider.RequestResponse[string, string]) provider.RequestResponse[string, string] {
		t.Fatal("wrapper should not be called when factory fails")
		return p
	})
	if err == nil {
		t.Fatal("expected factory error")
	}
	if !strings.Contains(err.Error(), "factory error") {
		t.Fatalf("expected factory error in message, got %q", err.Error())
	}
}

func TestManager_InitializeWithContext_CancellationMidInit(t *testing.T) {
	t.Parallel()
	registry := provider.NewRegistry[provider.RequestResponse[string, string]]()
	selector := &provider.HealthCheckSelector[provider.RequestResponse[string, string]]{}
	mgr := provider.NewManager(registry, selector)

	ctx, cancel := context.WithCancel(context.Background())

	registry.RegisterFactory("slow-init", func(_ map[string]any) (provider.RequestResponse[string, string], error) {
		return &slowInitProvider{name: "slow", initDelay: 500 * time.Millisecond}, nil
	})

	// Cancel context during init
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := mgr.InitializeWithContext(ctx, "slow-init", nil)
	if err == nil {
		t.Fatal("expected error from canceled context during init")
	}
}

type slowInitProvider struct {
	name      string
	initDelay time.Duration
}

func (p *slowInitProvider) Name() string                       { return p.name }
func (p *slowInitProvider) IsAvailable(_ context.Context) bool { return true }
func (p *slowInitProvider) Execute(_ context.Context, in string) (string, error) {
	return in, nil
}

func (p *slowInitProvider) Init(ctx context.Context) error {
	select {
	case <-time.After(p.initDelay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestManager_CloseAll_MixedCloseableNonCloseable(t *testing.T) {
	t.Parallel()
	registry := provider.NewRegistry[provider.RequestResponse[string, string]]()
	selector := &provider.HealthCheckSelector[provider.RequestResponse[string, string]]{}
	mgr := provider.NewManager(registry, selector)

	closeCalled := false
	registry.RegisterFactory("closeable", func(_ map[string]any) (provider.RequestResponse[string, string], error) {
		return &closeTrackingProvider{name: "closeable", closeCalled: &closeCalled}, nil
	})
	registry.RegisterFactory("non-closeable", func(_ map[string]any) (provider.RequestResponse[string, string], error) {
		return &nonCloseableProvider{name: "non-closeable"}, nil
	})

	mgr.Initialize("closeable", nil)
	mgr.Initialize("non-closeable", nil)

	err := mgr.CloseAll(context.Background())
	if err != nil {
		t.Fatalf("CloseAll should succeed: %v", err)
	}
	if !closeCalled {
		t.Fatal("expected Close to be called on closeable provider")
	}
}

type closeTrackingProvider struct {
	name        string
	closeCalled *bool
}

func (p *closeTrackingProvider) Name() string                                         { return p.name }
func (p *closeTrackingProvider) IsAvailable(_ context.Context) bool                   { return true }
func (p *closeTrackingProvider) Execute(_ context.Context, in string) (string, error) { return in, nil }
func (p *closeTrackingProvider) Close(_ context.Context) error {
	*p.closeCalled = true
	return nil
}

func TestManager_SetDefault_NonExistent(t *testing.T) {
	t.Parallel()
	registry := provider.NewRegistry[provider.RequestResponse[string, string]]()
	selector := &provider.HealthCheckSelector[provider.RequestResponse[string, string]]{}
	mgr := provider.NewManager(registry, selector)

	err := mgr.SetDefault("nonexistent")
	if err == nil {
		t.Fatal("expected error setting default to non-existent provider")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("expected 'not initialized' error, got %q", err.Error())
	}
}

func TestManager_Get_NoDefaultNoProviders(t *testing.T) {
	t.Parallel()
	registry := provider.NewRegistry[provider.RequestResponse[string, string]]()
	selector := &provider.HealthCheckSelector[provider.RequestResponse[string, string]]{}
	mgr := provider.NewManager(registry, selector)

	_, err := mgr.Get(context.Background())
	if err == nil {
		t.Fatal("expected error when no providers exist")
	}
}

func TestManager_CloseAll_WithCloseError(t *testing.T) {
	t.Parallel()
	registry := provider.NewRegistry[provider.RequestResponse[string, string]]()
	selector := &provider.HealthCheckSelector[provider.RequestResponse[string, string]]{}
	mgr := provider.NewManager(registry, selector)

	registry.RegisterFactory("close-err", func(_ map[string]any) (provider.RequestResponse[string, string], error) {
		return &closeErrProvider{name: "close-err"}, nil
	})

	mgr.Initialize("close-err", nil)

	err := mgr.CloseAll(context.Background())
	if err == nil {
		t.Fatal("expected error from CloseAll when a provider Close fails")
	}
	if !strings.Contains(err.Error(), "close error") {
		t.Fatalf("expected 'close error' in message, got %q", err.Error())
	}
}

func TestManager_InitializeWithResilience_InitError(t *testing.T) {
	t.Parallel()
	registry := provider.NewRegistry[provider.RequestResponse[string, string]]()
	selector := &provider.HealthCheckSelector[provider.RequestResponse[string, string]]{}
	mgr := provider.NewManager(registry, selector)

	registry.RegisterFactory("init-err", func(_ map[string]any) (provider.RequestResponse[string, string], error) {
		return &initErrProvider{name: "init-err"}, nil
	})

	err := mgr.InitializeWithResilience(context.Background(), "init-err", nil, func(p provider.RequestResponse[string, string]) provider.RequestResponse[string, string] {
		t.Fatal("wrapper should not be called when init fails")
		return p
	})
	if err == nil {
		t.Fatal("expected init error")
	}
	if !strings.Contains(err.Error(), "init failed") {
		t.Fatalf("expected 'init failed' error, got %q", err.Error())
	}
}

// ============================================================================
// GAP 5: DrainIterator Edge Cases
// ============================================================================

func TestDrainIterator_ExactlyWindowSizeItems(t *testing.T) {
	t.Parallel()
	inner := newSliceIter(1, 2, 3)
	drain := provider.DrainIterator(inner, 3)

	// Read all items
	var results []int
	for {
		v, ok, err := drain.Next(context.Background())
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		if !ok {
			break
		}
		results = append(results, v)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 items, got %d", len(results))
	}

	if err := drain.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	type drainGetter interface {
		Drained() []int
	}
	drained := drain.(drainGetter).Drained()
	if len(drained) != 0 {
		t.Fatalf("expected 0 drained items (all read), got %d", len(drained))
	}
}

func TestDrainIterator_MaxDrainZero(t *testing.T) {
	t.Parallel()
	inner := newSliceIter(1, 2, 3)
	drain := provider.DrainIterator(inner, 0)

	// Don't read any, close immediately with maxDrain=0
	if err := drain.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	type drainGetter interface {
		Drained() []int
	}
	drained := drain.(drainGetter).Drained()
	if len(drained) != 0 {
		t.Fatalf("expected 0 drained items (maxDrain=0), got %d", len(drained))
	}
}

func TestDrainIterator_CloseErrorPropagation(t *testing.T) {
	t.Parallel()
	closeErr := errors.New("iterator close error")
	inner := &countingIterator[int]{items: []int{1, 2, 3}, closeErr: closeErr}
	drain := provider.DrainIterator[int](inner, 10)

	err := drain.Close()
	if err == nil {
		t.Fatal("expected close error to propagate")
	}
	if err.Error() != "iterator close error" {
		t.Fatalf("expected 'iterator close error', got %q", err.Error())
	}
}

func TestMergedIterator_ErrorFromSource(t *testing.T) {
	t.Parallel()
	s1 := &streamTestHelper[string, int]{
		name: "s1-ok",
		fn: func(_ context.Context, _ string) (provider.Iterator[int], error) {
			return newSliceIter(1, 2), nil
		},
	}
	s2 := &streamTestHelper[string, int]{
		name: "s2-err",
		fn: func(_ context.Context, _ string) (provider.Iterator[int], error) {
			return &errorAtNIterator{items: []int{10, 20, 30}, errAt: 0}, nil
		},
	}

	fan := provider.FanOutStream("fan", s1, s2)
	iter, err := fan.Execute(context.Background(), "x")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	defer iter.Close()

	// Read all available items; should eventually get an error
	sawError := false
	for i := 0; i < 10; i++ {
		_, _, err := iter.Next(context.Background())
		if err != nil {
			sawError = true
			break
		}
	}
	if !sawError {
		t.Fatal("expected error from merged iterator")
	}
}

// ============================================================================
// GAP 6: Meta Provider (Stream, Sink, Duplex)
// ============================================================================

func TestWithStreamMeta_ExecuteAndMeta(t *testing.T) {
	t.Parallel()
	stream := &streamTestHelper[string, int]{
		name: "meta-stream",
		fn: func(_ context.Context, _ string) (provider.Iterator[int], error) {
			return newSliceIter(1, 2, 3), nil
		},
	}

	wrapped := provider.WithStreamMeta[string, int](stream, provider.Meta{"latency_ms": 50.0, "cost": 0.1})

	if wrapped.Name() != "meta-stream" {
		t.Fatalf("expected meta-stream, got %s", wrapped.Name())
	}
	if !wrapped.IsAvailable(context.Background()) {
		t.Fatal("expected available")
	}

	// Execute should work normally
	iter, err := wrapped.Execute(context.Background(), "input")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	defer iter.Close()

	var results []int
	for {
		v, ok, err := iter.Next(context.Background())
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		if !ok {
			break
		}
		results = append(results, v)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 items, got %d", len(results))
	}

	// Verify meta
	mp, ok := wrapped.(provider.MetaProvider)
	if !ok {
		t.Fatal("expected MetaProvider interface")
	}
	lat, ok := mp.Meta().Float("latency_ms")
	if !ok || lat != 50.0 {
		t.Fatalf("expected latency_ms=50, got %v", lat)
	}
}

func TestWithSinkMeta_SendAndMeta(t *testing.T) {
	t.Parallel()
	var received []string
	sink := provider.NewSinkFunc("meta-sink", func(_ context.Context, s string) error {
		received = append(received, s)
		return nil
	})

	wrapped := provider.WithSinkMeta[string](sink, provider.Meta{"cost": 0.5})

	if err := wrapped.Send(context.Background(), "hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(received) != 1 || received[0] != "hello" {
		t.Fatalf("expected [hello], got %v", received)
	}

	mp, ok := wrapped.(provider.MetaProvider)
	if !ok {
		t.Fatal("expected MetaProvider interface")
	}
	cost, ok := mp.Meta().Float("cost")
	if !ok || cost != 0.5 {
		t.Fatalf("expected cost=0.5, got %v", cost)
	}
}

func TestMetaProvider_InterfaceSatisfaction(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		provider any
	}{
		{
			name:     "metaRR",
			provider: provider.WithMeta[string, string](&echoProvider{name: "rr"}, provider.Meta{"x": 1}),
		},
		{
			name: "metaSink",
			provider: provider.WithSinkMeta[string](
				provider.NewSinkFunc("sink", func(_ context.Context, _ string) error { return nil }),
				provider.Meta{"y": 2},
			),
		},
		{
			name: "metaStream",
			provider: provider.WithStreamMeta[string, int](
				&streamTestHelper[string, int]{
					name: "stream",
					fn: func(_ context.Context, _ string) (provider.Iterator[int], error) {
						return newSliceIter[int](), nil
					},
				},
				provider.Meta{"z": 3},
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mp, ok := tt.provider.(provider.MetaProvider)
			if !ok {
				t.Fatalf("%s should implement MetaProvider", tt.name)
			}
			if len(mp.Meta()) == 0 {
				t.Fatal("expected non-empty meta")
			}
		})
	}
}

func TestMeta_PropagationThroughMiddlewareChain(t *testing.T) {
	t.Parallel()
	inner := &echoProvider{name: "meta-chain"}
	meta := provider.Meta{"cost": 0.5, "tier": "premium"}
	wrapped := provider.WithMeta[string, string](inner, meta)

	// Wrap with middleware chain
	log := logger.NewDefault("test")
	chained := provider.Chain(
		provider.WithLogging[string, string](log),
	)(wrapped)

	// Execute should still work
	result, err := chained.Execute(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "echo:test" {
		t.Fatalf("expected echo:test, got %q", result)
	}

	// Meta should be retrievable from any provider (check via GetMetaFromAny)
	got := provider.GetMetaFromAny(wrapped)
	cost, ok := got.Float("cost")
	if !ok || cost != 0.5 {
		t.Fatalf("expected cost=0.5 from wrapped, got %v", cost)
	}
}

// ============================================================================
// GAP 7: HealthChecker
// ============================================================================

func TestHealthStatus_Transitions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status   provider.Status
		expected string
	}{
		{provider.StatusHealthy, "healthy"},
		{provider.StatusDegraded, "degraded"},
		{provider.StatusUnavailable, "unavailable"},
		{provider.Status(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			if got := tt.status.String(); got != tt.expected {
				t.Fatalf("Status(%d).String() = %q, want %q", tt.status, got, tt.expected)
			}
		})
	}
}

func TestHealthCheck_UnavailableProvider(t *testing.T) {
	t.Parallel()
	p := &healthCheckProvider{
		name: "unhealthy",
		status: provider.HealthStatus{
			Status:  provider.StatusUnavailable,
			Message: "database unreachable",
			Details: map[string]any{"error": "connection refused"},
		},
	}

	health := p.Health(context.Background())
	if health.Status != provider.StatusUnavailable {
		t.Fatalf("expected unavailable, got %v", health.Status)
	}
	if health.Message != "database unreachable" {
		t.Fatalf("expected 'database unreachable', got %q", health.Message)
	}
	if p.IsAvailable(context.Background()) {
		t.Fatal("unavailable provider should return false from IsAvailable")
	}
	if p.IsAvailable(context.Background()) {
		t.Fatal("unavailable provider should return false from IsAvailable")
	}
}

func TestHealthCheck_DegradedProvider(t *testing.T) {
	t.Parallel()
	p := &healthCheckProvider{
		name: "degraded",
		status: provider.HealthStatus{
			Status:  provider.StatusDegraded,
			Message: "high latency",
			Details: map[string]any{"latency_ms": 500},
		},
	}

	health := p.Health(context.Background())
	if health.Status != provider.StatusDegraded {
		t.Fatalf("expected degraded, got %v", health.Status)
	}
	if health.Details["latency_ms"] != 500 {
		t.Fatalf("expected latency_ms=500, got %v", health.Details["latency_ms"])
	}
}

func TestHealthCheck_WithTimeout(t *testing.T) {
	t.Parallel()
	p := &slowHealthProvider{
		name:  "slow-health",
		delay: 200 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	health := p.Health(ctx)
	// Should return unavailable due to timeout
	if health.Status != provider.StatusUnavailable {
		t.Fatalf("expected unavailable due to timeout, got %v", health.Status)
	}
}

type slowHealthProvider struct {
	name  string
	delay time.Duration
}

func (p *slowHealthProvider) Name() string                       { return p.name }
func (p *slowHealthProvider) IsAvailable(_ context.Context) bool { return true }
func (p *slowHealthProvider) Execute(_ context.Context, in string) (string, error) {
	return in, nil
}

func (p *slowHealthProvider) Health(ctx context.Context) provider.HealthStatus {
	select {
	case <-time.After(p.delay):
		return provider.HealthStatus{Status: provider.StatusHealthy, Message: "ok"}
	case <-ctx.Done():
		return provider.HealthStatus{Status: provider.StatusUnavailable, Message: "health check timed out"}
	}
}

// ============================================================================
// GAP 8: Connector Edge Cases
// ============================================================================

func TestConnector_WithResilienceConfig(t *testing.T) {
	t.Parallel()
	callCount := atomic.Int32{}
	c := provider.NewConnector(provider.ConnectorConfig[string]{
		ServiceName: "resilient-svc",
		Create: func() (string, error) {
			callCount.Add(1)
			return "client", nil
		},
		Resilience: &provider.ResilienceConfig{
			RateLimiter: &resilience.RateLimiterConfig{
				Name:  "conn-rl",
				Rate:  10000,
				Burst: 100,
			},
		},
	})

	result, err := provider.Call(context.Background(), c, func(client string) (string, error) {
		return "result-from-" + client, nil
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result != "result-from-client" {
		t.Fatalf("expected result-from-client, got %q", result)
	}
}

func TestConnector_ResetDuringActiveCall(t *testing.T) {
	t.Parallel()
	createCount := atomic.Int32{}
	c := provider.NewConnector(provider.ConnectorConfig[string]{
		ServiceName: "reset-active-svc",
		Create: func() (string, error) {
			n := createCount.Add(1)
			return fmt.Sprintf("client-v%d", n), nil
		},
	})

	// First call
	result1, err := provider.Call(context.Background(), c, func(client string) (string, error) {
		return client, nil
	})
	if err != nil {
		t.Fatalf("first Call: %v", err)
	}
	if result1 != "client-v1" {
		t.Fatalf("expected client-v1, got %q", result1)
	}

	// Reset
	_ = c.Reset()

	// Second call should re-create
	result2, err := provider.Call(context.Background(), c, func(client string) (string, error) {
		return client, nil
	})
	if err != nil {
		t.Fatalf("second Call: %v", err)
	}
	if result2 != "client-v2" {
		t.Fatalf("expected client-v2, got %q", result2)
	}
}

func TestConnector_ConcurrentCloseAndCall(t *testing.T) {
	t.Parallel()
	c := provider.NewConnector(provider.ConnectorConfig[string]{
		ServiceName: "concurrent-close-svc",
		Create: func() (string, error) {
			return "client", nil
		},
		OnClose: func() error {
			return nil
		},
	})

	// Initialize
	_, _ = c.GetClient()

	var wg sync.WaitGroup
	errCount := atomic.Int32{}

	// Run Close and Call concurrently
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = c.Close()
		}()
		go func() {
			defer wg.Done()
			_, err := provider.Call(context.Background(), c, func(client string) (string, error) {
				return client, nil
			})
			if err != nil {
				errCount.Add(1)
			}
		}()
	}

	wg.Wait()
	// No panics = success; some errors are expected since Close resets state
}

func TestConnector_WithCircuitBreakerResilience(t *testing.T) {
	t.Parallel()
	callCount := atomic.Int32{}
	c := provider.NewConnector(provider.ConnectorConfig[string]{
		ServiceName: "cb-conn-svc",
		Create: func() (string, error) {
			return "client", nil
		},
		Resilience: &provider.ResilienceConfig{
			CircuitBreaker: &resilience.CircuitBreakerConfig{
				Name:        "conn-cb",
				MaxFailures: 2,
				Timeout:     time.Second,
			},
		},
	})

	// Make the function fail to trip the CB
	for i := 0; i < 2; i++ {
		_, _ = provider.Call(context.Background(), c, func(client string) (string, error) {
			callCount.Add(1)
			return "", errors.New("call failed")
		})
	}

	// Next call should be rejected by CB
	_, err := provider.Call(context.Background(), c, func(client string) (string, error) {
		return "ok", nil
	})
	if err == nil {
		t.Fatal("expected circuit breaker error")
	}
	if !errors.Is(err, resilience.ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen, got %v", err)
	}
}

// ============================================================================
// GAP 9: Security & Edge Cases
// ============================================================================

func TestEmptyProviderName(t *testing.T) {
	t.Parallel()
	p := &echoProvider{name: ""}
	if p.Name() != "" {
		t.Fatalf("expected empty name, got %q", p.Name())
	}

	// Empty name should work with registry
	registry := provider.NewRegistry[provider.RequestResponse[string, string]]()
	registry.RegisterFactory("", func(_ map[string]any) (provider.RequestResponse[string, string], error) {
		return p, nil
	})
	created, err := registry.Create("", nil)
	if err != nil {
		t.Fatalf("Create with empty name: %v", err)
	}
	if created.Name() != "" {
		t.Fatalf("expected empty name, got %q", created.Name())
	}
}

func TestSinkFunc_NilFunction_Panics(t *testing.T) {
	t.Parallel()
	// Creating a SinkFunc with nil fn should panic when Send is called
	sink := provider.NewSinkFunc[string]("nil-fn", nil)
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when calling Send with nil function")
		}
	}()
	_ = sink.Send(context.Background(), "test")
}

func TestContextCancellation_RequestResponse(t *testing.T) {
	t.Parallel()
	callStarted := make(chan struct{})
	p := &rrTestHelper[string, string]{
		name: "blocking-rr",
		fn: func(ctx context.Context, in string) (string, error) {
			close(callStarted)
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(5 * time.Second):
				return "late", nil
			}
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := p.Execute(ctx, "test")
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
}

func TestContextCancellation_Stream(t *testing.T) {
	t.Parallel()
	stream := &streamTestHelper[string, int]{
		name: "blocking-stream",
		fn: func(_ context.Context, _ string) (provider.Iterator[int], error) {
			return &blockingIterator{
				items:   []int{1, 2, 3, 4, 5},
				blockAt: 1,
				unblock: make(chan struct{}), // never unblocked
			}, nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	iter, err := stream.Execute(ctx, "x")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	defer iter.Close()

	// First item should succeed
	_, ok, err := iter.Next(ctx)
	if err != nil || !ok {
		t.Fatalf("first Next: ok=%v, err=%v", ok, err)
	}

	// Second item should block and timeout
	_, _, err = iter.Next(ctx)
	if err == nil {
		t.Fatal("expected timeout error from blocked iterator")
	}
}

func TestContextCancellation_Sink(t *testing.T) {
	t.Parallel()
	sink := provider.NewSinkFunc("blocking-sink", func(ctx context.Context, _ string) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return nil
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := sink.Send(ctx, "test")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
}

func TestLargePayload_Adapt(t *testing.T) {
	t.Parallel()
	largeInput := strings.Repeat("x", 1<<16) // 64KB

	backend := &echoProvider{name: "large-backend"}
	adapted := provider.Adapt[string, string, string, string](
		backend,
		"large-adapted",
		func(_ context.Context, in string) (string, error) { return in, nil },
		func(out string) (string, error) { return out, nil },
	)

	result, err := adapted.Execute(context.Background(), largeInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "echo:"+largeInput {
		t.Fatalf("payload corrupted: expected len %d, got len %d", len("echo:")+len(largeInput), len(result))
	}
}

func TestLargePayload_FanOutSink(t *testing.T) {
	t.Parallel()
	largeInput := strings.Repeat("x", 1<<16)

	var mu sync.Mutex
	var received []string

	sink1 := provider.NewSinkFunc("s1", func(_ context.Context, s string) error {
		mu.Lock()
		received = append(received, s)
		mu.Unlock()
		return nil
	})
	sink2 := provider.NewSinkFunc("s2", func(_ context.Context, s string) error {
		mu.Lock()
		received = append(received, s)
		mu.Unlock()
		return nil
	})

	fan := provider.FanOutSink("fan", sink1, sink2)
	err := fan.Send(context.Background(), largeInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(received))
	}
	for _, r := range received {
		if len(r) != len(largeInput) {
			t.Fatalf("payload corrupted: expected len %d, got len %d", len(largeInput), len(r))
		}
	}
}

func TestIterator_ErrorOnClose(t *testing.T) {
	t.Parallel()
	closeErr := errors.New("close failed")
	iter := &countingIterator[int]{items: []int{1, 2}, closeErr: closeErr}

	// Read all items
	for {
		_, ok, err := iter.Next(context.Background())
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		if !ok {
			break
		}
	}

	err := iter.Close()
	if !errors.Is(err, closeErr) {
		t.Fatalf("expected close error, got %v", err)
	}
}

// ============================================================================
// Additional coverage: Sink resilience edge cases
// ============================================================================

func TestWithSinkResilience_CircuitBreakerTrips(t *testing.T) {
	t.Parallel()
	sink := provider.NewSinkFunc("cb-sink", func(_ context.Context, _ string) error {
		return errors.New("send failed")
	})

	wrapped := provider.WithSinkResilience[string](sink, provider.ResilienceConfig{
		CircuitBreaker: &resilience.CircuitBreakerConfig{
			Name:             "sink-cb",
			MaxFailures:      2,
			Timeout:          time.Second,
			HalfOpenMaxCalls: 1,
		},
	})

	// Trip the circuit breaker
	for i := 0; i < 2; i++ {
		_ = wrapped.Send(context.Background(), "msg")
	}

	// Next call should be rejected by CB
	err := wrapped.Send(context.Background(), "msg")
	if err == nil {
		t.Fatal("expected circuit breaker error")
	}
	appErr, ok := goerrors.AsAppError(err)
	if !ok {
		t.Fatalf("expected AppError, got %T: %v", err, err)
	}
	if appErr.Code != goerrors.ErrCodeServiceUnavailable {
		t.Fatalf("expected SERVICE_UNAVAILABLE, got %s", appErr.Code)
	}
}

func TestWithSinkResilience_EmptyConfig(t *testing.T) {
	t.Parallel()
	sink := provider.NewSinkFunc("passthrough-sink", func(_ context.Context, _ string) error {
		return nil
	})

	wrapped := provider.WithSinkResilience[string](sink, provider.ResilienceConfig{})
	if wrapped.Name() != "passthrough-sink" {
		t.Fatalf("expected passthrough, got %s", wrapped.Name())
	}
}

func TestWithSinkResilience_NameAndIsAvailable(t *testing.T) {
	t.Parallel()
	sink := provider.NewSinkFunc("delegate-sink", func(_ context.Context, _ string) error {
		return nil
	})

	wrapped := provider.WithSinkResilience[string](sink, provider.ResilienceConfig{
		CircuitBreaker: &resilience.CircuitBreakerConfig{
			Name:        "sink-delegate-cb",
			MaxFailures: 5,
			Timeout:     time.Second,
		},
	})

	if wrapped.Name() != "delegate-sink" {
		t.Fatalf("expected delegate-sink, got %s", wrapped.Name())
	}
	if !wrapped.IsAvailable(context.Background()) {
		t.Fatal("expected available")
	}
}

// ============================================================================
// Additional coverage: WindowedStream exact window size
// ============================================================================

func TestWindowedStream_ExactWindowSize(t *testing.T) {
	t.Parallel()
	inner := &streamTestHelper[string, int]{
		name: "source",
		fn: func(_ context.Context, _ string) (provider.Iterator[int], error) {
			return newSliceIter(1, 2, 3, 4), nil
		},
	}

	summer := &rrTestHelper[[]int, int]{
		name: "sum",
		fn: func(_ context.Context, batch []int) (int, error) {
			sum := 0
			for _, v := range batch {
				sum += v
			}
			return sum, nil
		},
	}

	// Window size = 4, exactly matches item count
	windowed := provider.WindowedStream[string, int, int]("exact", inner, 4, summer)
	iter, err := windowed.Execute(context.Background(), "x")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	defer iter.Close()

	v, ok, err := iter.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if !ok || v != 10 {
		t.Fatalf("expected 10, got %d (ok=%v)", v, ok)
	}

	// Second call should return no more items
	_, ok, err = iter.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if ok {
		t.Fatal("expected no more items")
	}
}

// ============================================================================
// Additional coverage: RateLimiter timeout
// ============================================================================

func TestWithResilience_RateLimiterTimeout(t *testing.T) {
	t.Parallel()
	p := &echoProvider{name: "rl-timeout"}
	wrapped := provider.WithResilience[string, string](p, provider.ResilienceConfig{
		RateLimiter: &resilience.RateLimiterConfig{
			Name:  "tight-rl",
			Rate:  0.001, // Very low rate
			Burst: 1,
		},
	})

	// First call should succeed (uses initial burst)
	_, err := wrapped.Execute(context.Background(), "first")
	if err != nil {
		t.Fatalf("first call should succeed: %v", err)
	}

	// Second call with short timeout should fail
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err = wrapped.Execute(ctx, "second")
	if err == nil {
		t.Fatal("expected rate limiter error with short timeout")
	}
}

// ============================================================================
// Additional coverage: Concurrent manager operations
// ============================================================================

func TestManager_ConcurrentGetAndInitialize(t *testing.T) {
	t.Parallel()
	registry := provider.NewRegistry[provider.RequestResponse[string, string]]()
	selector := &provider.RoundRobinSelector[provider.RequestResponse[string, string]]{}
	mgr := provider.NewManager(registry, selector)

	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("p%d", i)
		registry.RegisterFactory(name, func(_ map[string]any) (provider.RequestResponse[string, string], error) {
			return &echoProvider{name: name}, nil
		})
	}

	var wg sync.WaitGroup

	// Initialize concurrently
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = mgr.Initialize(fmt.Sprintf("p%d", i), nil)
		}(i)
	}
	wg.Wait()

	// Get concurrently
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = mgr.Get(context.Background())
		}()
	}
	wg.Wait()

	if len(mgr.Available()) != 5 {
		t.Fatalf("expected 5 providers, got %d", len(mgr.Available()))
	}
}

// ============================================================================
// Additional coverage: Context propagation through resilience
// ============================================================================

func TestContextCancellation_ThroughResilience(t *testing.T) {
	t.Parallel()
	p := &rrTestHelper[string, string]{
		name: "ctx-res",
		fn: func(ctx context.Context, _ string) (string, error) {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(5 * time.Second):
				return "late", nil
			}
		},
	}

	wrapped := provider.WithResilience[string, string](p, provider.ResilienceConfig{
		CircuitBreaker: &resilience.CircuitBreakerConfig{
			Name:        "ctx-cb",
			MaxFailures: 5,
			Timeout:     time.Second,
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := wrapped.Execute(ctx, "test")
	if err == nil {
		t.Fatal("expected context error")
	}
}

// ============================================================================
// Additional coverage: FanOutStream with multiple error sources
// ============================================================================

func TestFanOutStream_MultipleErrorSources(t *testing.T) {
	t.Parallel()
	s1 := &streamTestHelper[string, int]{
		name: "err-s1",
		fn: func(_ context.Context, _ string) (provider.Iterator[int], error) {
			return &errorAtNIterator{items: []int{1, 2, 3}, errAt: 1}, nil
		},
	}
	s2 := &streamTestHelper[string, int]{
		name: "err-s2",
		fn: func(_ context.Context, _ string) (provider.Iterator[int], error) {
			return &errorAtNIterator{items: []int{10, 20, 30}, errAt: 0}, nil
		},
	}

	fan := provider.FanOutStream("multi-err", s1, s2)
	iter, err := fan.Execute(context.Background(), "x")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	defer iter.Close()

	// Should encounter at least one error
	sawError := false
	for i := 0; i < 10; i++ {
		_, ok, err := iter.Next(context.Background())
		if err != nil {
			sawError = true
			break
		}
		if !ok {
			break
		}
	}
	if !sawError {
		t.Fatal("expected error from at least one source")
	}
}

// ============================================================================
// Additional: SinkResilience with Bulkhead
// ============================================================================

func TestWithSinkResilience_Bulkhead(t *testing.T) {
	t.Parallel()
	var callCount atomic.Int32
	sink := provider.NewSinkFunc("bh-sink", func(_ context.Context, _ string) error {
		callCount.Add(1)
		time.Sleep(50 * time.Millisecond)
		return nil
	})

	wrapped := provider.WithSinkResilience[string](sink, provider.ResilienceConfig{
		Bulkhead: &resilience.BulkheadConfig{
			Name:          "sink-bh",
			MaxConcurrent: 1,
			MaxWait:       0, // fail immediately if full
		},
	})

	// One call should succeed
	err := wrapped.Send(context.Background(), "msg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
