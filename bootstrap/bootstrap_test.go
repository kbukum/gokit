package bootstrap

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/config"
	"github.com/kbukum/gokit/di"
	"github.com/kbukum/gokit/logger"
)

// testConfig is a minimal config for testing that satisfies the Config interface.
type testConfig struct {
	config.ServiceConfig
}

// mockComponent implements component.Component for testing.
type mockComponent struct {
	name     string
	startErr error
	stopErr  error
	health   component.Health
	started  bool
	stopped  bool
}

func (m *mockComponent) Name() string { return m.name }
func (m *mockComponent) Start(ctx context.Context) error {
	m.started = true
	return m.startErr
}
func (m *mockComponent) Stop(ctx context.Context) error {
	m.stopped = true
	return m.stopErr
}
func (m *mockComponent) Health(ctx context.Context) component.Health {
	return m.health
}

func newTestConfig(name, version string) *testConfig {
	return &testConfig{
		ServiceConfig: config.ServiceConfig{
			Name:        name,
			Version:     version,
			Environment: "development",
		},
	}
}

func TestNewApp(t *testing.T) {
	cfg := newTestConfig("test-svc", "1.0.0")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp failed: %v", err)
	}
	if app == nil {
		t.Fatal("expected non-nil app")
	}
	if app.Name != "test-svc" {
		t.Errorf("expected name 'test-svc', got %q", app.Name)
	}
	if app.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", app.Version)
	}
	if app.Container == nil {
		t.Error("expected non-nil container")
	}
	if app.Components == nil {
		t.Error("expected non-nil components registry")
	}
	if app.Logger == nil {
		t.Error("expected non-nil logger")
	}
	// Config is typed
	if app.Cfg.Name != "test-svc" {
		t.Errorf("expected cfg.Name 'test-svc', got %q", app.Cfg.Name)
	}
}

func TestNewAppValidation(t *testing.T) {
	cfg := &testConfig{
		ServiceConfig: config.ServiceConfig{
			// Name is empty — should fail validation
			Environment: "development",
		},
	}
	_, err := NewApp(cfg)
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestNewAppWithOptions(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	container := di.NewContainer()
	app, err := NewApp(cfg,
		WithGracefulTimeout(30*time.Second),
		WithContainer(container),
	)
	if err != nil {
		t.Fatalf("NewApp failed: %v", err)
	}

	if app.gracefulTimeout != 30*time.Second {
		t.Errorf("expected 30s timeout, got %v", app.gracefulTimeout)
	}
	if app.Container != container {
		t.Error("expected custom container")
	}
}

func TestRegisterComponent(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	c := &mockComponent{
		name:   "db",
		health: component.Health{Name: "db", Status: component.StatusHealthy},
	}

	if err := app.RegisterComponent(c); err != nil {
		t.Fatalf("RegisterComponent failed: %v", err)
	}

	got := app.Components.Get("db")
	if got == nil {
		t.Error("expected component to be registered")
	}
}

func TestRegisterComponentDuplicate(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	c := &mockComponent{name: "db"}
	app.RegisterComponent(c)

	err := app.RegisterComponent(&mockComponent{name: "db"})
	if err == nil {
		t.Error("expected error for duplicate component registration")
	}
}

func TestOnStartHook(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	called := false
	app.OnStart(func(ctx context.Context) error {
		called = true
		return nil
	})

	if len(app.onStart) != 1 {
		t.Errorf("expected 1 onStart hook, got %d", len(app.onStart))
	}

	err := runHooks(context.Background(), app.onStart)
	if err != nil {
		t.Fatalf("hook failed: %v", err)
	}
	if !called {
		t.Error("expected onStart hook to be called")
	}
}

func TestOnReadyHook(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	called := false
	app.OnReady(func(ctx context.Context) error {
		called = true
		return nil
	})

	err := runHooks(context.Background(), app.onReady)
	if err != nil {
		t.Fatalf("hook failed: %v", err)
	}
	if !called {
		t.Error("expected onReady hook to be called")
	}
}

func TestOnStopHook(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	called := false
	app.OnStop(func(ctx context.Context) error {
		called = true
		return nil
	})

	err := runHooks(context.Background(), app.onStop)
	if err != nil {
		t.Fatalf("hook failed: %v", err)
	}
	if !called {
		t.Error("expected onStop hook to be called")
	}
}

func TestMultipleHooks(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	order := []string{}
	app.OnStart(
		func(ctx context.Context) error { order = append(order, "first"); return nil },
		func(ctx context.Context) error { order = append(order, "second"); return nil },
	)

	runHooks(context.Background(), app.onStart)
	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Errorf("expected [first, second], got %v", order)
	}
}

