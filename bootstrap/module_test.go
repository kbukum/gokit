package bootstrap

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/kbukum/gokit/component"
)

type greeter interface{ Greet(name string) string }

type store interface{ Get(key string) string }

type greeterFunc func(string) string

func (f greeterFunc) Greet(name string) string { return f(name) }

type storeFunc func(string) string

func (f storeFunc) Get(key string) string { return f(key) }

var (
	greeterPort = NewPort[greeter]("greeter")
	storePort   = NewPort[store]("store")
)

func aGreeter() greeter { return greeterFunc(func(n string) string { return "hello " + n }) }
func aStore() store     { return storeFunc(func(k string) string { return "value:" + k }) }

// testModule is a configurable module whose Register runs the supplied function.
type testModule struct {
	spec     ModuleSpec
	register func(context.Context, *ModuleContext) error
}

func (m *testModule) Spec() ModuleSpec { return m.spec }
func (m *testModule) Register(ctx context.Context, mc *ModuleContext) error {
	if m.register == nil {
		return nil
	}
	return m.register(ctx, mc)
}

func module(name string, provides, needs []PortRef, register func(context.Context, *ModuleContext) error) *testModule {
	return &testModule{spec: ModuleSpec{Name: name, Provides: provides, Needs: needs}, register: register}
}

// on declares the listeners a test module mounts routes on.
func (m *testModule) on(listeners ...string) *testModule {
	m.spec.Listeners = listeners
	return m
}

func refs(rs ...PortRef) []PortRef { return rs }

func mustUse(t *testing.T, app *App[*testConfig], modules ...Module) {
	t.Helper()
	if err := app.Use(modules...); err != nil {
		t.Fatal(err)
	}
}

// testListener is an ingress listener over a ServeMux that records its lifecycle.
type testListener struct {
	orderTrackingComponent
	*http.ServeMux
	phase    component.DrainPhase
	quiesced bool
}

func newListener(name string, order *[]string) *testListener {
	if order == nil {
		order = &[]string{}
	}
	return &testListener{
		orderTrackingComponent: orderTrackingComponent{name: name, order: order, health: component.Health{Name: name, Status: component.StatusHealthy}},
		ServeMux:               http.NewServeMux(),
		phase:                  component.DrainIngress,
	}
}

// Quiesce is idempotent, as component.Quiescer requires; it records only the first call.
func (l *testListener) Quiesce() error {
	if !l.quiesced {
		l.quiesced = true
		*l.order = append(*l.order, l.name+":quiesce")
	}
	return nil
}

func (l *testListener) Drain(context.Context) error {
	*l.order = append(*l.order, l.name+":drain")
	return nil
}

func (l *testListener) DrainPhase() component.DrainPhase { return l.phase }

// Handle reports the panics http.ServeMux raises for a nil handler or an invalid or conflicting pattern as errors, as a Listener must.
func (l *testListener) Handle(pattern string, h http.Handler) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("test listener: %v", r)
		}
	}()
	l.ServeMux.Handle(pattern, h)
	return nil
}

func (l *testListener) Fallback(h http.Handler) error { return l.Handle("/", h) }

func mustListen(t *testing.T, app *App[*testConfig], name string, order *[]string) *testListener {
	t.Helper()
	l := newListener(name, order)
	if err := app.Listen(name, l); err != nil {
		t.Fatal(err)
	}
	return l
}

// workerComponent records its lifecycle and drains as a worker.
type workerComponent struct{ orderTrackingComponent }

func (w *workerComponent) Drain(context.Context) error {
	*w.order = append(*w.order, w.name+":drain")
	return nil
}

func (w *workerComponent) DrainPhase() component.DrainPhase { return component.DrainWorkers }

// moduleProblems starts app, expects a modules-phase failure, and returns its problems.
func moduleProblems(t *testing.T, app *App[*testConfig]) []ModuleProblem {
	t.Helper()
	err := app.Startup(context.Background())
	var startErr *StartupError
	if !errors.As(err, &startErr) || startErr.Phase != PhaseModules {
		t.Fatalf("Startup error = %v, want a modules-phase *StartupError", err)
	}
	var modErr *ModuleError
	if !errors.As(err, &modErr) {
		t.Fatalf("Startup error = %v, want *ModuleError", err)
	}
	return modErr.Problems
}

func hasProblem(problems []ModuleProblem, kind ModuleProblemKind, port PortRef) bool {
	return slices.ContainsFunc(problems, func(p ModuleProblem) bool { return p.Kind == kind && p.Port == port })
}

