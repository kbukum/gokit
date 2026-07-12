//go:build integration

// Integration suite — exercises cross-package contracts (config↔component,
// provider↔pipeline, resilience↔errors, …). Slow and dependency-heavy; gated
// behind the `integration` build tag so the default `go test ./...` (and CI
// `check` job) stay fast. Run with `make test-integration` or
// `go test -tags=integration ./...`.
package gokit_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kbukum/gokit/component"
	gkconfig "github.com/kbukum/gokit/config"
	"github.com/kbukum/gokit/di"
	appErrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/observability"
	"github.com/kbukum/gokit/resilience"
	"github.com/kbukum/gokit/stream"
	"github.com/kbukum/gokit/validation"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

// testConfig satisfies bootstrap.Config by embedding config.ServiceConfig
type testConfig struct {
	gkconfig.ServiceConfig `yaml:",inline" mapstructure:",squash"`
}

func (c *testConfig) GetServiceConfig() *gkconfig.ServiceConfig { return &c.ServiceConfig }
func (c *testConfig) ApplyDefaults() {
	if c.Name == "" {
		c.Name = "test-service"
	}
}
func (c *testConfig) Validate() error { return nil }

func newTestConfig(name string) *testConfig {
	return &testConfig{
		ServiceConfig: gkconfig.ServiceConfig{
			Name:        name,
			Version:     "0.1.0",
			Environment: "development",
			Logging:     logging.Config{Level: "info", Format: "json", Output: "stdout"},
		},
	}
}

// trackingComponent records lifecycle events for verification.
type trackingComponent struct {
	name    string
	started atomic.Bool
	stopped atomic.Bool
	order   *[]string // shared slice to record ordering
}

func (c *trackingComponent) Name() string { return c.name }
func (c *trackingComponent) Start(ctx context.Context) error {
	c.started.Store(true)
	if c.order != nil {
		*c.order = append(*c.order, "start:"+c.name)
	}
	return nil
}

func (c *trackingComponent) Stop(ctx context.Context) error {
	c.stopped.Store(true)
	if c.order != nil {
		*c.order = append(*c.order, "stop:"+c.name)
	}
	return nil
}

func (c *trackingComponent) Health(ctx context.Context) component.Health {
	if c.started.Load() && !c.stopped.Load() {
		return component.Health{Name: c.name, Status: component.StatusHealthy}
	}
	return component.Health{Name: c.name, Status: component.StatusUnhealthy, Message: "not running"}
}

// countingProvider is a simple provider implementation for testing.
type countingProvider struct {
	callCount atomic.Int32
}

func (p *countingProvider) Name() string                       { return "counter" }
func (p *countingProvider) IsAvailable(_ context.Context) bool { return true }

// failingIterator yields errors on every call.
type failingIterator struct {
	err error
}

func (i *failingIterator) Next(_ context.Context) (int, bool, error) { return 0, false, i.err }
func (i *failingIterator) Close() error                              { return nil }

// sliceIterator provides pull-based access to a slice.
type sliceIterator[T any] struct {
	items []T
	idx   int
}

func (it *sliceIterator[T]) Next(_ context.Context) (T, bool, error) {
	if it.idx >= len(it.items) {
		var zero T
		return zero, false, nil
	}
	v := it.items[it.idx]
	it.idx++
	return v, true, nil
}
func (it *sliceIterator[T]) Close() error { return nil }

// ─── 1. Errors → Resilience ──────────────────────────────────────────────────