func TestHookError(t *testing.T) {
	hooks := []Hook{
		func(ctx context.Context) error { return fmt.Errorf("hook failed") },
	}
	err := runHooks(context.Background(), hooks)
	if err == nil {
		t.Error("expected error from failing hook")
	}
}

func TestHookErrorStopsExecution(t *testing.T) {
	secondCalled := false
	hooks := []Hook{
		func(ctx context.Context) error { return fmt.Errorf("fail") },
		func(ctx context.Context) error { secondCalled = true; return nil },
	}
	runHooks(context.Background(), hooks)
	if secondCalled {
		t.Error("expected second hook not to be called after first fails")
	}
}

func TestReadyCheckAllHealthy(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	app.RegisterComponent(&mockComponent{
		name:   "db",
		health: component.Health{Name: "db", Status: component.StatusHealthy},
	})
	app.RegisterComponent(&mockComponent{
		name:   "cache",
		health: component.Health{Name: "cache", Status: component.StatusHealthy},
	})

	err := app.ReadyCheck(context.Background())
	if err != nil {
		t.Errorf("expected no error for all healthy, got %v", err)
	}
}

func TestReadyCheckUnhealthy(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	app.RegisterComponent(&mockComponent{
		name:   "db",
		health: component.Health{Name: "db", Status: component.StatusHealthy},
	})
	app.RegisterComponent(&mockComponent{
		name:   "cache",
		health: component.Health{Name: "cache", Status: component.StatusUnhealthy, Message: "timeout"},
	})

	err := app.ReadyCheck(context.Background())
	if err == nil {
		t.Error("expected error for unhealthy component")
	}
}

func TestReadyCheckDegraded(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	app.RegisterComponent(&mockComponent{
		name:   "svc",
		health: component.Health{Name: "svc", Status: component.StatusDegraded, Message: "slow"},
	})

	err := app.ReadyCheck(context.Background())
	if err == nil {
		t.Error("expected error for degraded component")
	}
}

func TestReadyCheckEmpty(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	err := app.ReadyCheck(context.Background())
	if err != nil {
		t.Errorf("expected no error for empty registry, got %v", err)
	}
}

func TestOnConfigure(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	configured := false
	app.OnConfigure(func(ctx context.Context, a *App[*testConfig]) error {
		configured = true
		if a.Name != "test" {
			t.Errorf("expected app name 'test' in configure callback, got %q", a.Name)
		}
		// Type-safe config access
		if a.Cfg.Name != "test" {
			t.Errorf("expected cfg.Name 'test', got %q", a.Cfg.Name)
		}
		return nil
	})

	if len(app.onConfigure) != 1 {
		t.Errorf("expected 1 configure callback, got %d", len(app.onConfigure))
	}

	for _, fn := range app.onConfigure {
		if err := fn(context.Background(), app); err != nil {
			t.Fatalf("configure failed: %v", err)
		}
	}
	if !configured {
		t.Error("expected configure callback to run")
	}
}

func TestWithGracefulTimeout(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg, WithGracefulTimeout(5*time.Second))
	if app.gracefulTimeout != 5*time.Second {
		t.Errorf("expected 5s, got %v", app.gracefulTimeout)
	}
}

func TestDefaultGracefulTimeout(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	if app.gracefulTimeout != 15*time.Second {
		t.Errorf("expected default 15s, got %v", app.gracefulTimeout)
	}
}

func TestRunTaskSuccess(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	executed := false
	err := app.RunTask(context.Background(), func(ctx context.Context) error {
		executed = true
		return nil
	})
	if err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}
	if !executed {
		t.Error("expected task to be executed")
	}
}

func TestRunTaskError(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	err := app.RunTask(context.Background(), func(ctx context.Context) error {
		return fmt.Errorf("task error")
	})
	if err == nil {
		t.Error("expected error from failing task")
	}
	if err.Error() != "task error" {
		t.Errorf("expected 'task error', got %q", err.Error())
	}
}

func TestRunTaskCancellation(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	ctx, cancel := context.WithCancel(context.Background())

	err := app.RunTask(ctx, func(taskCtx context.Context) error {
		cancel() // simulate signal
		<-taskCtx.Done()
		return taskCtx.Err()
	})
	if err == nil {
		t.Error("expected error from canceled task")
	}
}

