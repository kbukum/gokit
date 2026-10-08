package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/logging"
)

// switchableComponent reports whatever health the test sets.
type switchableComponent struct {
	mu     sync.Mutex
	status component.HealthStatus
}

func (s *switchableComponent) Name() string                { return "peer" }
func (s *switchableComponent) Start(context.Context) error { return nil }
func (s *switchableComponent) Stop(context.Context) error  { return nil }
func (s *switchableComponent) set(status component.HealthStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = status
}

func (s *switchableComponent) Health(context.Context) component.Health {
	s.mu.Lock()
	defer s.mu.Unlock()
	return component.Health{Name: "peer", Status: s.status, Message: string(s.status)}
}

func newAdminApp(t *testing.T, cfg AdminConfig) *App[*testConfig] {
	t.Helper()
	app, err := NewApp(newTestConfig("test", "1.0"), WithLogger(logging.NewDefault("test")), WithAdmin(cfg))
	if err != nil {
		t.Fatal(err)
	}
	app.Summary.SetWriter(io.Discard)
	return app
}

type probe struct {
	code int
	body Readiness
}

func get(ctx context.Context, t *testing.T, app *App[*testConfig], path string) probe {
	t.Helper()
	addr := app.AdminAddr()
	if addr == nil {
		t.Fatal("admin listener is not running")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr.String()+path, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var p probe
	p.code = resp.StatusCode
	if path == "/readyz" {
		if err := json.NewDecoder(resp.Body).Decode(&p.body); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
	}
	return p
}

func TestAdminReportsReadinessThroughTheLifecycle(t *testing.T) {
	t.Parallel()
	app := newAdminApp(t, AdminConfig{})
	peer := &switchableComponent{status: component.StatusHealthy}
	mustRegisterComponent(t, app, peer)
	var starting, draining, metricsWhileDraining probe
	app.OnAfterStart(func(ctx context.Context) error {
		starting = get(ctx, t, app, "/readyz")
		return nil
	})
	app.OnBeforeStop(func(ctx context.Context) error {
		draining = get(ctx, t, app, "/readyz")
		metricsWhileDraining = get(ctx, t, app, "/metrics")
		return nil
	})
	if err := app.Startup(t.Context()); err != nil {
		t.Fatal(err)
	}
	if starting.code != http.StatusServiceUnavailable || starting.body.Status != ReadinessStarting {
		t.Errorf("starting = %d %q, want 503 starting", starting.code, starting.body.Status)
	}
	if live := get(t.Context(), t, app, "/livez"); live.code != http.StatusOK {
		t.Errorf("livez = %d", live.code)
	}
	for _, tc := range []struct {
		health component.HealthStatus
		code   int
		status ReadinessStatus
	}{
		{component.StatusHealthy, http.StatusOK, ReadinessReady},
		{component.StatusDegraded, http.StatusOK, ReadinessDegraded},
		{component.StatusUnhealthy, http.StatusServiceUnavailable, ReadinessNotReady},
		{"", http.StatusServiceUnavailable, ReadinessNotReady},
		{"unknown", http.StatusServiceUnavailable, ReadinessNotReady},
	} {
		peer.set(tc.health)
		got := get(t.Context(), t, app, "/readyz")
		if got.code != tc.code || got.body.Status != tc.status {
			t.Errorf("peer %s: readyz = %d %q, want %d %q", tc.health, got.code, got.body.Status, tc.code, tc.status)
		}
		if !hasComponent(got.body.Components, "peer", tc.health) {
			t.Errorf("peer %s: components = %+v", tc.health, got.body.Components)
		}
	}
	addr := app.AdminAddr().String()
	if err := app.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if draining.code != http.StatusServiceUnavailable || draining.body.Status != ReadinessDraining {
		t.Errorf("draining = %d %q, want 503 draining", draining.code, draining.body.Status)
	}
	if metricsWhileDraining.code != http.StatusOK {
		t.Errorf("metrics while draining = %d", metricsWhileDraining.code)
	}
	if app.AdminAddr() != nil {
		t.Error("AdminAddr is set after shutdown")
	}
	if resp, err := http.Get("http://" + addr + "/livez"); err == nil { //nolint:noctx // a plain probe of a closed listener
		_ = resp.Body.Close()
		t.Fatal("admin listener still accepting after shutdown")
	}
}

func hasComponent(hs []component.Health, name string, status component.HealthStatus) bool {
	for _, h := range hs {
		if h.Name == name && h.Status == status {
			return true
		}
	}
	return false
}

// probingWriter reads /readyz the first time the startup summary is written.
type probingWriter struct {
	once  sync.Once
	probe func()
}

func (w *probingWriter) Write(p []byte) (int, error) {
	w.once.Do(w.probe)
	return len(p), nil
}

func TestAdminReportsStartingWhileTheSummaryIsWritten(t *testing.T) {
	t.Parallel()
	app := newAdminApp(t, AdminConfig{})
	var during probe
	app.Summary.SetWriter(&probingWriter{probe: func() { during = get(t.Context(), t, app, "/readyz") }})
	if err := app.Startup(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Shutdown(context.Background()) })
	if during.code != http.StatusServiceUnavailable || during.body.Status != ReadinessStarting {
		t.Errorf("readyz during summary = %d %q, want 503 starting", during.code, during.body.Status)
	}
	if after := get(t.Context(), t, app, "/readyz"); after.body.Status != ReadinessReady {
		t.Errorf("readyz after startup = %q, want ready", after.body.Status)
	}
}