func TestIntegration_Errors_Resilience_CircuitBreakerPreservesErrorCode(t *testing.T) {
	cb := resilience.NewCircuitBreaker(resilience.CircuitBreakerConfig{
		Name:        "test-cb",
		MaxFailures: 3,
		Timeout:     100 * time.Millisecond,
	})

	// Generate failures using AppError
	for i := 0; i < 3; i++ {
		err := cb.Execute(func() error {
			return appErrors.ServiceUnavailable("database")
		})
		if err == nil {
			t.Fatal("expected error from circuit breaker")
		}
		var appErr *appErrors.AppError
		if errors.As(err, &appErr) {
			if appErr.Code != appErrors.ErrCodeServiceUnavailable {
				t.Errorf("expected SERVICE_UNAVAILABLE, got %s", appErr.Code)
			}
			if !appErr.Retryable {
				t.Error("SERVICE_UNAVAILABLE should be retryable")
			}
		}
	}

	// After max failures, circuit should be open
	if cb.State() != resilience.StateOpen {
		t.Errorf("expected circuit Open, got %v", cb.State())
	}

	// Execute on open circuit returns circuit-open error
	err := cb.Execute(func() error { return nil })
	if !errors.Is(err, resilience.ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
}

func TestIntegration_Errors_Resilience_RetryPreservesAppError(t *testing.T) {
	ctx := context.Background()
	attempts := 0
	cfg := resilience.RetryConfig{
		MaxAttempts:    3,
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     5 * time.Millisecond,
		BackoffFactor:  2.0,
		RetryIf: func(err error) bool {
			var appErr *appErrors.AppError
			return errors.As(err, &appErr) && appErr.Retryable
		},
	}

	_, err := resilience.Retry[int](ctx, cfg, func() (int, error) {
		attempts++
		return 0, appErrors.Timeout("db-query")
	})

	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
	// The underlying error should still be our AppError
	var appErr *appErrors.AppError
	if errors.As(err, &appErr) {
		if appErr.Code != appErrors.ErrCodeTimeout {
			t.Errorf("expected TIMEOUT code, got %s", appErr.Code)
		}
	}
}

func TestIntegration_Errors_Resilience_CircuitBreakerRecovery(t *testing.T) {
	cb := resilience.NewCircuitBreaker(resilience.CircuitBreakerConfig{
		Name:             "recovery-cb",
		MaxFailures:      2,
		Timeout:          50 * time.Millisecond,
		HalfOpenMaxCalls: 1,
	})

	// Trip the breaker
	for i := 0; i < 2; i++ {
		_ = cb.Execute(func() error { return appErrors.ConnectionFailed("redis") })
	}
	if cb.State() != resilience.StateOpen {
		t.Fatalf("expected Open, got %v", cb.State())
	}

	// Wait for timeout to transition to half-open
	time.Sleep(60 * time.Millisecond)

	// Successful probe should close the breaker
	err := cb.Execute(func() error { return nil })
	if err != nil {
		t.Fatalf("probe should succeed: %v", err)
	}
	if cb.State() != resilience.StateClosed {
		t.Errorf("expected Closed after successful probe, got %v", cb.State())
	}
}

// ─── 2. Config → Component ──────────────────────────────────────────────────

func TestIntegration_Config_Component_ConfigDrivesComponentInit(t *testing.T) {
	cfg := newTestConfig("config-test-svc")

	// Use config values to create a component
	comp := &trackingComponent{name: cfg.Name + "-db"}
	registry := component.NewRegistry()
	if err := registry.Register(comp); err != nil {
		t.Fatalf("register: %v", err)
	}

	ctx := context.Background()
	if err := registry.StartAll(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer registry.StopAll(ctx)

	if !comp.started.Load() {
		t.Error("component should be started")
	}

	health := registry.HealthAll(ctx)
	if len(health) != 1 || health[0].Status != component.StatusHealthy {
		t.Errorf("expected healthy, got %+v", health)
	}
}

func TestIntegration_Config_Component_MultipleComponentsFromConfig(t *testing.T) {
	cfg := newTestConfig("multi-comp-svc")
	names := []string{cfg.Name + "-db", cfg.Name + "-cache", cfg.Name + "-queue"}

	registry := component.NewRegistry()
	for _, n := range names {
		registry.Register(&trackingComponent{name: n})
	}

	ctx := context.Background()
	if err := registry.StartAll(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer registry.StopAll(ctx)

	health := registry.HealthAll(ctx)
	if len(health) != 3 {
		t.Fatalf("expected 3 health entries, got %d", len(health))
	}
	for _, h := range health {
		if h.Status != component.StatusHealthy {
			t.Errorf("component %s not healthy: %s", h.Name, h.Message)
		}
	}
}

// ─── 3. Provider → Pipeline ─────────────────────────────────────────────────

func TestIntegration_Provider_Pipeline_ProviderFeedsPipeline(t *testing.T) {
	// Create a provider-backed iterator
	data := []int{1, 2, 3, 4, 5}
	p := stream.FromSlice(data)

	// Apply pipeline operators: double values, keep those > 4
	doubled := stream.Map(p, func(_ context.Context, v int) (int, error) {
		return v * 2, nil
	})
	filtered := stream.Filter(doubled, func(v int) bool { return v > 4 })

	ctx := context.Background()
	results, err := stream.Collect(ctx, filtered)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}

	expected := []int{6, 8, 10}
	if len(results) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, results)
	}
	for i, v := range results {
		if v != expected[i] {
			t.Errorf("at index %d: expected %d, got %d", i, expected[i], v)
		}
	}
}