func TestRunTaskWithHooks(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)

	order := []string{}
	app.OnStart(func(ctx context.Context) error {
		order = append(order, "start")
		return nil
	})
	app.OnConfigure(func(ctx context.Context, a *App[*testConfig]) error {
		order = append(order, "configure")
		return nil
	})
	app.OnReady(func(ctx context.Context) error {
		order = append(order, "ready")
		return nil
	})
	app.OnStop(func(ctx context.Context) error {
		order = append(order, "stop")
		return nil
	})

	app.RunTask(context.Background(), func(ctx context.Context) error {
		order = append(order, "task")
		return nil
	})

	expected := []string{"start", "configure", "ready", "task", "stop"}
	if len(order) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("order[%d] = %q, expected %q", i, order[i], v)
		}
	}
}

func TestRunTaskWithComponents(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	comp := &mockComponent{
		name:   "db",
		health: component.Health{Name: "db", Status: component.StatusHealthy},
	}
	app.RegisterComponent(comp)

	app.RunTask(context.Background(), func(ctx context.Context) error {
		return nil
	})

	if !comp.started {
		t.Error("expected component to be started")
	}
	if !comp.stopped {
		t.Error("expected component to be stopped after task")
	}
}

func TestShutdown(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	comp := &mockComponent{
		name:   "db",
		health: component.Health{Name: "db", Status: component.StatusHealthy},
	}
	app.RegisterComponent(comp)

	// Start components first
	app.RunTask(context.Background(), func(ctx context.Context) error {
		// While running, call Shutdown
		return nil
	})

	// Shutdown should work after RunTask
	err := app.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}

func TestWaitForSignalContextCancellation(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	sig := app.WaitForSignal(ctx)
	if sig != nil {
		t.Errorf("expected nil signal for context cancellation, got %v", sig)
	}
}

func TestWithLogger(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	customLogger := logger.NewDefault("custom-logger")

	app, err := NewApp(cfg, WithLogger(customLogger))
	if err != nil {
		t.Fatalf("NewApp failed: %v", err)
	}
	if app.Logger != customLogger {
		t.Error("expected custom logger to be set")
	}
}

func TestRunTaskWithStartHookError(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	app.OnStart(func(ctx context.Context) error {
		return fmt.Errorf("start hook failed")
	})

	err := app.RunTask(context.Background(), func(ctx context.Context) error {
		return nil
	})
	if err == nil {
		t.Error("expected error from failing start hook")
	}
}

func TestRunTaskWithConfigureError(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	app.OnConfigure(func(ctx context.Context, a *App[*testConfig]) error {
		return fmt.Errorf("configure failed")
	})

	err := app.RunTask(context.Background(), func(ctx context.Context) error {
		return nil
	})
	if err == nil {
		t.Error("expected error from failing configure callback")
	}
}

func TestRunTaskWithReadyHookError(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	app.OnReady(func(ctx context.Context) error {
		return fmt.Errorf("ready hook failed")
	})

	err := app.RunTask(context.Background(), func(ctx context.Context) error {
		return nil
	})
	if err == nil {
		t.Error("expected error from failing ready hook")
	}
}

func TestRunTaskWithStopHookError(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	app.OnStop(func(ctx context.Context) error {
		return fmt.Errorf("stop hook failed")
	})

	err := app.RunTask(context.Background(), func(ctx context.Context) error {
		return nil
	})
	if err == nil {
		t.Error("expected error from failing stop hook")
	}
}

func TestRunTaskComponentStartError(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	app.RegisterComponent(&mockComponent{
		name:     "bad",
		startErr: fmt.Errorf("start failed"),
	})

	err := app.RunTask(context.Background(), func(ctx context.Context) error {
		return nil
	})
	if err == nil {
		t.Error("expected error from component start failure")
	}
}

func TestNewSummary(t *testing.T) {
	s := NewSummary("my-service", "2.0.0")
	if s == nil {
		t.Fatal("expected non-nil summary")
	}
	if s.serviceName != "my-service" {
		t.Errorf("expected 'my-service', got %q", s.serviceName)
	}
	if s.version != "2.0.0" {
		t.Errorf("expected '2.0.0', got %q", s.version)
	}
}

func TestSummaryTrackComponent(t *testing.T) {
	s := NewSummary("svc", "1.0")
	s.TrackComponent("db", "active", true)
	s.TrackComponent("cache", "error", false)

	if len(s.components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(s.components))
	}
	if s.components[0].Name != "db" || !s.components[0].Healthy {
		t.Error("expected healthy db component")
	}
	if s.components[1].Healthy {
		t.Error("expected unhealthy cache component")
	}
}