func countKind(problems []ModuleProblem, kind ModuleProblemKind) int {
	n := 0
	for _, p := range problems {
		if p.Kind == kind {
			n++
		}
	}
	return n
}

func TestModulesProvideAndNeedInDependencyOrder(t *testing.T) {
	t.Parallel()
	app := newQuietApp(t)
	var order []string
	consumer := module("consumer", nil, refs(greeterPort.Ref()), func(_ context.Context, mc *ModuleContext) error {
		order = append(order, "consumer")
		g, err := Need(mc, greeterPort)
		if err != nil {
			return err
		}
		if got := g.Greet("ada"); got != "hello ada" {
			t.Errorf("Greet = %q", got)
		}
		return nil
	})
	provider := module("provider", refs(greeterPort.Ref()), nil, func(_ context.Context, mc *ModuleContext) error {
		order = append(order, "provider")
		return Provide(mc, greeterPort, aGreeter())
	})
	// Use order is consumer first; dependency order must register the provider first.
	mustUse(t, app, consumer, provider)
	if err := app.Startup(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Shutdown(context.Background()) })
	if !slices.Equal(order, []string{"provider", "consumer"}) {
		t.Fatalf("register order = %v", order)
	}
}

func TestValueModuleProvidesPort(t *testing.T) {
	t.Parallel()
	app := newQuietApp(t)
	var got string
	mustUse(t, app,
		module("consumer", nil, refs(storePort.Ref()), func(_ context.Context, mc *ModuleContext) error {
			s, err := Need(mc, storePort)
			if err != nil {
				return err
			}
			got = s.Get("k")
			return nil
		}),
		ValueModule("store-double", storePort, aStore()),
	)
	if err := app.Startup(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Shutdown(context.Background()) })
	if got != "value:k" {
		t.Fatalf("store returned %q", got)
	}
}

func TestValueModuleRejectsNilValue(t *testing.T) {
	t.Parallel()
	app := newQuietApp(t)
	var nilStore store
	mustUse(t, app, ValueModule("store-double", storePort, nilStore))
	err := app.Startup(context.Background())
	if !errors.Is(err, ErrNilPortValue) || !strings.Contains(err.Error(), `"store-double"`) {
		t.Fatalf("Startup = %v, want ErrNilPortValue naming the module", err)
	}
}

func TestPortsAreIdentifiedByValue(t *testing.T) {
	t.Parallel()
	app := newQuietApp(t)
	otherStore := NewPort[store]("store") // same name and type, but a different port
	mustUse(t, app,
		ValueModule("other", otherStore, aStore()),
		module("consumer", nil, refs(storePort.Ref()), nil),
	)
	problems := moduleProblems(t, app)
	if !hasProblem(problems, ProblemMissingPort, storePort.Ref()) || len(problems) != 1 {
		t.Fatalf("problems = %v, want only storePort missing", problems)
	}
	if storePort.Ref() == otherStore.Ref() {
		t.Fatal("two NewPort calls produced equal refs")
	}
}

func TestModuleWiringReportsEveryProblemAtOnce(t *testing.T) {
	t.Parallel()
	app := newQuietApp(t)
	missing := NewPort[store]("missing")
	registered := false
	mustUse(t, app,
		module("a", refs(greeterPort.Ref()), refs(storePort.Ref()), func(context.Context, *ModuleContext) error {
			registered = true
			return nil
		}).on("admin"),
		module("b", refs(greeterPort.Ref()), nil, nil),
		module("c", nil, refs(missing.Ref()), nil),
	)
	problems := moduleProblems(t, app)
	for _, want := range []struct {
		kind ModuleProblemKind
		port PortRef
	}{
		{ProblemDuplicatePort, greeterPort.Ref()},
		{ProblemMissingPort, storePort.Ref()},
		{ProblemMissingPort, missing.Ref()},
	} {
		if !hasProblem(problems, want.kind, want.port) {
			t.Errorf("missing %s problem for %s in %v", want.kind, want.port, problems)
		}
	}
	if countKind(problems, ProblemMissingListener) != 1 {
		t.Errorf("missing listener not reported with port problems: %v", problems)
	}
	if registered {
		t.Fatal("a module registered although wiring was invalid")
	}
}

func TestModuleErrorNamesModulesAndPorts(t *testing.T) {
	t.Parallel()
	app := newQuietApp(t)
	mustUse(t, app,
		module("runs", nil, refs(storePort.Ref()), nil),
		module("gates", nil, refs(storePort.Ref()), nil),
		module("p1", refs(greeterPort.Ref()), nil, nil),
		ValueModule("p2", greeterPort, aGreeter()),
	)
	msg := app.Startup(context.Background()).Error()
	for _, want := range []string{`missing port "store" [runs, gates]`, `duplicate port "greeter" [p1, p2]`} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not contain %q", msg, want)
		}
	}
}