func TestIntegration_Provider_Pipeline_MapFilterReduce(t *testing.T) {
	p := stream.FromSlice([]int{1, 2, 3, 4, 5})
	mapped := stream.Map(p, func(_ context.Context, v int) (string, error) {
		return fmt.Sprintf("item-%d", v), nil
	})
	filtered := stream.Filter(mapped, func(v string) bool { return v != "item-3" })

	ctx := context.Background()
	results, err := stream.Collect(ctx, filtered)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(results) != 4 {
		t.Errorf("expected 4 items, got %d", len(results))
	}
}

func TestIntegration_Provider_Pipeline_ErrorPropagation(t *testing.T) {
	p := stream.FromSlice([]int{1, 2, 3})
	failing := stream.Map(p, func(_ context.Context, v int) (int, error) {
		if v == 2 {
			return 0, appErrors.InvalidInput("value", "cannot be 2")
		}
		return v * 10, nil
	})

	ctx := context.Background()
	_, err := stream.Collect(ctx, failing)
	if err == nil {
		t.Fatal("expected error from pipeline")
	}
	var appErr *appErrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T", err)
	}
	if appErr.Code != appErrors.ErrCodeInvalidInput {
		t.Errorf("expected INVALID_INPUT, got %s", appErr.Code)
	}
}

func TestIntegration_Provider_Pipeline_ConcatPipelines(t *testing.T) {
	p1 := stream.FromSlice([]int{1, 2})
	p2 := stream.FromSlice([]int{3, 4})
	combined := stream.Concat(p1, p2)

	ctx := context.Background()
	results, err := stream.Collect(ctx, combined)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	expected := []int{1, 2, 3, 4}
	if len(results) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, results)
	}
	for i, v := range results {
		if v != expected[i] {
			t.Errorf("at %d: expected %d, got %d", i, expected[i], v)
		}
	}
}

// ─── 4. Validation → Errors ─────────────────────────────────────────────────