func TestSummaryTrackInfrastructure(t *testing.T) {
	s := NewSummary("svc", "1.0")
	s.TrackInfrastructure("PostgreSQL", "database", "active", "localhost:5432", 5432, true)

	if len(s.infrastructure) != 1 {
		t.Fatalf("expected 1 infrastructure, got %d", len(s.infrastructure))
	}
	inf := s.infrastructure[0]
	if inf.Name != "PostgreSQL" || inf.Port != 5432 {
		t.Errorf("unexpected infrastructure: %+v", inf)
	}
}

func TestSummaryTrackBusinessComponent(t *testing.T) {
	s := NewSummary("svc", "1.0")
	s.TrackBusinessComponent("user-service", "service", "active", []string{"db", "cache"})

	if len(s.business) != 1 {
		t.Fatalf("expected 1 business component, got %d", len(s.business))
	}
	if s.business[0].Name != "user-service" {
		t.Errorf("expected 'user-service', got %q", s.business[0].Name)
	}
	if len(s.business[0].Dependencies) != 2 {
		t.Errorf("expected 2 dependencies, got %d", len(s.business[0].Dependencies))
	}
}

func TestSummaryTrackRoute(t *testing.T) {
	s := NewSummary("svc", "1.0")
	s.TrackRoute("GET", "/users", "UserHandler")
	s.TrackRoute("POST", "/users", "CreateUserHandler")

	if len(s.routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(s.routes))
	}
}

func TestSummaryTrackConsumer(t *testing.T) {
	s := NewSummary("svc", "1.0")
	s.TrackConsumer("order-consumer", "group-1", "orders", "active")

	if len(s.consumers) != 1 {
		t.Fatalf("expected 1 consumer, got %d", len(s.consumers))
	}
	if s.consumers[0].Topic != "orders" {
		t.Errorf("expected topic 'orders', got %q", s.consumers[0].Topic)
	}
}

func TestSummaryTrackClient(t *testing.T) {
	s := NewSummary("svc", "1.0")
	s.TrackClient("auth-client", "localhost:9090", "connected", "grpc")

	if len(s.clients) != 1 {
		t.Fatalf("expected 1 client, got %d", len(s.clients))
	}
	if s.clients[0].Type != "grpc" {
		t.Errorf("expected type 'grpc', got %q", s.clients[0].Type)
	}
}

func TestSummarySetStartupDuration(t *testing.T) {
	s := NewSummary("svc", "1.0")
	s.SetStartupDuration(500 * time.Millisecond)

	if s.startupDuration != 500*time.Millisecond {
		t.Errorf("expected 500ms, got %v", s.startupDuration)
	}
}

func TestSummaryDisplaySummary(t *testing.T) {
	s := NewSummary("test-svc", "1.0.0")
	s.SetStartupDuration(100 * time.Millisecond)
	s.TrackInfrastructure("DB", "database", "active", "localhost:5432", 5432, true)
	s.TrackRoute("GET", "/health", "HealthHandler")
	s.TrackBusinessComponent("user-svc", "service", "active", []string{"db"})
	s.TrackConsumer("events", "g1", "events-topic", "active")
	s.TrackClient("auth", "localhost:9090", "connected", "grpc")

	registry := component.NewRegistry()
	container := di.NewContainer()

	// DisplaySummary should not panic
	s.DisplaySummary(registry, container, nil)
}

func TestSummaryDisplaySummaryNilRegistry(t *testing.T) {
	s := NewSummary("test-svc", "1.0.0")
	s.SetStartupDuration(100 * time.Millisecond)

	// Should not panic with nil container (registry is required)
	registry := component.NewRegistry()
	s.DisplaySummary(registry, nil, nil)
}

func TestSummaryDisplayWithDIRegistrations(t *testing.T) {
	s := NewSummary("test-svc", "1.0.0")
	s.SetStartupDuration(100 * time.Millisecond)

	registry := component.NewRegistry()
	container := di.NewContainer()
	container.RegisterSingleton("service.user", "user-svc")
	container.RegisterSingleton("repository.user", "user-repo")
	container.RegisterSingleton("handler.users", "users-handler")
	container.RegisterSingleton("config", "cfg")

	// Should not panic
	s.DisplaySummary(registry, container, nil)
}

func TestTreePrefix(t *testing.T) {
	// Last item should use └──
	if p := treePrefix(2, 3); p != "└──" {
		t.Errorf("expected '└──' for last item, got %q", p)
	}
	// Non-last item should use ├──
	if p := treePrefix(0, 3); p != "├──" {
		t.Errorf("expected '├──' for non-last item, got %q", p)
	}
}