func TestAdminServesPprofOnlyWhenEnabled(t *testing.T) {
	t.Parallel()
	for _, enabled := range []bool{false, true} {
		app := newAdminApp(t, AdminConfig{Pprof: enabled})
		if err := app.Startup(t.Context()); err != nil {
			t.Fatal(err)
		}
		want := http.StatusNotFound
		if enabled {
			want = http.StatusOK
		}
		if got := get(t.Context(), t, app, "/debug/pprof/"); got.code != want {
			t.Errorf("pprof %v: /debug/pprof/ = %d, want %d", enabled, got.code, want)
		}
		if err := app.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAdminUsesTheGivenMetricsHandler(t *testing.T) {
	t.Parallel()
	app := newAdminApp(t, AdminConfig{Metrics: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })})
	if err := app.Startup(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Shutdown(context.Background()) })
	if got := get(t.Context(), t, app, "/metrics"); got.code != http.StatusTeapot {
		t.Errorf("metrics = %d, want the custom handler", got.code)
	}
}

func TestAdminConfigIsValidated(t *testing.T) {
	t.Parallel()
	for name, cfg := range map[string]AdminConfig{
		"public host":             {Host: "0.0.0.0"},
		"public address":          {Host: "203.0.113.7"},
		"hostname":                {Host: "localhost"},
		"negative port":           {Port: -1},
		"port too large":          {Port: 65536},
		"negative health timeout": {HealthTimeout: -1},
		"typed-nil metrics":       {Metrics: http.HandlerFunc(nil)},
	} {
		_, err := NewApp(newTestConfig("test", "1.0"), WithLogger(logging.NewDefault("test")), WithAdmin(cfg))
		if !errors.Is(err, ErrInvalidAdminConfig) {
			t.Errorf("%s: NewApp = %v, want ErrInvalidAdminConfig", name, err)
		}
	}
}

func TestAppWithoutAdminHasNoAdminAddr(t *testing.T) {
	t.Parallel()
	if newQuietApp(t).AdminAddr() != nil {
		t.Fatal("AdminAddr without WithAdmin")
	}
}

func TestAdminPortInUseFailsStartup(t *testing.T) {
	t.Parallel()
	first := newAdminApp(t, AdminConfig{})
	if err := first.Startup(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Shutdown(context.Background()) })
	taken, ok := first.AdminAddr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("AdminAddr = %T", first.AdminAddr())
	}
	second := newAdminApp(t, AdminConfig{Port: taken.Port})
	var startupErr *StartupError
	if err := second.Startup(t.Context()); !errors.As(err, &startupErr) || startupErr.Phase != PhaseStart {
		t.Fatalf("Startup on a taken port = %v, want a start-phase StartupError", err)
	}
}