func TestModuleCyclesAreReported(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		modules []Module
		path    []string
	}{
		"two modules": {[]Module{
			module("a", refs(greeterPort.Ref()), refs(storePort.Ref()), nil),
			module("b", refs(storePort.Ref()), refs(greeterPort.Ref()), nil),
		}, []string{"a", "b", "a"}},
		"self": {[]Module{
			module("self", refs(greeterPort.Ref()), refs(greeterPort.Ref()), nil),
		}, []string{"self", "self"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			app := newQuietApp(t)
			mustUse(t, app, tc.modules...)
			problems := moduleProblems(t, app)
			i := slices.IndexFunc(problems, func(p ModuleProblem) bool { return p.Kind == ProblemCycle })
			if i < 0 {
				t.Fatalf("no cycle problem in %v", problems)
			}
			if got := problems[i].Modules; !slices.Equal(got, tc.path) {
				t.Fatalf("cycle path = %v, want %v", got, tc.path)
			}
		})
	}
}

func TestInvalidModuleNamesDoNotHideWiringProblems(t *testing.T) {
	t.Parallel()
	app := newQuietApp(t)
	missing := NewPort[store]("missing")
	registered := false
	register := func(context.Context, *ModuleContext) error {
		registered = true
		return nil
	}
	mustUse(t, app,
		module("dup", refs(greeterPort.Ref()), nil, register),
		module("dup", refs(greeterPort.Ref()), refs(missing.Ref()), register).on("public"),
		module("", refs(PortRef{}), refs(storePort.Ref()), register).on("internal"),
	)
	err := app.CheckModules()
	var modErr *ModuleError
	if !errors.As(err, &modErr) {
		t.Fatalf("CheckModules = %v", err)
	}
	for _, want := range []struct {
		kind ModuleProblemKind
		port PortRef
	}{
		{ProblemDuplicatePort, greeterPort.Ref()},
		{ProblemMissingPort, missing.Ref()},
		{ProblemMissingPort, storePort.Ref()},
	} {
		if !hasProblem(modErr.Problems, want.kind, want.port) {
			t.Errorf("missing %s for %s: %v", want.kind, want.port, err)
		}
	}
	if countKind(modErr.Problems, ProblemInvalid) != 3 || countKind(modErr.Problems, ProblemMissingListener) != 2 {
		t.Errorf("CheckModules = %v, want invalid names/port and both missing listeners", err)
	}
	var startupProblems *ModuleError
	if got := app.Startup(t.Context()); !errors.As(got, &startupProblems) || !reflect.DeepEqual(startupProblems.Problems, modErr.Problems) {
		t.Errorf("Startup = %v, want the same complete problems as CheckModules", got)
	}
	if registered {
		t.Error("invalid composition registered a module")
	}
}

func TestInvalidDeclarationsAreReported(t *testing.T) {
	t.Parallel()
	app := newQuietApp(t)
	notInterface := NewPort[string]("plain")
	var nilPort *Port[store]
	mustUse(t, app,
		nil,                          // nil module
		module("", nil, nil, nil),    // unnamed module
		module("dup", nil, nil, nil), //
		module("dup", nil, nil, nil), // duplicate module name
		module("zero", refs(PortRef{}), refs(notInterface.Ref(), nilPort.Ref()), nil),                            // zero, non-interface and nil ports
		module("twice", refs(greeterPort.Ref(), greeterPort.Ref()), refs(storePort.Ref(), storePort.Ref()), nil), // ports listed twice
		module("listeners", nil, nil, nil).on("", "public", "public"),                                            // unnamed and repeated listener
		ValueModule("store", storePort, aStore()),
	)
	var nilListener *testListener
	worker := newListener("worker", nil)
	worker.phase = component.DrainWorkers
	for _, l := range []struct {
		name string
		l    Listener
	}{{"", newListener("x", nil)}, {"public", newListener("public", nil)}, {"public", newListener("public", nil)}, {"nil", nilListener}, {"worker", worker}} {
		if err := app.Listen(l.name, l.l); err != nil {
			t.Fatal(err)
		}
	}
	problems := moduleProblems(t, app)
	// Modules: nil, unnamed, duplicate name, zero port, non-interface port, nil port, provided twice, needed twice,
	// unnamed listener, repeated listener. Listeners: unnamed, duplicate, nil, not ingress.
	if got := countKind(problems, ProblemInvalid); got != 14 {
		t.Fatalf("got %d invalid problems, want 14: %v", got, problems)
	}
	if countKind(problems, ProblemCycle) != 0 || countKind(problems, ProblemDuplicatePort) != 0 {
		t.Fatalf("a port listed twice by one module was also reported as a cycle or duplicate: %v", problems)
	}
}