func TestStatusIcon(t *testing.T) {
	tests := []struct {
		status  string
		healthy bool
		icon    string
	}{
		{"active", true, "✅"},
		{"lazy", true, "⚡"},
		{"inactive", true, "⏸️"},
		{"error", true, "❌"},
		{"unknown", true, "⚠️"},
		{"active", false, "❌"},
	}

	for _, tc := range tests {
		got := statusIcon(tc.status, tc.healthy)
		if got != tc.icon {
			t.Errorf("statusIcon(%q, %v) = %q, expected %q", tc.status, tc.healthy, got, tc.icon)
		}
	}
}

func TestHealthStatusIcon(t *testing.T) {
	tests := []struct {
		status component.HealthStatus
		icon   string
	}{
		{component.StatusHealthy, "✅"},
		{component.StatusDegraded, "⚠️"},
		{component.StatusUnhealthy, "❌"},
		{"unknown", "❓"},
	}

	for _, tc := range tests {
		got := healthStatusIcon(tc.status)
		if got != tc.icon {
			t.Errorf("healthStatusIcon(%q) = %q, expected %q", tc.status, got, tc.icon)
		}
	}
}

func TestBusinessIcon(t *testing.T) {
	if businessIcon("service") != "⚙️" {
		t.Error("expected ⚙️ for service")
	}
	if businessIcon("repository") != "📁" {
		t.Error("expected 📁 for repository")
	}
	if businessIcon("handler") != "🎯" {
		t.Error("expected 🎯 for handler")
	}
	if businessIcon("other") != "💼" {
		t.Error("expected 💼 for other")
	}
}

func TestMethodColor(t *testing.T) {
	tests := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "CONNECT", "OPTIONS"}
	for _, m := range tests {
		got := methodColor(m)
		if got == "" {
			t.Errorf("expected non-empty color for %s", m)
		}
	}
}

// mockDescribableComponent implements Component + Describable + RouteProvider
type mockDescribableComponent struct {
	mockComponent
	desc   component.Description
	routes []component.Route
}

func (m *mockDescribableComponent) Describe() component.Description { return m.desc }
func (m *mockDescribableComponent) Routes() []component.Route       { return m.routes }

func TestSummaryCollectFromRegistry(t *testing.T) {
	s := NewSummary("test-svc", "1.0.0")
	s.SetStartupDuration(100 * time.Millisecond)

	registry := component.NewRegistry()
	// Register a describable component with routes
	comp := &mockDescribableComponent{
		mockComponent: mockComponent{
			name:   "http-server",
			health: component.Health{Name: "http-server", Status: component.StatusHealthy},
		},
		desc: component.Description{
			Name:    "HTTP Server",
			Type:    "server",
			Details: "localhost:8080",
			Port:    8080,
		},
		routes: []component.Route{
			{Method: "GET", Path: "/api/users", Handler: "ListUsers"},
			{Method: "POST", Path: "/api/users", Handler: "CreateUser"},
		},
	}
	registry.Register(comp)

	container := di.NewContainer()
	s.DisplaySummary(registry, container, nil)

	// Verify infrastructure was auto-discovered
	if len(s.infrastructure) != 1 {
		t.Errorf("expected 1 infrastructure from auto-discovery, got %d", len(s.infrastructure))
	}
	// Verify routes were auto-discovered
	if len(s.routes) != 2 {
		t.Errorf("expected 2 routes from auto-discovery, got %d", len(s.routes))
	}
}

func TestSummaryDisplayWithUnhealthyComponents(t *testing.T) {
	s := NewSummary("test-svc", "1.0.0")
	s.SetStartupDuration(100 * time.Millisecond)

	registry := component.NewRegistry()
	registry.Register(&mockComponent{
		name:   "db",
		health: component.Health{Name: "db", Status: component.StatusUnhealthy, Message: "connection refused"},
	})

	container := di.NewContainer()
	// Should not panic and should show health issues
	s.DisplaySummary(registry, container, nil)
}

func TestRunTaskWithComponentStopError(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	comp := &mockComponent{
		name:    "db",
		stopErr: fmt.Errorf("stop failed"),
		health:  component.Health{Name: "db", Status: component.StatusHealthy},
	}
	app.RegisterComponent(comp)

	err := app.RunTask(context.Background(), func(ctx context.Context) error {
		return nil
	})
	if err == nil {
		t.Error("expected error from component stop failure")
	}
}