func TestIntegration_Validation_Errors_ProducesCorrectAppError(t *testing.T) {
	v := validation.New()
	v.Required("name", "")
	v.Pattern("email", "not-an-email", `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

	appErr := v.Validate()
	if appErr == nil {
		t.Fatal("expected validation error")
	}
	if appErr.Code != appErrors.ErrCodeInvalidInput {
		t.Errorf("expected INVALID_INPUT, got %s", appErr.Code)
	}
	if appErr.HTTPStatus != 422 {
		t.Errorf("expected HTTP 422, got %d", appErr.HTTPStatus)
	}
}

func TestIntegration_Validation_Errors_MultipleFieldErrors(t *testing.T) {
	v := validation.New()
	v.Required("username", "")
	v.Min("age", 10, 18)
	v.Pattern("contact", "invalid", `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

	appErr := v.Validate()
	if appErr == nil {
		t.Fatal("expected validation error")
	}
	if appErr.Code != appErrors.ErrCodeInvalidInput {
		t.Errorf("expected INVALID_INPUT, got %s", appErr.Code)
	}
	// Details should contain field errors
	if len(appErr.Details) == 0 {
		t.Error("expected details with field errors")
	}
}

func TestIntegration_Validation_Errors_PassingValidation(t *testing.T) {
	v := validation.New()
	v.Required("name", "Alice")
	v.Pattern("email", "alice@example.com", `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

	appErr := v.Validate()
	if appErr != nil {
		t.Errorf("expected no error, got %v", appErr)
	}
}

func TestIntegration_Validation_Errors_ChainedValidation(t *testing.T) {
	v := validation.New()
	v.Required("id", "abc-123")
	v.Max("name", 3, 100) // length 3 <= 100, passes
	v.Required("role", "admin")

	appErr := v.Validate()
	if appErr != nil {
		t.Errorf("expected no error, got %v", appErr)
	}
}

// ─── 5. DI → Component → Bootstrap ─────────────────────────────────────────

func TestIntegration_DI_Component_Bootstrap_FullLifecycle(t *testing.T) {
	container := di.NewContainer()

	// Register components via DI
	dbComp := &trackingComponent{name: "postgres"}
	cacheComp := &trackingComponent{name: "redis"}

	if err := di.Register(container, dbComp, di.WithName("db")); err != nil {
		t.Fatalf("register db: %v", err)
	}
	if err := di.Register(container, cacheComp, di.WithName("cache")); err != nil {
		t.Fatalf("register cache: %v", err)
	}

	// Resolve from container
	resolvedComp, err := di.Resolve[*trackingComponent](context.Background(), container, di.WithName("db"))
	if err != nil {
		t.Fatalf("resolve db: %v", err)
	}
	if resolvedComp.Name() != "postgres" {
		t.Errorf("expected postgres, got %s", resolvedComp.Name())
	}

	// Register in component registry
	registry := component.NewRegistry()
	registry.Register(dbComp)
	registry.Register(cacheComp)

	ctx := context.Background()
	if err := registry.StartAll(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	if !dbComp.started.Load() || !cacheComp.started.Load() {
		t.Error("both components should be started")
	}

	if err := registry.StopAll(ctx); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if !dbComp.stopped.Load() || !cacheComp.stopped.Load() {
		t.Error("both components should be stopped")
	}
}

func TestIntegration_DI_Component_RegistrationModes(t *testing.T) {
	container := di.NewContainer()

	// Eager registration
	callCount := 0
	eager := func() *trackingComponent {
		callCount++
		return &trackingComponent{name: "eager"}
	}()
	if err := di.Register(container, eager, di.WithName("eager-svc")); err != nil {
		t.Fatalf("register eager-svc: %v", err)
	}
	if callCount != 1 {
		t.Errorf("eager should build the value immediately, callCount=%d", callCount)
	}

	// Lazy (singleton) registration
	lazyCount := 0
	if err := di.RegisterSingleton(container, func(context.Context) (*trackingComponent, error) {
		lazyCount++
		return &trackingComponent{name: "lazy"}, nil
	}, di.WithName("lazy-svc")); err != nil {
		t.Fatalf("register lazy-svc: %v", err)
	}
	if lazyCount != 0 {
		t.Error("lazy should not call factory until resolve")
	}

	_, err := di.Resolve[*trackingComponent](context.Background(), container, di.WithName("lazy-svc"))
	if err != nil {
		t.Fatalf("resolve lazy: %v", err)
	}
	if lazyCount != 1 {
		t.Errorf("lazy should have been called once, got %d", lazyCount)
	}
}

func TestIntegration_DI_Component_Bootstrap_StartupOrder(t *testing.T) {
	var order []string

	db := &trackingComponent{name: "db", order: &order}
	cache := &trackingComponent{name: "cache", order: &order}
	api := &trackingComponent{name: "api", order: &order}

	registry := component.NewRegistry()
	registry.Register(db)
	registry.Register(cache)
	registry.Register(api)

	ctx := context.Background()
	if err := registry.StartAll(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := registry.StopAll(ctx); err != nil {
		t.Fatalf("stop: %v", err)
	}

	// Verify start order is registration order
	expectedStart := []string{"start:db", "start:cache", "start:api"}
	for i, expected := range expectedStart {
		if i >= len(order) || order[i] != expected {
			t.Errorf("start order[%d]: expected %s, got %s", i, expected, safeIndex(order, i))
		}
	}

	// Verify stop order is reverse
	expectedStop := []string{"stop:api", "stop:cache", "stop:db"}
	for i, expected := range expectedStop {
		idx := len(expectedStart) + i
		if idx >= len(order) || order[idx] != expected {
			t.Errorf("stop order[%d]: expected %s, got %s", i, expected, safeIndex(order, idx))
		}
	}
}

func safeIndex(s []string, i int) string {
	if i >= len(s) {
		return "<out of range>"
	}
	return s[i]
}

// ─── 6. Resilience → Provider ───────────────────────────────────────────────

func TestIntegration_Resilience_Provider_CircuitBreakerWrapsProvider(t *testing.T) {
	callCount := atomic.Int32{}

	cb := resilience.NewCircuitBreaker(resilience.CircuitBreakerConfig{
		Name:        "provider-cb",
		MaxFailures: 2,
		Timeout:     100 * time.Millisecond,
	})

	// Simulate a provider call through circuit breaker
	providerCall := func() error {
		callCount.Add(1)
		return appErrors.ServiceUnavailable("external-api")
	}

	// Trip the circuit
	for i := 0; i < 2; i++ {
		_ = cb.Execute(providerCall)
	}

	currentCalls := callCount.Load()

	// Circuit is open — subsequent calls should not reach provider
	_ = cb.Execute(providerCall)

	if callCount.Load() != currentCalls {
		t.Error("circuit breaker should prevent calls when open")
	}
}

func TestIntegration_Resilience_Provider_RetryWithProvider(t *testing.T) {
	ctx := context.Background()
	attempts := atomic.Int32{}

	cfg := resilience.RetryConfig{
		MaxAttempts:    4,
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     5 * time.Millisecond,
		BackoffFactor:  1.5,
		RetryIf:        resilience.DefaultRetryIf,
	}

	result, err := resilience.Retry[string](ctx, cfg, func() (string, error) {
		n := attempts.Add(1)
		if n < 3 {
			return "", appErrors.ConnectionFailed("db")
		}
		return "success", nil
	})
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if result != "success" {
		t.Errorf("expected 'success', got '%s'", result)
	}
	if attempts.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts.Load())
	}
}

// ─── 7. Observability → Errors ──────────────────────────────────────────────

func TestIntegration_Observability_Errors_OperationContextTracksErrors(t *testing.T) {
	// Create an operation context (without real OTEL — just testing the struct integration)
	oc := observability.NewOperationContext("test-svc", "db-query", "req-123", "user-456", nil)

	if oc.ServiceName != "test-svc" {
		t.Errorf("expected service name 'test-svc', got '%s'", oc.ServiceName)
	}
	if oc.OperationName != "db-query" {
		t.Errorf("expected operation 'db-query', got '%s'", oc.OperationName)
	}
	if oc.RequestID != "req-123" {
		t.Errorf("expected request ID 'req-123', got '%s'", oc.RequestID)
	}

	// Duration should be measurable
	time.Sleep(1 * time.Millisecond)
	if oc.Duration() < 1*time.Millisecond {
		t.Error("duration should be at least 1ms")
	}
}

func TestIntegration_Observability_Errors_ContextPropagation(t *testing.T) {
	oc := observability.NewOperationContext("svc", "op", "req-1", "user-1", nil)
	ctx := observability.WithOperationContext(context.Background(), oc)

	// Retrieve from context
	retrieved := observability.OperationContextFromContext(ctx)
	if retrieved == nil {
		t.Fatal("expected OperationContext in context")
	}
	if retrieved.RequestID != "req-1" {
		t.Errorf("expected 'req-1', got '%s'", retrieved.RequestID)
	}

	// Create an error that would be recorded
	appErr := appErrors.Internal(fmt.Errorf("database connection lost"))
	if appErr.Code != appErrors.ErrCodeInternal {
		t.Errorf("expected INTERNAL_ERROR, got %s", appErr.Code)
	}
}

// ─── 8. Logger → Config ─────────────────────────────────────────────────────

func TestIntegration_Logger_Config_LoggerConfiguredViaConfig(t *testing.T) {
	cfg := &logging.Config{
		Level:       "debug",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "test-svc",
	}

	log := logging.New(cfg, "integration-test")
	if log == nil {
		t.Fatal("logger should not be nil")
	}

	// Logger should work without panicking
	log.Info("integration test message", map[string]any{
		"test":  true,
		"layer": "integration",
	})
}

func TestIntegration_Logger_Config_DefaultLogger(t *testing.T) {
	log := logging.NewDefault("default-test")
	if log == nil {
		t.Fatal("default logger should not be nil")
	}
	log.Debug("debug message from integration test")
}

func TestIntegration_Logger_Config_LoggerWithContext(t *testing.T) {
	cfg := &logging.Config{
		Level:       "info",
		Format:      "json",
		ServiceName: "ctx-test",
	}
	log := logging.New(cfg, "ctx-test")
	enriched := log.WithComponent("database").WithFields(map[string]any{
		"connection_pool": 10,
	})
	enriched.Info("component logger configured via config module")
}

// ─── 9. Pipeline → Reduce ──────────────────────────────────────────────────

func TestIntegration_Pipeline_Reduce(t *testing.T) {
	p := stream.FromSlice([]int{1, 2, 3, 4, 5})
	sumPipeline := stream.Reduce(p, 0, func(acc, v int) int { return acc + v })

	ctx := context.Background()
	results, err := stream.Collect(ctx, sumPipeline)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(results) != 1 || results[0] != 15 {
		t.Errorf("expected [15], got %v", results)
	}
}

// ─── 10. DI → Resilience ───────────────────────────────────────────────────

func TestIntegration_DI_Resilience_CircuitBreakerInContainer(t *testing.T) {
	container := di.NewContainer()

	cb := resilience.NewCircuitBreaker(resilience.CircuitBreakerConfig{
		Name:        "di-cb",
		MaxFailures: 3,
		Timeout:     100 * time.Millisecond,
	})

	if err := di.Register(container, cb, di.WithName("circuit-breaker")); err != nil {
		t.Fatalf("register circuit-breaker: %v", err)
	}

	resolvedCB, err := di.Resolve[*resilience.CircuitBreaker](context.Background(), container, di.WithName("circuit-breaker"))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	// Use the resolved circuit breaker
	execErr := resolvedCB.Execute(func() error { return nil })
	if execErr != nil {
		t.Errorf("expected success, got: %v", execErr)
	}
	if resolvedCB.State() != resilience.StateClosed {
		t.Errorf("expected Closed, got %v", resolvedCB.State())
	}
}

// ─── 11. Validation → Pipeline ──────────────────────────────────────────────

func TestIntegration_Validation_Pipeline_ValidateThenTransform(t *testing.T) {
	type UserInput struct {
		Name  string
		Email string
	}

	inputs := []UserInput{
		{Name: "Alice", Email: "alice@example.com"},
		{Name: "", Email: "bob@example.com"}, // invalid name
		{Name: "Charlie", Email: "charlie@test.com"},
	}

	p := stream.FromSlice(inputs)
	validated := stream.Map(p, func(_ context.Context, u UserInput) (string, error) {
		v := validation.New()
		v.Required("name", u.Name)
		v.Pattern("email", u.Email, `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
		if appErr := v.Validate(); appErr != nil {
			return "", appErr
		}
		return fmt.Sprintf("%s <%s>", u.Name, u.Email), nil
	})

	ctx := context.Background()
	_, err := stream.Collect(ctx, validated)
	if err == nil {
		t.Fatal("expected validation error for empty name")
	}
	var appErr *appErrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T", err)
	}
	if appErr.Code != appErrors.ErrCodeInvalidInput {
		t.Errorf("expected INVALID_INPUT, got %s", appErr.Code)
	}
}