func TestDeclaredPortMustBeProvided(t *testing.T) {
	t.Parallel()
	app := newQuietApp(t)
	mustUse(t, app, module("lazy", refs(greeterPort.Ref()), nil, nil))
	problems := moduleProblems(t, app)
	if !hasProblem(problems, ProblemNotProvided, greeterPort.Ref()) {
		t.Fatalf("problems = %v", problems)
	}
}

func TestModuleContextRejectsUndeclaredAndNilPorts(t *testing.T) {
	t.Parallel()
	app := newQuietApp(t)
	var nilPort *Port[store]
	mustUse(t, app, module("m", nil, nil, func(_ context.Context, mc *ModuleContext) error {
		if mc.Name() != "m" || mc.Logger() == nil {
			t.Errorf("Name = %q, Logger = %v", mc.Name(), mc.Logger())
		}
		if err := mc.AddComponent(nil); err == nil {
			t.Error("AddComponent(nil) succeeded")
		}
		if _, err := Need(mc, storePort); !errors.Is(err, ErrPortNotDeclared) {
			t.Errorf("Need undeclared = %v", err)
		}
		if _, err := Need(mc, nilPort); !errors.Is(err, ErrPortNotDeclared) {
			t.Errorf("Need nil port = %v", err)
		}
		if err := Provide(mc, greeterPort, aGreeter()); !errors.Is(err, ErrPortNotDeclared) {
			t.Errorf("Provide undeclared = %v", err)
		}
		if err := Provide(mc, nilPort, aStore()); !errors.Is(err, ErrPortNotDeclared) {
			t.Errorf("Provide nil port = %v", err)
		}
		return nil
	}))
	if err := app.Startup(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Shutdown(context.Background()) })
}

func TestProvideRejectsNilAndRepeatedValues(t *testing.T) {
	t.Parallel()
	app := newQuietApp(t)
	mustUse(t, app, module("m", refs(greeterPort.Ref()), nil, func(_ context.Context, mc *ModuleContext) error {
		var nilGreeter greeter
		if err := Provide(mc, greeterPort, nilGreeter); !errors.Is(err, ErrNilPortValue) {
			t.Errorf("Provide nil = %v", err)
		}
		var typedNil greeterFunc
		if err := Provide(mc, greeterPort, greeter(typedNil)); !errors.Is(err, ErrNilPortValue) {
			t.Errorf("Provide typed nil = %v", err)
		}
		if err := Provide(mc, greeterPort, aGreeter()); err != nil {
			return err
		}
		if err := Provide(mc, greeterPort, aGreeter()); !errors.Is(err, ErrPortAlreadyProvided) {
			t.Errorf("second Provide = %v", err)
		}
		return nil
	}))
	if err := app.Startup(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Shutdown(context.Background()) })
}

func TestModuleContextClosesAfterRegister(t *testing.T) {
	t.Parallel()
	app := newQuietApp(t)
	mustListen(t, app, "public", nil)
	var saved *ModuleContext
	mustUse(t, app, module("m", refs(greeterPort.Ref()), nil, func(_ context.Context, mc *ModuleContext) error {
		saved = mc
		return Provide(mc, greeterPort, aGreeter())
	}).on("public"))
	if err := app.Startup(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Shutdown(context.Background()) })
	for name, err := range map[string]error{
		"AddComponent": saved.AddComponent(&mockComponent{name: "late"}),
		"Handle":       saved.Handle("public", "/late", http.NotFoundHandler()),
		"Provide":      Provide(saved, greeterPort, aGreeter()),
	} {
		if !errors.Is(err, ErrModuleContextClosed) {
			t.Errorf("late %s = %v, want ErrModuleContextClosed", name, err)
		}
	}
	if _, err := Need(saved, greeterPort); !errors.Is(err, ErrModuleContextClosed) {
		t.Errorf("late Need = %v", err)
	}
}

