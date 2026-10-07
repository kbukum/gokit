package testutil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/kbukum/gokit/bootstrap"
)

type clock interface{ Now() string }

type fixedClock string

func (c fixedClock) Now() string { return string(c) }

type greeter interface{ Greet(name string) string }

type clockGreeter struct{ clock clock }

func (g clockGreeter) Greet(name string) string { return "hello " + name + " at " + g.clock.Now() }

var (
	clockPort   = bootstrap.NewPort[clock]("clock")
	greeterPort = bootstrap.NewPort[greeter]("greeter")
)

// greeterModule is a module under test: it needs a clock, provides a greeter and serves it on "public".
type greeterModule struct{}

func (greeterModule) Spec() bootstrap.ModuleSpec {
	return bootstrap.ModuleSpec{Name: "greeter", Provides: []bootstrap.PortRef{greeterPort.Ref()}, Needs: []bootstrap.PortRef{clockPort.Ref()}, Listeners: []string{"public"}}
}

func (greeterModule) Register(_ context.Context, mc *bootstrap.ModuleContext) error {
	c, err := bootstrap.Need(mc, clockPort)
	if err != nil {
		return err
	}
	g := clockGreeter{clock: c}
	if err := mc.Handle("public", "GET /greet/{name}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, g.Greet(r.PathValue("name")))
	})); err != nil {
		return err
	}
	return bootstrap.Provide(mc, greeterPort, greeter(g))
}

// recordingTB records failures instead of stopping the test, so helper failure paths can be asserted.
type recordingTB struct {
	testing.TB
	failures []string
}

func (r *recordingTB) Helper() {}

func (r *recordingTB) Fatalf(format string, args ...any) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
}

func (r *recordingTB) Errorf(format string, args ...any) { r.Fatalf(format, args...) }

func get(t *testing.T, url string) (code int, body string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(b)
}

func TestHarnessRunsModuleWithDoublesCaptureAndListener(t *testing.T) {
	t.Parallel()
	var url string
	// Start's cleanup shuts the app down when the subtest finishes, before this cleanup runs.
	t.Cleanup(func() {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url+"/greet/x", http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		if resp, err := http.DefaultClient.Do(req); err == nil {
			_ = resp.Body.Close()
			t.Error("listener still serving after the test finished")
		}
	})
	t.Run("app", func(t *testing.T) {
		t.Parallel()
		app := NewApp(t)
		public := Listen(t, app, "public")
		got := Capture(t, app, greeterPort)
		if err := app.Use(greeterModule{}, bootstrap.ValueModule("clock", clockPort, clock(fixedClock("noon")))); err != nil {
			t.Fatal(err)
		}
		Start(t, app)
		if g := got().Greet("ada"); g != "hello ada at noon" {
			t.Fatalf("captured greeter = %q", g)
		}
		url = public.URL()
		if code, body := get(t, url+"/greet/grace"); code != http.StatusOK || body != "hello grace at noon" {
			t.Fatalf("GET = %d %q", code, body)
		}
	})
}

func TestCaptureMissingProviderFailsStartup(t *testing.T) {
	t.Parallel()
	app := NewApp(t)
	Capture(t, app, greeterPort)
	err := app.Startup(t.Context())
	var modErr *bootstrap.ModuleError
	if !errors.As(err, &modErr) || modErr.Problems[0].Kind != bootstrap.ProblemMissingPort || !strings.Contains(err.Error(), "capture greeter") {
		t.Fatalf("Startup = %v", err)
	}
}

func TestCheckModulesReportsCompositionGapsWithoutStarting(t *testing.T) {
	t.Parallel()
	app := NewApp(t)
	if err := app.Use(greeterModule{}); err != nil {
		t.Fatal(err)
	}
	err := app.CheckModules()
	var modErr *bootstrap.ModuleError
	if !errors.As(err, &modErr) || len(modErr.Problems) != 2 {
		t.Fatalf("CheckModules = %v, want missing clock and missing listener", err)
	}
	Listen(t, app, "public")
	if err := app.Use(bootstrap.ValueModule("clock", clockPort, clock(fixedClock("noon")))); err != nil {
		t.Fatal(err)
	}
	if err := app.CheckModules(); err != nil {
		t.Fatalf("CheckModules = %v", err)
	}
}

func TestHelpersReportFailuresOnT(t *testing.T) {
	t.Parallel()
	rec := &recordingTB{TB: t}

	early := NewApp(t)
	get := Capture(rec, early, greeterPort)
	_ = get()
	if len(rec.failures) != 1 || !strings.Contains(rec.failures[0], "before the app started") {
		t.Fatalf("read before start: %v", rec.failures)
	}

	failing := NewApp(t)
	Capture(t, failing, greeterPort)
	rec.failures = nil
	Start(rec, failing)
	if len(rec.failures) != 1 || !strings.Contains(rec.failures[0], "startup") {
		t.Fatalf("Start of an unwired app: %v", rec.failures)
	}

	started := NewApp(t)
	Start(t, started)
	rec.failures = nil
	Listen(rec, started, "late")
	Capture(rec, started, greeterPort)
	if len(rec.failures) != 2 {
		t.Fatalf("declarations after start: %v", rec.failures)
	}
}

func TestNewAppAppliesOptionsAfterDefaults(t *testing.T) {
	t.Parallel()
	app := NewApp(t, bootstrap.WithGracefulTimeout(0))
	if app.Name != "test" || app.Cfg.Environment != "development" {
		t.Fatalf("app = %q %q", app.Name, app.Cfg.Environment)
	}
}

func TestCaptureDistinguishesPortsWithTheSameName(t *testing.T) {
	t.Parallel()
	app := NewApp(t)
	first, second := bootstrap.NewPort[clock]("clock"), bootstrap.NewPort[clock]("clock")
	a, b := Capture(t, app, first), Capture(t, app, second)
	if err := app.Use(
		bootstrap.ValueModule("first", first, clock(fixedClock("1"))),
		bootstrap.ValueModule("second", second, clock(fixedClock("2"))),
	); err != nil {
		t.Fatal(err)
	}
	Start(t, app)
	if a().Now() != "1" || b().Now() != "2" {
		t.Fatalf("captured %q and %q", a().Now(), b().Now())
	}
}