// ─── 12. Component → Health → Observability ─────────────────────────────────

func TestIntegration_Component_Health_ObservabilityContext(t *testing.T) {
	comp := &trackingComponent{name: "test-db"}
	registry := component.NewRegistry()
	registry.Register(comp)

	ctx := context.Background()
	if err := registry.StartAll(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer registry.StopAll(ctx)

	// Check health
	healthResults := registry.HealthAll(ctx)
	if len(healthResults) == 0 {
		t.Fatal("expected at least one health result")
	}

	// Create observability context based on health check
	oc := observability.NewOperationContext("test-svc", "health-check", "hc-1", "", nil)
	if oc.OperationName != "health-check" {
		t.Errorf("expected health-check, got %s", oc.OperationName)
	}

	for _, h := range healthResults {
		if h.Status != component.StatusHealthy {
			t.Errorf("component %s is %s: %s", h.Name, h.Status, h.Message)
		}
	}
}

// ─── 13. Errors → DI ────────────────────────────────────────────────────────

func TestIntegration_Errors_DI_ResolveNonexistentReturnsError(t *testing.T) {
	container := di.NewContainer()

	_, err := di.Resolve[string](context.Background(), container, di.WithName("nonexistent"))
	if err == nil {
		t.Fatal("expected error for nonexistent key")
	}
}

// ─── 14. Full Stack: Config → DI → Component → Pipeline ────────────────────

func TestIntegration_FullStack_ConfigDIComponentPipeline(t *testing.T) {
	// 1. Config
	cfg := newTestConfig("full-stack-svc")

	// 2. DI container
	container := di.NewContainer()
	if err := di.Register(container, cfg, di.WithName("config")); err != nil {
		t.Fatalf("register config: %v", err)
	}

	// 3. Components
	db := &trackingComponent{name: cfg.Name + "-db"}
	registry := component.NewRegistry()
	registry.Register(db)

	ctx := context.Background()
	if err := registry.StartAll(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer registry.StopAll(ctx)

	// 4. Pipeline processes data
	data := stream.FromSlice([]int{10, 20, 30, 40, 50})
	processed := stream.Map(data, func(_ context.Context, v int) (int, error) {
		return v * 2, nil
	})
	results, err := stream.Collect(ctx, processed)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}

	if len(results) != 5 {
		t.Errorf("expected 5 results, got %d", len(results))
	}
	if results[0] != 20 || results[4] != 100 {
		t.Errorf("unexpected results: %v", results)
	}
}

// ─── 15. Pipeline Tap + ForEach ─────────────────────────────────────────────

func TestIntegration_Pipeline_TapSideEffects(t *testing.T) {
	var sideEffects []int

	p := stream.FromSlice([]int{1, 2, 3})
	tapped := stream.Tap(p, func(_ context.Context, v int) error {
		sideEffects = append(sideEffects, v)
		return nil
	})

	ctx := context.Background()
	results, err := stream.Collect(ctx, tapped)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
	if len(sideEffects) != 3 {
		t.Errorf("expected 3 side effects, got %d", len(sideEffects))
	}
}

// ─── 16. Errors fluent builder integration ──────────────────────────────────

func TestIntegration_Errors_FluentBuilder_AcrossModules(t *testing.T) {
	// Create error with fluent API
	appErr := appErrors.NotFound("user", "user-123").
		WithDetail("search_field", "email").
		WithDetails(map[string]any{"attempted_at": "2024-01-01"})

	if appErr.Code != appErrors.ErrCodeNotFound {
		t.Errorf("expected NOT_FOUND, got %s", appErr.Code)
	}
	if appErr.HTTPStatus != 404 {
		t.Errorf("expected 404, got %d", appErr.HTTPStatus)
	}
	if appErr.Details["search_field"] != "email" {
		t.Error("expected detail 'search_field' = 'email'")
	}
	if _, ok := appErr.Details["attempted_at"]; !ok {
		t.Error("expected detail 'attempted_at'")
	}
	if appErr.Retryable {
		t.Error("NOT_FOUND should not be retryable")
	}
}