func TestRegisterFailureRollsBackAndNamesModule(t *testing.T) {
	t.Parallel()
	app := newQuietApp(t)
	boom := errors.New("boom")
	var order []string
	app.OnConfigure(func(_ context.Context, a *App[*testConfig]) error {
		return a.RegisterComponent(&orderTrackingComponent{name: "configured", order: &order})
	})
	app.OnAfterStop(func(context.Context) error {
		order = append(order, "after_stop")
		return nil
	})
	laterRegistered := false
	mustUse(t, app,
		module("first", refs(greeterPort.Ref()), nil, func(_ context.Context, mc *ModuleContext) error {
			if err := mc.AddComponent(&orderTrackingComponent{name: "first-res", order: &order}); err != nil {
				return err
			}
			return Provide(mc, greeterPort, aGreeter())
		}),
		module("broken", refs(storePort.Ref()), refs(greeterPort.Ref()), func(_ context.Context, mc *ModuleContext) error {
			if err := mc.AddComponent(&orderTrackingComponent{name: "broken-res", order: &order}); err != nil {
				return err
			}
			return boom
		}),
		module("later", nil, refs(storePort.Ref()), func(context.Context, *ModuleContext) error {
			laterRegistered = true
			return nil
		}),
	)
	err := app.Startup(context.Background())
	var startErr *StartupError
	if !errors.As(err, &startErr) || startErr.Phase != PhaseModules || !errors.Is(err, boom) {
		t.Fatalf("Startup = %v", err)
	}
	if !strings.Contains(err.Error(), `"broken"`) {
		t.Fatalf("error %q does not name the module", err)
	}
	if startErr.Rollback != nil {
		t.Fatalf("Rollback = %v", startErr.Rollback)
	}
	if laterRegistered {
		t.Fatal("a module registered after an earlier Register failed")
	}
	// Nothing started, so rollback stops nothing but still runs teardown hooks once.
	if !slices.Equal(order, []string{"after_stop"}) {
		t.Fatalf("order = %v", order)
	}
	if err := app.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown after failed startup = %v", err)
	}
	if !slices.Equal(order, []string{"after_stop"}) {
		t.Fatalf("teardown ran twice: %v", order)
	}
}

func TestListenerStartFailureStopsModuleComponents(t *testing.T) {
	t.Parallel()
	app := newQuietApp(t)
	var order []string
	boom := errors.New("bind failed")
	l := mustListen(t, app, "public", &order)
	l.startErr = boom
	mustUse(t, app, module("m", nil, nil, func(_ context.Context, mc *ModuleContext) error {
		return mc.AddComponent(&orderTrackingComponent{name: "module", order: &order})
	}))
	err := app.Startup(context.Background())
	var startErr *StartupError
	if !errors.As(err, &startErr) || startErr.Phase != PhaseStart || !errors.Is(err, boom) {
		t.Fatalf("Startup = %v", err)
	}
	if !slices.Equal(order, []string{"module:start", "public:start", "module:stop"}) {
		t.Fatalf("order = %v", order)
	}
}

func TestConfigureComponentsStartBeforeModulesAndListenersLast(t *testing.T) {
	t.Parallel()
	app := newQuietApp(t)
	var order []string
	app.OnConfigure(func(_ context.Context, a *App[*testConfig]) error {
		return a.RegisterComponent(&orderTrackingComponent{name: "configured", order: &order})
	})
	mustListen(t, app, "public", &order)
	mustUse(t, app, module("m", nil, nil, func(_ context.Context, mc *ModuleContext) error {
		return mc.AddComponentInPhase(&workerComponent{orderTrackingComponent{name: "module", order: &order}}, component.PhaseResources)
	}))
	if err := app.Startup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := app.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"configured:start", "module:start", "public:start",
		"public:quiesce", "public:drain", "module:drain",
		"public:stop", "module:stop", "configured:stop",
	}
	if !slices.Equal(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestConfigureHookCanUseModulesAndListeners(t *testing.T) {
	t.Parallel()
	app := newQuietApp(t)
	var got string
	app.OnConfigure(func(_ context.Context, a *App[*testConfig]) error {
		return errors.Join(
			a.Use(
				module("consumer", nil, refs(storePort.Ref()), func(_ context.Context, mc *ModuleContext) error {
					s, err := Need(mc, storePort)
					if err != nil {
						return err
					}
					got = s.Get("k")
					return mc.Handle("public", "/x", http.NotFoundHandler())
				}).on("public"),
				ValueModule("store-double", storePort, aStore()),
			),
			a.Listen("public", newListener("public", nil)),
		)
	})
	var lateUse, lateListen error
	app.OnBeforeStart(func(context.Context) error {
		lateUse = app.Use(module("late", nil, nil, nil))
		lateListen = app.Listen("late", newListener("late", nil))
		return nil
	})
	if err := app.Startup(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Shutdown(context.Background()) })
	if got != "value:k" {
		t.Fatalf("store returned %q", got)
	}
	if !errors.Is(lateUse, ErrLifecycleUsed) || !errors.Is(lateListen, ErrLifecycleUsed) {
		t.Fatalf("after modules phase: Use = %v, Listen = %v, want ErrLifecycleUsed", lateUse, lateListen)
	}
}

func TestStartupFailsWhenCallerCancelsDuringModules(t *testing.T) {
	t.Parallel()
	app := newQuietApp(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mustUse(t, app, module("last", nil, nil, func(context.Context, *ModuleContext) error {
		cancel()
		return nil
	}))
	ran := false
	err := app.RunTask(ctx, func(context.Context) error {
		ran = true
		return nil
	})
	var startErr *StartupError
	if !errors.As(err, &startErr) || startErr.Phase != PhaseModules || !errors.Is(err, context.Canceled) {
		t.Fatalf("RunTask = %v, want a modules-phase StartupError wrapping context.Canceled", err)
	}
	if ran {
		t.Fatal("task ran after startup was canceled")
	}
}

func TestStartupStopsBetweenModulesWhenCanceled(t *testing.T) {
	t.Parallel()
	app := newQuietApp(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	secondRegistered := false
	mustUse(t, app,
		module("first", nil, nil, func(context.Context, *ModuleContext) error {
			cancel()
			return nil
		}),
		module("second", nil, nil, func(context.Context, *ModuleContext) error {
			secondRegistered = true
			return nil
		}),
	)
	if err := app.Startup(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Startup = %v", err)
	}
	if secondRegistered {
		t.Fatal("a module registered after the context was canceled")
	}
}

func TestHandleMountsOnNamedListener(t *testing.T) {
	t.Parallel()
	app := newQuietApp(t)
	public, internal := mustListen(t, app, "public", nil), mustListen(t, app, "internal", nil)
	mustUse(t, app, module("m", nil, nil, func(_ context.Context, mc *ModuleContext) error {
		ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
		if err := mc.Handle("internal", "GET /ping", ok); err != nil {
			return err
		}
		for name, err := range map[string]error{
			"duplicate route": mc.Handle("internal", "GET /ping", ok),
			"invalid pattern": mc.Handle("internal", "GET /ping/{", ok),
			"empty pattern":   mc.Handle("internal", "", ok),
			"nil handler":     mc.Handle("internal", "/nil", nil),
		} {
			if !errors.Is(err, ErrRouteConflict) {
				t.Errorf("%s = %v, want ErrRouteConflict", name, err)
			}
		}
		if err := mc.Handle("public", "/x", ok); !errors.Is(err, ErrListenerNotDeclared) {
			t.Errorf("undeclared listener = %v", err)
		}
		return nil
	}).on("internal"))
	if err := app.Startup(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Shutdown(context.Background()) })
	rec := httptest.NewRecorder()
	internal.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ping", http.NoBody))
	if rec.Body.String() != "ok" {
		t.Fatalf("internal /ping = %d %q", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	public.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ping", http.NoBody))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("public /ping = %d, want 404", rec.Code)
	}
}

func TestFallbackServesUnmatchedPathsOutsideModuleRoutes(t *testing.T) {
	t.Parallel()
	app := newQuietApp(t)
	var summary bytes.Buffer
	app.Summary.SetWriter(&summary)
	public := mustListen(t, app, "public", nil)
	write := func(body string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) })
	}
	mustUse(t, app,
		module("spa", nil, nil, func(_ context.Context, mc *ModuleContext) error {
			if err := mc.Fallback("public", write("spa")); err != nil {
				return err
			}
			for name, err := range map[string]error{
				"second fallback": mc.Fallback("public", write("again")),
				"nil handler":     mc.Fallback("public", nil),
			} {
				if !errors.Is(err, ErrRouteConflict) {
					t.Errorf("%s = %v, want ErrRouteConflict", name, err)
				}
			}
			if err := mc.Fallback("internal", write("x")); !errors.Is(err, ErrListenerNotDeclared) {
				t.Errorf("undeclared listener = %v", err)
			}
			return nil
		}).on("public"),
		// Routes mounted after the fallback, by a later module, are still reserved.
		module("api", nil, nil, func(_ context.Context, mc *ModuleContext) error {
			for _, pattern := range []string{"POST /auth/login", "/svc.v1.Service/", "GET /%61dmin/users"} {
				if err := mc.Handle("public", pattern, write("api")); err != nil {
					return err
				}
			}
			return nil
		}).on("public"))
	if err := app.Startup(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Shutdown(context.Background()) })
	for path, want := range map[string]string{
		"/":                     "spa",
		"/settings/profile":     "spa",
		"/authors":              "spa",
		"/auth/unknown":         "404",
		"/auth":                 "404",
		"/svc.v1.Service/Other": "api",
		"/admin/other":          "404",
		"/%61dmin/other":        "404",
		"/admin%2Fx":            "404",
		"/auth%2Funknown":       "404",
		"/authors%2Fx":          "spa",
	} {
		rec := httptest.NewRecorder()
		public.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, http.NoBody))
		got := rec.Body.String()
		if rec.Code == http.StatusNotFound {
			got = "404"
		}
		if got != want {
			t.Errorf("GET %s = %d %q, want %s", path, rec.Code, rec.Body, want)
		}
	}
	if !strings.Contains(summary.String(), "public fallback") {
		t.Errorf("summary does not list the fallback:\n%s", summary.String())
	}
}

func TestRoutePrefix(t *testing.T) {
	t.Parallel()
	for pattern, want := range map[string]string{
		"POST /auth/login":      "/auth",
		"POST /%61uth/login":    "/auth",
		"/a%2Fb/c":              "/a/b",
		"/%zz/c":                "",
		"/svc.v1.Service/":      "/svc.v1.Service",
		"example.com/feed/x":    "/feed",
		"GET /":                 "",
		"/{tenant}/feed":        "",
		"GET example.com/{id}/": "",
		"no-slash":              "",
	} {
		if got := routePrefix(pattern); got != want {
			t.Errorf("routePrefix(%q) = %q, want %q", pattern, got, want)
		}
	}
}

func TestFallbackConflictsWithRootRoute(t *testing.T) {
	t.Parallel()
	app := newQuietApp(t)
	mustListen(t, app, "public", nil)
	mustUse(t, app, module("m", nil, nil, func(_ context.Context, mc *ModuleContext) error {
		if err := mc.Handle("public", "/", http.NotFoundHandler()); err != nil {
			return err
		}
		return mc.Fallback("public", http.NotFoundHandler())
	}).on("public"))
	if err := app.Startup(context.Background()); !errors.Is(err, ErrRouteConflict) {
		t.Fatalf("Startup = %v, want ErrRouteConflict", err)
	}
}

func TestUseAfterLifecycleIsRejected(t *testing.T) {
	t.Parallel()
	app := newQuietApp(t)
	if err := app.Startup(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Shutdown(context.Background()) })
	if err := app.Use(module("late", nil, nil, nil)); !errors.Is(err, ErrLifecycleUsed) {
		t.Fatalf("Use = %v", err)
	}
	if err := app.Listen("x", newListener("x", nil)); !errors.Is(err, ErrLifecycleUsed) {
		t.Fatalf("Listen = %v", err)
	}
}

func TestSummaryListsModules(t *testing.T) {
	t.Parallel()
	app := newQuietApp(t)
	var buf bytes.Buffer
	app.Summary.SetWriter(&buf)
	mustListen(t, app, "internal", nil)
	mustUse(t, app,
		module("provider", refs(greeterPort.Ref()), nil, func(_ context.Context, mc *ModuleContext) error {
			if err := mc.Handle("internal", "GET /greet", http.NotFoundHandler()); err != nil {
				return err
			}
			return Provide(mc, greeterPort, aGreeter())
		}).on("internal"),
		module("consumer", nil, refs(greeterPort.Ref(), storePort.Ref()), nil),
		ValueModule("store-double", storePort, aStore()),
	)
	if err := app.Startup(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Shutdown(context.Background()) })
	out := buf.String()
	for _, want := range []string{"Modules (3)", "provider", "provides greeter", "internal GET /greet", "consumer", "needs greeter ← provider, store ← store-double"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q:\n%s", want, out)
		}
	}
}

func TestStartupWithoutModulesIsUnchanged(t *testing.T) {
	t.Parallel()
	app := newQuietApp(t)
	var buf bytes.Buffer
	app.Summary.SetWriter(&buf)
	if err := app.Startup(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Shutdown(context.Background()) })
	if strings.Contains(buf.String(), "Modules") {
		t.Fatalf("summary lists modules for an app without modules:\n%s", buf.String())
	}
}

func TestPortRefString(t *testing.T) {
	t.Parallel()
	if got := greeterPort.Ref().String(); got != "greeter (bootstrap.greeter)" {
		t.Fatalf("String = %q", got)
	}
	var nilPort *Port[store]
	if got := (PortRef{}).String(); got != "<invalid port>" || nilPort.Ref().String() != "<invalid port>" {
		t.Fatalf("zero String = %q", got)
	}
	if greeterPort.Name() != "greeter" || greeterPort.Ref().Name() != "greeter" || nilPort.Name() != "" {
		t.Fatal("Name mismatch")
	}
}

func TestDisjointCyclesAreAllReported(t *testing.T) {
	t.Parallel()
	app := newQuietApp(t)
	p1, p2 := NewPort[greeter]("p1"), NewPort[greeter]("p2")
	p3, p4 := NewPort[greeter]("p3"), NewPort[greeter]("p4")
	mustUse(t, app,
		module("a", refs(p1.Ref()), refs(p2.Ref()), nil),
		module("b", refs(p2.Ref()), refs(p1.Ref()), nil),
		module("downstream", nil, refs(p1.Ref()), nil), // depends on a cycle but is not in one
		module("c", refs(p3.Ref()), refs(p4.Ref()), nil),
		module("d", refs(p4.Ref()), refs(p3.Ref()), nil),
	)
	var cycles [][]string
	for _, p := range moduleProblems(t, app) {
		if p.Kind == ProblemCycle {
			cycles = append(cycles, p.Modules)
		}
	}
	want := [][]string{{"a", "b", "a"}, {"c", "d", "c"}}
	if !slices.EqualFunc(cycles, want, slices.Equal[[]string]) {
		t.Fatalf("cycles = %v, want %v", cycles, want)
	}
}

func TestListenerComponentNamesAreChecked(t *testing.T) {
	t.Parallel()
	app := newQuietApp(t)
	mustRegisterComponent(t, app, &mockComponent{name: "taken"})
	for name, component := range map[string]string{"public": "http", "internal": "http", "admin": "", "debug": "taken"} {
		if err := app.Listen(name, newListener(component, nil)); err != nil {
			t.Fatal(err)
		}
	}
	err := app.CheckModules()
	var modErr *ModuleError
	if !errors.As(err, &modErr) || countKind(modErr.Problems, ProblemInvalid) != 3 {
		t.Fatalf("CheckModules = %v, want duplicate, empty and taken component names", err)
	}
	for _, want := range []string{`component name "http" used by listeners`, `"admin"`, `component name "taken" already registered`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

func TestStartupKeepsCancelCauseWhenPhaseFails(t *testing.T) {
	t.Parallel()
	app := newQuietApp(t)
	cause := errors.New("deploy aborted")
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	mustUse(t, app,
		module("first", nil, nil, func(context.Context, *ModuleContext) error {
			cancel(cause)
			return nil
		}),
		module("second", nil, nil, nil),
	)
	err := app.Startup(ctx)
	var startErr *StartupError
	if !errors.As(err, &startErr) || startErr.Phase != PhaseModules || !errors.Is(err, cause) {
		t.Fatalf("Startup = %v, want a modules-phase StartupError wrapping the cancel cause", err)
	}
}

func TestStartupJoinsCancelCauseWithPhaseError(t *testing.T) {
	t.Parallel()
	app := newQuietApp(t)
	cause, boom := errors.New("deploy aborted"), errors.New("boom")
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	mustUse(t, app, module("m", nil, nil, func(context.Context, *ModuleContext) error {
		cancel(cause)
		return boom
	}))
	if err := app.Startup(ctx); !errors.Is(err, cause) || !errors.Is(err, boom) {
		t.Fatalf("Startup = %v, want both the phase error and the cancel cause", err)
	}
}

func TestPortsOfDifferentInterfacesAreNotConvertible(t *testing.T) {
	t.Parallel()
	// A conversion like (*Port[greeter])(storePort) would let Provide store a greeter under the store key.
	for _, pair := range [][2]reflect.Type{
		{reflect.TypeFor[Port[greeter]](), reflect.TypeFor[Port[store]]()},
		{reflect.TypeFor[*Port[greeter]](), reflect.TypeFor[*Port[store]]()},
	} {
		if pair[0].ConvertibleTo(pair[1]) || pair[1].ConvertibleTo(pair[0]) {
			t.Errorf("%s and %s are convertible", pair[0], pair[1])
		}
	}
}

func TestIsReserved(t *testing.T) {
	t.Parallel()
	reserved := []string{"/auth", "/a/b"}
	for path, want := range map[string]bool{
		"/auth":         true,
		"/auth/x":       true,
		"/auth%2Fx":     true,
		"/%61uth":       true,
		"/a%2Fb":        true,
		"/a%2Fb%2Fc/d":  true,
		"/a%2Fbc":       false,
		"/a/b":          false,
		"/authors%2Fx":  false,
		"/%zz":          true,
		"/":             false,
		"/settings/x/y": false,
	} {
		if got := isReserved(path, reserved); got != want {
			t.Errorf("isReserved(%q) = %v, want %v", path, got, want)
		}
	}
}
